package theme

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultTheme(t *testing.T) {
	th := DefaultTheme()
	if th.Name != "default" {
		t.Fatalf("expected name 'default', got %q", th.Name)
	}
	if th.Prompt != PromptStyleInline {
		t.Fatalf("expected inline prompt style, got %v", th.Prompt)
	}
	if th.Colors.Brand != "blue" {
		t.Fatalf("expected blue brand, got %q", th.Colors.Brand)
	}
}

func TestBuiltinTheme(t *testing.T) {
	for _, name := range []string{"default", "kali", "minimal", "no-color"} {
		th := BuiltinTheme(name)
		if th.Name != name {
			t.Fatalf("BuiltinTheme(%q) returned theme %q", name, th.Name)
		}
	}
	// Unknown name falls back to default.
	th := BuiltinTheme("nonexistent")
	if th.Name != "default" {
		t.Fatalf("expected fallback to default, got %q", th.Name)
	}
}

func TestBuiltinThemesCount(t *testing.T) {
	themes := BuiltinThemes()
	if len(themes) != 4 {
		t.Fatalf("expected 4 built-in themes, got %d", len(themes))
	}
}

func TestResolveColor(t *testing.T) {
	tests := []struct {
		input Color
		want  string
	}{
		{"green", "\033[32m"},
		{"blue", "\033[34m"},
		{"cyan", "\033[36m"},
		{"bright_cyan", "\033[96m"},
		{"dim", "\033[2m"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ResolveColor(tt.input)
		if got != tt.want {
			t.Errorf("ResolveColor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveColorNoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")
	if got := ResolveColor("green"); got != "" {
		t.Errorf("expected empty string with NO_COLOR set, got %q", got)
	}
}

func TestColorize(t *testing.T) {
	got := Colorize("test", "green")
	want := "\033[32mtest\033[0m"
	if got != want {
		t.Errorf("Colorize() = %q, want %q", got, want)
	}
}

func TestColorizeEmptyColor(t *testing.T) {
	got := Colorize("test", "")
	if got != "test" {
		t.Errorf("expected plain text with empty color, got %q", got)
	}
}

func TestKaliTheme(t *testing.T) {
	th := KaliTheme()
	if th.Prompt != PromptStyleTwoLine {
		t.Fatalf("kali should use twoline prompt, got %v", th.Prompt)
	}
	if th.Colors.User != "cyan" {
		t.Fatalf("kali user should be cyan, got %q", th.Colors.User)
	}
}

func TestMinimalTheme(t *testing.T) {
	th := MinimalTheme()
	if th.Layout.Marker != ">" {
		t.Fatalf("minimal marker should be '>', got %q", th.Layout.Marker)
	}
}

func TestNoColorTheme(t *testing.T) {
	th := NoColorTheme()
	for k, v := range map[string]Color{
		"User": th.Colors.User,
		"Brand": th.Colors.Brand,
		"Case": th.Colors.Case,
	} {
		if v != "" {
			t.Errorf("no-color theme %s should be empty, got %q", k, v)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test-theme.yaml"
	th := KaliTheme()
	if err := Save(th, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Name != th.Name {
		t.Fatalf("loaded name = %q, want %q", loaded.Name, th.Name)
	}
	if loaded.Colors.User != th.Colors.User {
		t.Fatalf("loaded user color = %q, want %q", loaded.Colors.User, th.Colors.User)
	}
}

func TestLoadNonexistent(t *testing.T) {
	th, err := Load("/nonexistent/path/theme.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if th.Name != "default" {
		t.Fatalf("expected default theme fallback, got %q", th.Name)
	}
}

func TestThemeYAML(t *testing.T) {
	th := DefaultTheme()
	data, err := yaml.Marshal(th)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "name: default") {
		t.Fatalf("YAML should contain name, got: %s", data)
	}
}
