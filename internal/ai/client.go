// Package ai provides a provider-agnostic LLM client for Mimir's AI
// commands. It supports any OpenAI-compatible endpoint plus native Anthropic
// and Ollama providers.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Role is a chat message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat message.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Provider is the interface all LLM providers must satisfy.
type Provider interface {
	// Complete sends a chat completion request and returns the full response.
	Complete(ctx context.Context, messages []Message, opts ProviderOptions) (string, error)
	// Stream sends a chat completion request and returns a channel of token chunks.
	Stream(ctx context.Context, messages []Message, opts ProviderOptions) (<-chan string, <-chan error)
	// Name returns the provider identifier.
	Name() string
}

// ProviderOptions controls how the LLM is called.
type ProviderOptions struct {
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Model       string  `json:"model"`
	Stream      bool    `json:"stream"`
}

// Client is a provider-agnostic LLM client.
type Client struct {
	provider Provider
	model    string
	timeout  time.Duration
}

// NewClient creates a Client from the given config.
func NewClient(cfg AIConfig) (*Client, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("ai: provider not configured")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("ai: model not configured")
	}

	var p Provider
	switch strings.ToLower(cfg.Provider) {
	case "anthropic":
		p = NewAnthropicProvider(cfg)
	case "ollama":
		p = NewOllamaProvider(cfg)
	default:
		p = NewOpenAIProvider(cfg)
	}

	return &Client{
		provider: p,
		model:    cfg.Model,
		timeout:  time.Duration(cfg.Timeout) * time.Second,
	}, nil
}

// Complete sends a non-streaming chat completion request.
func (c *Client) Complete(ctx context.Context, messages []Message, opts ProviderOptions) (string, error) {
	opts.Model = c.model
	return c.provider.Complete(ctx, messages, opts)
}

// Stream sends a streaming chat completion request.
func (c *Client) Stream(ctx context.Context, messages []Message, opts ProviderOptions) (<-chan string, <-chan error) {
	opts.Model = c.model
	return c.provider.Stream(ctx, messages, opts)
}

// ProviderName returns the name of the active provider.
func (c *Client) ProviderName() string {
	return c.provider.Name()
}

// --- OpenAI-compatible provider ---

// OpenAIProvider implements Provider for any /v1/chat/completions endpoint.
type OpenAIProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIProvider creates an OpenAI-compatible provider.
func NewOpenAIProvider(cfg AIConfig) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		httpClient: &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

// openaiRequest is the JSON body for OpenAI-compatible APIs.
type openaiRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// openaiStreamChunk is the JSON body for a single SSE chunk.
type openaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, messages []Message, opts ProviderOptions) (string, error) {
	body := openaiRequest{
		Model:       opts.Model,
		Messages:    messages,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
		Stream:      false,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("ai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai: API error %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ai: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ai: empty response")
	}
	return result.Choices[0].Message.Content, nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, messages []Message, opts ProviderOptions) (<-chan string, <-chan error) {
	out := make(chan string, 64)
	errCh := make(chan error, 1)

	body := openaiRequest{
		Model:       opts.Model,
		Messages:    messages,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
		Stream:      true,
	}
	data, err := json.Marshal(body)
	if err != nil {
		errCh <- fmt.Errorf("ai: marshal request: %w", err)
		close(out)
		return out, errCh
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		errCh <- fmt.Errorf("ai: build request: %w", err)
		close(out)
		return out, errCh
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		errCh <- fmt.Errorf("ai: request failed: %w", err)
		close(out)
		return out, errCh
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		errCh <- fmt.Errorf("ai: API error %d: %s", resp.StatusCode, body)
		close(out)
		return out, errCh
	}

	go func() {
		defer close(out)
		defer close(errCh)
		defer resp.Body.Close()
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				// Parse SSE lines.
				text := string(buf[:n])
				for _, line := range strings.Split(text, "\n") {
					line = strings.TrimSpace(line)
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					payload := strings.TrimPrefix(line, "data: ")
					if payload == "[DONE]" {
						return
					}
					var chunk openaiStreamChunk
					if jerr := json.Unmarshal([]byte(payload), &chunk); jerr != nil {
						continue
					}
					if len(chunk.Choices) > 0 {
						out <- chunk.Choices[0].Delta.Content
					}
				}
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				errCh <- fmt.Errorf("ai: stream read error: %w", err)
				return
			}
		}
	}()

	return out, errCh
}

// --- Ollama provider ---

