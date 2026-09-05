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

// TestCmdAnalyzeReusesExistingEvidenceByHash verifies the hash-dedup path in
// cmdAnalyze: if the exact same content is already registered as evidence
// under one name, analyzing a different file with identical bytes must reuse
// the existing evidence entry (matched via FindEvidenceByHash) rather than
// registering a second, duplicate entry under the new basename.
func TestCmdAnalyzeReusesExistingEvidenceByHash(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.Cases.Create("t"); err != nil {
		t.Fatal(err)
	}
	c, err := app.Cases.Open("t")
	if err != nil {
		t.Fatal(err)
	}

	data := append([]byte("MZ"), make([]byte, 62)...)

	dir := t.TempDir()
	original := filepath.Join(dir, "original.exe")
	if err := os.WriteFile(original, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.evidenceAdd(c, []string{original}); err != nil {
		t.Fatalf("evidenceAdd(original): %v", err)
	}

	before := len(c.Evidence())

	renamed := filepath.Join(dir, "renamed.exe")
	if err := os.WriteFile(renamed, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := app.cmdAnalyze([]string{renamed}); err != nil {
		t.Fatalf("cmdAnalyze(renamed, same content): %v", err)
	}

	after := len(c.Evidence())
	if after != before {
		t.Errorf("expected no new evidence entry for same-content rename: before=%d after=%d", before, after)
	}
	for _, e := range c.Evidence() {
		if e.Name == "renamed.exe" {
			t.Error("expected no evidence entry named renamed.exe — FindEvidenceByHash reuse should have fired")
		}
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
