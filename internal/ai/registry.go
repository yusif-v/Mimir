package ai

import "strings"

// Factory builds a Provider from config. Providers register themselves via
// Register (typically in an init function) so new providers can be added
// without editing the client's construction logic.
type Factory func(AIConfig) Provider

var registry = map[string]Factory{}

// Register adds a provider factory under the given name (case-insensitive).
func Register(name string, f Factory) {
	registry[strings.ToLower(name)] = f
}

// lookup returns the factory for a provider name, or false if unregistered.
func lookup(name string) (Factory, bool) {
	f, ok := registry[strings.ToLower(name)]
	return f, ok
}

// RegisteredProviders returns the names of all registered providers.
func RegisteredProviders() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	return names
}

func init() {
	Register("ollama", func(c AIConfig) Provider { return NewOllamaProvider(c) })
	Register("anthropic", func(c AIConfig) Provider { return NewAnthropicProvider(c) })
	Register("openai", func(c AIConfig) Provider { return NewOpenAIProvider(c) })
	Register("openrouter", func(c AIConfig) Provider { return NewOpenAIProvider(c) })
}