// OllamaProvider implements Provider for local Ollama.
type OllamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaProvider creates an Ollama provider.
func NewOllamaProvider(cfg AIConfig) *OllamaProvider {
	base := cfg.BaseURL
	if base == "" {
		base = "http://localhost:11434"
	}
	return &OllamaProvider{
		baseURL:    strings.TrimRight(base, "/"),
		model:      cfg.Model,
		httpClient: &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) Complete(ctx context.Context, messages []Message, opts ProviderOptions) (string, error) {
	body := map[string]any{
		"model":    p.model,
		"messages": messages,
		"stream":   false,
		"options": map[string]any{
			"temperature": opts.Temperature,
			"num_predict":  opts.MaxTokens,
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("ai: marshal: %w", err)
	}

	resp, err := p.httpClient.Post(p.baseURL+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai: ollama error %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ai: decode: %w", err)
	}
	return result.Message.Content, nil
}

func (p *OllamaProvider) Stream(ctx context.Context, messages []Message, opts ProviderOptions) (<-chan string, <-chan error) {
	out := make(chan string, 64)
	errCh := make(chan error, 1)

	body := map[string]any{
		"model":    p.model,
		"messages": messages,
		"stream":   true,
		"options": map[string]any{
			"temperature": opts.Temperature,
			"num_predict":  opts.MaxTokens,
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		errCh <- fmt.Errorf("ai: marshal: %w", err)
		close(out)
		return out, errCh
	}

	resp, err := p.httpClient.Post(p.baseURL+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		errCh <- fmt.Errorf("ai: request failed: %w", err)
		close(out)
		return out, errCh
	}

	go func() {
		defer close(out)
		defer close(errCh)
		defer resp.Body.Close()
		dec := json.NewDecoder(resp.Body)
		for {
			var chunk struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}
			if err := dec.Decode(&chunk); err == io.EOF {
				return
			} else if err != nil {
				errCh <- fmt.Errorf("ai: stream decode: %w", err)
				return
			}
			if chunk.Message.Content != "" {
				out <- chunk.Message.Content
			}
			if chunk.Done {
				return
			}
		}
	}()

	return out, errCh
}

// --- Anthropic provider ---

// AnthropicProvider implements Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewAnthropicProvider creates an Anthropic provider.
func NewAnthropicProvider(cfg AIConfig) *AnthropicProvider {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.anthropic.com/v1"
	}
	return &AnthropicProvider{
		apiKey:     cfg.APIKey,
		baseURL:    strings.TrimRight(base, "/"),
		httpClient: &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Complete(ctx context.Context, messages []Message, opts ProviderOptions) (string, error) {
	body := map[string]any{
		"model":      opts.Model,
		"max_tokens": opts.MaxTokens,
		"messages":   messages,
	}
	if opts.Temperature > 0 {
		body["temperature"] = opts.Temperature
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("ai: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai: anthropic error %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ai: decode: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("ai: empty response")
	}
	return result.Content[0].Text, nil
}

func (p *AnthropicProvider) Stream(ctx context.Context, messages []Message, opts ProviderOptions) (<-chan string, <-chan error) {
	out := make(chan string, 64)
	errCh := make(chan error, 1)

	body := map[string]any{
		"model":      opts.Model,
		"max_tokens": opts.MaxTokens,
		"messages":   messages,
		"stream":     true,
	}
	if opts.Temperature > 0 {
		body["temperature"] = opts.Temperature
	}
	data, err := json.Marshal(body)
	if err != nil {
		errCh <- fmt.Errorf("ai: marshal: %w", err)
		close(out)
		return out, errCh
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		errCh <- fmt.Errorf("ai: build request: %w", err)
		close(out)
		return out, errCh
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		errCh <- fmt.Errorf("ai: request failed: %w", err)
		close(out)
		return out, errCh
	}

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		errCh <- fmt.Errorf("ai: anthropic error %d: %s", resp.StatusCode, b)
		close(out)
		return out, errCh
	}

	go func() {
		defer close(out)
		defer close(errCh)
		defer resp.Body.Close()
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				text := string(buf[:n])
				for _, line := range strings.Split(text, "\n") {
					line = strings.TrimSpace(line)
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					payload := strings.TrimPrefix(line, "data: ")
					var chunk struct {
						Type  string `json:"type"`
						Delta struct {
							Text string `json:"text"`
						} `json:"delta"`
					}
					if jerr := json.Unmarshal([]byte(payload), &chunk); jerr != nil {
						continue
					}
					if chunk.Type == "content_block_delta" {
						out <- chunk.Delta.Text
					}
				}
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				errCh <- fmt.Errorf("ai: stream read error: %w", err)
				return
			}
		}
	}()

	return out, errCh
}
