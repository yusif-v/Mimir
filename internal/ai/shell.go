package ai

import (
	"context"
	"fmt"
)

// Shell integrates AI into the Mimir REPL.
type Shell struct {
	client *Client
}

// NewShell creates an AI shell from the config.
func NewShell(cfg AIConfig) (*Shell, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Shell{client: client}, nil
}

// ProviderName returns the active provider name.
func (s *Shell) ProviderName() string {
	return s.client.ProviderName()
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
		Role: RoleUser,
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
		Role: RoleUser,
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
