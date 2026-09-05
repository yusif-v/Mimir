package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdAnalyzeRequiresOpenCase(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.exe")
	if err := os.WriteFile(sample, []byte("MZ"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdAnalyze([]string{sample}); err == nil {
		t.Error("cmdAnalyze with no open case: expected error")
	}
}

func TestCmdAnalyzeRequiresPath(t *testing.T) {
	app := newTestApp(t)
	if err := app.cmdAnalyze(nil); err == nil {
		t.Error("cmdAnalyze with no args: expected usage error")
	}
}

func TestCmdAnalyzeMissingSample(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.Cases.Create("t"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Cases.Open("t"); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdAnalyze([]string{"/nonexistent/nope.exe"}); err == nil {
		t.Error("cmdAnalyze with missing sample: expected error")
	}
}

// TestCmdAnalyzeSkipsUninstalledTools: none of yara/capa/oletools are
// installed in a fresh test app, so every applicable tool for a PE sample
// should be skipped with a warning, evidence should still be registered,
// and the command should still succeed overall (skip, don't abort).
func TestCmdAnalyzeSkipsUninstalledTools(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.Cases.Create("t"); err != nil {
		t.Fatal(err)
	}
	c, err := app.Cases.Open("t")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.exe")
	data := append([]byte("MZ"), make([]byte, 62)...)
	if err := os.WriteFile(sample, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := app.cmdAnalyze([]string{sample}); err != nil {
		t.Fatalf("cmdAnalyze: %v", err)
	}

	found := false
	for _, e := range c.Evidence() {
		if e.Name == "sample.exe" {
			found = true
		}
	}
	if !found {
		t.Error("expected sample.exe to be registered as evidence")
	}
}

func TestStaticToolsForKind(t *testing.T) {
	tests := []struct {
		kind string
		want []string
	}{
		{"pe", []string{"yara", "capa"}},
		{"elf", []string{"yara", "capa"}},
		{"ole", []string{"yara", "oletools"}},
		{"ooxml", []string{"yara", "oletools"}},
		{"unknown", []string{"yara"}},
	}
	for _, tc := range tests {
		got := staticToolNames(tc.kind)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("staticToolNames(%s): got %v, want %v", tc.kind, got, tc.want)
		}
	}
}
