package ai

import (
	"context"
	"fmt"

	"github.com/yusif-v/mimir/internal/cases"
)

// Shell integrates AI into the Mimir REPL.
type Shell struct {
	client *Client
	// local records whether the configured provider runs on this host. Local
	// providers may receive unmasked case context; remote providers get
	// PII-safe (filename-masked) context.
	local bool
}

// NewShell creates an AI shell from the config.
func NewShell(cfg AIConfig) (*Shell, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Shell{client: client, local: cfg.IsLocal()}, nil
}

// IsLocal reports whether the active provider is a local (self-hosted) model.
func (s *Shell) IsLocal() bool {
	return s.local
}

// ProviderName returns the active provider name.
func (s *Shell) ProviderName() string {
	return s.client.ProviderName()
}

// Model returns the active model.
func (s *Shell) Model() string {
	return s.client.Model()
}

// SetModel rebinds the active model on the live client.
func (s *Shell) SetModel(model string) {
	s.client.SetModel(model)
}

// ListModels returns the models the active provider can serve.
func (s *Shell) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return s.client.ListModels(ctx)
}

// Ask sends a free-form question to the LLM, optionally with case context.
func (s *Shell) Ask(ctx context.Context, question string, messages []Message) error {
	if messages == nil {
		messages = []Message{}
	}
	messages = append(messages, Message{Role: RoleUser, Content: question})
	return s.complete(ctx, messages, false)
}

// AskStream sends a question and streams the response token by token.
func (s *Shell) AskStream(ctx context.Context, question string, messages []Message) error {
	if messages == nil {
		messages = []Message{}
	}
	messages = append(messages, Message{Role: RoleUser, Content: question})
	return s.complete(ctx, messages, true)
}

// Analyze sends a case analysis request.
func (s *Shell) Analyze(ctx context.Context, messages []Message) error {
	if messages == nil {
		messages = []Message{}
	}
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: "Analyze the case data above. Summarize findings, flag suspicious items, and identify gaps.",
	})
	return s.complete(ctx, messages, true)
}

// Suggest sends a next-steps request.
func (s *Shell) Suggest(ctx context.Context, messages []Message) error {
	if messages == nil {
		messages = []Message{}
	}
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: "Based on the case data, what are the recommended next investigation steps? Be specific and actionable.",
	})
	return s.complete(ctx, messages, true)
}

// Explain sends an explanation request about some output.
func (s *Shell) Explain(ctx context.Context, output string, messages []Message) error {
	if messages == nil {
		messages = []Message{}
	}
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: fmt.Sprintf("Explain what this output means in the context of a DFIR investigation:\n\n%s", output),
	})
	return s.complete(ctx, messages, false)
}

// Summarize produces a structured case narrative (what happened, key
// evidence, notable IOCs, recommended next steps) and returns it as a string
// rather than streaming. Returns an error if no case context is available.
func (s *Shell) Summarize(ctx context.Context, c *cases.Case) (string, error) {
	if c == nil {
		return "", fmt.Errorf("ai: no case available")
	}
	messages := BuildAnalysisContextLocal(c, s.local)
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: "Write a concise incident summary for this case. Cover: (1) what the evidence shows happened, (2) the most important evidence items, (3) notable IOCs, (4) recommended next investigation steps. Be specific and technical.",
	})
	return s.completeString(ctx, messages)
}

// completeString runs a non-streaming completion and returns the full text.
func (s *Shell) completeString(ctx context.Context, messages []Message) (string, error) {
	opts := ProviderOptions{MaxTokens: 2048, Temperature: 0.2}
	resp, err := s.client.Complete(ctx, messages, opts)
	if err != nil {
		return "", err
	}
	return resp, nil
}

func (s *Shell) complete(ctx context.Context, messages []Message, stream bool) error {
	opts := ProviderOptions{
		MaxTokens:   4096,
		Temperature: 0.1,
	}

	if stream {
		out, errCh := s.client.Stream(ctx, messages, opts)
		for {
			select {
			case chunk, ok := <-out:
				if !ok {
					return nil
				}
				fmt.Print(chunk)
			case err := <-errCh:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	resp, err := s.client.Complete(ctx, messages, opts)
	if err != nil {
		return err
	}
	fmt.Println(resp)
	return nil
}
