package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
)

// mockProvider is a test double for Provider.
type mockProvider struct {
	response string
	err      error
	streamed []string
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Complete(ctx context.Context, messages []Message, opts ProviderOptions) (string, error) {
	return m.response, m.err
}

func (m *mockProvider) Stream(ctx context.Context, messages []Message, opts ProviderOptions) (<-chan string, <-chan error) {
	out := make(chan string, 64)
	errCh := make(chan error, 1)
	for _, chunk := range m.streamed {
		out <- chunk
	}
	close(out)
	return out, errCh
}

func TestNewClientOpenAI(t *testing.T) {
	cfg := AIConfig{
		Provider: "openrouter",
		BaseURL:  "https://example.com/v1",
		APIKey:   "test-key",
		Model:    "test-model",
		Timeout:  30,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if c.ProviderName() != "openai" {
		t.Fatalf("expected 'openai' provider, got %q", c.ProviderName())
	}
}

func TestNewClientOllama(t *testing.T) {
	cfg := AIConfig{
		Provider: "ollama",
		BaseURL:  "http://localhost:11434",
		Model:    "qwen3.5:4b",
		Timeout:  60,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if c.ProviderName() != "ollama" {
		t.Fatalf("expected 'ollama', got %q", c.ProviderName())
	}
}

func TestNewClientAnthropic(t *testing.T) {
	cfg := AIConfig{
		Provider: "anthropic",
		APIKey:   "sk-ant-test",
		Model:    "claude-sonnet-4",
		Timeout:  60,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if c.ProviderName() != "anthropic" {
		t.Fatalf("expected 'anthropic', got %q", c.ProviderName())
	}
}

func TestNewClientNoProvider(t *testing.T) {
	_, err := NewClient(AIConfig{})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestNewClientNoModel(t *testing.T) {
	_, err := NewClient(AIConfig{Provider: "ollama"})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestDefaultAIConfig(t *testing.T) {
	cfg := DefaultAIConfig()
	if cfg.Provider != "openrouter" {
		t.Fatalf("expected openrouter default, got %q", cfg.Provider)
	}
	if cfg.MaxTokens != 4096 {
		t.Fatalf("expected 4096 max tokens, got %d", cfg.MaxTokens)
	}
	if cfg.Timeout != 60 {
		t.Fatalf("expected 60s timeout, got %d", cfg.Timeout)
	}
}

func TestLoadAIConfigEnv(t *testing.T) {
	cfg := DefaultAIConfig()
	t.Setenv("MIMIR_AI_PROVIDER", "ollama")
	t.Setenv("MIMIR_AI_MODEL", "qwen3.5:4b")
	t.Setenv("MIMIR_AI_BASE_URL", "http://localhost:11434")
	t.Setenv("MIMIR_AI_API_KEY", "test-key")
	t.Setenv("MIMIR_AI_MAX_TOKENS", "2048")
	t.Setenv("MIMIR_AI_TEMPERATURE", "0.5")
	t.Setenv("MIMIR_AI_CONTEXT_WINDOW", "30000")
	t.Setenv("MIMIR_AI_TIMEOUT", "120")

	cfg = LoadAIConfig(cfg)
	if cfg.Provider != "ollama" {
		t.Fatalf("expected ollama, got %q", cfg.Provider)
	}
	if cfg.Model != "qwen3.5:4b" {
		t.Fatalf("expected qwen3.5:4b, got %q", cfg.Model)
	}
	if cfg.MaxTokens != 2048 {
		t.Fatalf("expected 2048, got %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.5 {
		t.Fatalf("expected 0.5, got %f", cfg.Temperature)
	}
	if cfg.Timeout != 120 {
		t.Fatalf("expected 120, got %d", cfg.Timeout)
	}
}

func TestAIConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  AIConfig
		err  bool
	}{
		{"missing provider", AIConfig{}, true},
		{"missing model", AIConfig{Provider: "ollama"}, true},
		{"missing key for remote", AIConfig{Provider: "openai", Model: "gpt-4"}, true},
		{"valid ollama", AIConfig{Provider: "ollama", Model: "qwen3.5:4b"}, false},
		{"valid openai", AIConfig{Provider: "openai", Model: "gpt-4", APIKey: "sk-test"}, false},
	}
	for _, tt := range tests {
		err := tt.cfg.Validate()
		if tt.err && err == nil {
			t.Errorf("%s: expected error, got nil", tt.name)
		}
		if !tt.err && err != nil {
			t.Errorf("%s: unexpected error: %v", tt.name, err)
		}
	}
}

func TestAIConfigRedact(t *testing.T) {
	cfg := AIConfig{
		Provider: "openai",
		APIKey:   "sk-1234567890abcdef",
		Model:    "gpt-4",
	}
	redacted := cfg.Redact()
	if !strings.Contains(redacted.APIKey, "...") {
		t.Fatalf("expected redacted key to contain '...', got %q", redacted.APIKey)
	}
	if redacted.Model != "gpt-4" {
		t.Fatalf("model should not be redacted, got %q", redacted.Model)
	}
}

func TestAIConfigIsLocal(t *testing.T) {
	ollamaCfg := AIConfig{Provider: "ollama"}
	openaiCfg := AIConfig{Provider: "openrouter"}
	if !ollamaCfg.IsLocal() {
		t.Fatal("ollama should be local")
	}
	if openaiCfg.IsLocal() {
		t.Fatal("openrouter should not be local")
	}
}

func TestBuildContextNoCase(t *testing.T) {
	msgs := BuildContext(nil, DefaultContextOptions())
	if msgs != nil {
		t.Fatalf("expected nil for nil case, got %v", msgs)
	}
}

func TestBuildContextEmptyCase(t *testing.T) {
	c := &cases.Case{Name: "test-case", Status: "open", Path: t.TempDir()}
	msgs := BuildContext(c, DefaultContextOptions())
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != RoleSystem {
		t.Fatalf("expected system role, got %q", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "test-case") {
		t.Fatalf("expected case name in context, got %q", msgs[0].Content)
	}
}

func TestBuildContextWithEvidence(t *testing.T) {
	dir := t.TempDir()
	c := &cases.Case{Name: "test", Status: "open", Path: dir}
	c.AppendEvidence(cases.EvidenceRecord{
		Op:     "add",
		Name:   "malware.exe",
		SHA256: "abc123def456789012345678",
		Size:   1024,
		Tags:   []string{"pe", "packed"},
	})
	opts := DefaultContextOptions()
	opts.IncludeIOCs = false
	opts.IncludeTimeline = false
	msgs := BuildContext(c, opts)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "malware.exe") {
		t.Fatalf("expected evidence name in context")
	}
	if !strings.Contains(msgs[0].Content, "pe") {
		t.Fatalf("expected evidence tags in context")
	}
}

func TestBuildContextWithIOCs(t *testing.T) {
	dir := t.TempDir()
	c := &cases.Case{Name: "test", Status: "open", Path: dir}
	c.AppendIOC(cases.IOCRecord{
		Type:   "ipv4",
		Value:  "1.2.3.4",
		Source: "manual",
	})
	opts := DefaultContextOptions()
	opts.IncludeEvidence = false
	opts.IncludeTimeline = false
	msgs := BuildContext(c, opts)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "1.2.3.4") {
		t.Fatalf("expected IOC in context")
	}
}

func TestBuildContextWithTimeline(t *testing.T) {
	c := &cases.Case{Name: "test", Status: "open", Path: t.TempDir()}
	c.AppendEvent(cases.TimelineEvent{
		Type:      "tool_run",
		Timestamp: "2026-06-20T12:00:00Z",
		Payload:   map[string]any{"tool": "hash"},
	})
	opts := DefaultContextOptions()
	opts.IncludeEvidence = false
	opts.IncludeIOCs = false
	msgs := BuildContext(c, opts)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "tool_run") {
		t.Fatalf("expected timeline event in context")
	}
}

func TestContextOptionsLimits(t *testing.T) {
	dir := t.TempDir()
	c := &cases.Case{Name: "test", Status: "open", Path: dir}
	// Add 5 evidence items with different names.
	for i := 0; i < 5; i++ {
		c.AppendEvidence(cases.EvidenceRecord{
			Op:     "add",
			Name:   fmt.Sprintf("file%d.bin", i),
			SHA256: "abcdef1234567890abcdef1234567890",
			Size:   100,
		})
	}
	opts := DefaultContextOptions()
	opts.MaxEvidenceItems = 2
	opts.IncludeIOCs = false
	opts.IncludeTimeline = false
	msgs := BuildContext(c, opts)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	// Should only show 2 evidence items (limited).
	count := strings.Count(msgs[0].Content, "sha256:")
	if count != 2 {
		t.Fatalf("expected 2 evidence items (limited), got %d", count)
	}
}

func TestSystemPromptPrefix(t *testing.T) {
	c := &cases.Case{Name: "test", Status: "open"}
	msgs := BuildContext(c, DefaultContextOptions())
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 message")
	}
	if !strings.HasPrefix(msgs[0].Content, SystemPromptPrefix) {
		t.Fatalf("expected system prompt prefix, got: %q", msgs[0].Content[:50])
	}
}
