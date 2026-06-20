package ai

import (
	"fmt"
	"os"
	"strings"
)

// AIConfig holds all AI-related configuration.
type AIConfig struct {
	Provider      string  `yaml:"provider" mapstructure:"provider"`
	BaseURL       string  `yaml:"base_url" mapstructure:"base_url"`
	APIKey        string  `yaml:"api_key" mapstructure:"api_key"`
	Model         string  `yaml:"model" mapstructure:"model"`
	MaxTokens     int     `yaml:"max_tokens" mapstructure:"max_tokens"`
	Temperature   float64 `yaml:"temperature" mapstructure:"temperature"`
	ContextWindow int     `yaml:"context_window" mapstructure:"context_window"`
	Timeout       int     `yaml:"timeout" mapstructure:"timeout"`
}

// DefaultAIConfig returns sensible defaults.
func DefaultAIConfig() AIConfig {
	return AIConfig{
		Provider:      "openrouter",
		BaseURL:       "https://openrouter.ai/api/v1",
		Model:         "",
		MaxTokens:     4096,
		Temperature:   0.1,
		ContextWindow: 50000,
		Timeout:       60,
	}
}

// LoadAIConfig loads AI config from the main Config, env vars, and .env.
// Env vars take precedence:
//   - MIMIR_AI_PROVIDER
//   - MIMIR_AI_BASE_URL
//   - MIMIR_AI_API_KEY
//   - MIMIR_AI_MODEL
//   - MIMIR_AI_MAX_TOKENS
//   - MIMIR_AI_TEMPERATURE
//   - MIMIR_AI_CONTEXT_WINDOW
//   - MIMIR_AI_TIMEOUT
func LoadAIConfig(cfg AIConfig) AIConfig {
	if v := os.Getenv("MIMIR_AI_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv("MIMIR_AI_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("MIMIR_AI_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("MIMIR_AI_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("MIMIR_AI_MAX_TOKENS"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.MaxTokens)
	}
	if v := os.Getenv("MIMIR_AI_TEMPERATURE"); v != "" {
		fmt.Sscanf(v, "%f", &cfg.Temperature)
	}
	if v := os.Getenv("MIMIR_AI_CONTEXT_WINDOW"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.ContextWindow)
	}
	if v := os.Getenv("MIMIR_AI_TIMEOUT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Timeout)
	}
	return cfg
}

// Validate checks the config for missing required fields.
func (c AIConfig) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("ai.provider is required (openrouter, anthropic, ollama, or custom)")
	}
	if c.Model == "" {
		return fmt.Errorf("ai.model is required")
	}
	if c.Provider != "ollama" && c.APIKey == "" {
		return fmt.Errorf("ai.api_key is required for provider %s", c.Provider)
	}
	return nil
}

// Redact returns a copy with the API key masked for display.
func (c AIConfig) Redact() AIConfig {
	redacted := c
	if c.APIKey != "" {
		redacted.APIKey = c.APIKey[:4] + "..." + c.APIKey[len(c.APIKey)-4:]
	}
	return redacted
}

// IsLocal returns true if the provider is a local model (Ollama).
func (c AIConfig) IsLocal() bool {
	return strings.ToLower(c.Provider) == "ollama"
}
