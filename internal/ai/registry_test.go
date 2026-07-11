package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisteredProvidersIncludeBuiltins(t *testing.T) {
	names := RegisteredProviders()
	want := map[string]bool{"ollama": false, "anthropic": false, "openai": false, "openrouter": false}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, found := range want {
		if !found {
			t.Errorf("expected built-in provider %q to be registered", n)
		}
	}
}

func TestRegisterAndLookupCaseInsensitive(t *testing.T) {
	Register("TestProvider", func(c AIConfig) Provider { return NewOpenAIProvider(c) })
	f, ok := lookup("testprovider")
	if !ok {
		t.Fatal("lookup should be case-insensitive")
	}
	if f(AIConfig{}) == nil {
		t.Fatal("factory returned nil provider")
	}
}

func TestNewClientUnregisteredFallsBackToOpenAI(t *testing.T) {
	c := AIConfig{Provider: "totally-unknown", Model: "x", BaseURL: "http://example/v1"}
	client, err := NewClient(c)
	if err != nil {
		t.Fatalf("unregistered provider should fall back to OpenAI: %v", err)
	}
	if client.ProviderName() != "openai" {
		t.Errorf("expected openai provider, got %q", client.ProviderName())
	}
}

func TestSetModelAndModel(t *testing.T) {
	client, err := NewClient(AIConfig{Provider: "ollama", Model: "qwen2.5:3b"})
	if err != nil {
		t.Fatal(err)
	}
	if client.Model() != "qwen2.5:3b" {
		t.Fatalf("initial model = %q", client.Model())
	}
	client.SetModel("llama3.2:3b")
	if client.Model() != "llama3.2:3b" {
		t.Fatalf("after SetModel = %q", client.Model())
	}
}

func TestOllamaListModelsParsesTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "qwen2.5:3b", "size": 1929912432, "modified_at": "2026-06-27T12:33:41Z"},
				{"name": "sphinx:latest", "size": 18253611008, "modified_at": "2026-06-27T12:33:41Z"},
			},
		})
	}))
	defer srv.Close()

	p := NewOllamaProvider(AIConfig{BaseURL: srv.URL, Model: "qwen2.5:3b"})
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	// Sorted by name: qwen2.5:3b before sphinx:latest
	if models[0].Name != "qwen2.5:3b" {
		t.Errorf("first model = %q, want qwen2.5:3b", models[0].Name)
	}
	if models[0].Size != 1929912432 {
		t.Errorf("size = %d, wrong", models[0].Size)
	}
}

func TestOpenAIListModelsParsesData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(AIConfig{BaseURL: srv.URL, APIKey: "test"})
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0].Name != "gpt-4o" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestClientListModelsUnsupportedReturnsSentinel(t *testing.T) {
	// Anthropic does not implement ModelLister.
	client, err := NewClient(AIConfig{Provider: "anthropic", Model: "claude-3-5-sonnet", APIKey: "x"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListModels(context.Background())
	if err != ErrModelDiscoveryUnsupported {
		t.Fatalf("expected ErrModelDiscoveryUnsupported, got %v", err)
	}
}
