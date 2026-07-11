package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
)

// ErrModelDiscoveryUnsupported is returned when the active provider cannot
// enumerate its available models (e.g. Anthropic has no list endpoint).
var ErrModelDiscoveryUnsupported = errors.New("ai: model discovery not supported by this provider")

// ModelInfo describes a single model exposed by a provider.
type ModelInfo struct {
	Name     string
	Size     int64  // bytes, 0 if unknown
	Modified string // provider-supplied timestamp, empty if unknown
}

// ModelLister is an optional capability: providers that can enumerate the
// models they serve implement it. Client.ListModels uses an interface
// assertion so providers without discovery simply return the sentinel error.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// ListModels queries Ollama's /api/tags endpoint.
func (p *OllamaProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ai: build request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai: ollama error %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ai: decode: %w", err)
	}

	out := make([]ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		out = append(out, ModelInfo{Name: m.Name, Size: m.Size, Modified: m.ModifiedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ListModels queries an OpenAI-compatible /models endpoint.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("ai: build request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai: API error %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ai: decode: %w", err)
	}

	out := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		out = append(out, ModelInfo{Name: m.ID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
