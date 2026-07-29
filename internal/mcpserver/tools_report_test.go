package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReportForCase(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-r"})
	call(t, cs, "note_add", map[string]any{"case": "inc-r", "text": "finding"})
	r := call(t, cs, "report", map[string]any{"case": "inc-r"})
	if r.IsError {
		t.Fatalf("report: %v", r.Content)
	}
}

func TestExportForCase(t *testing.T) {
	cs, app := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-e"})
	call(t, cs, "note_add", map[string]any{"case": "inc-e", "text": "note1"})

	c, err := app.Cases.Open("inc-e")
	if err != nil {
		t.Fatalf("open case: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "timeline.json")

	r := call(t, cs, "export", map[string]any{
		"case":   "inc-e",
		"format": "json",
		"path":   outPath,
	})
	if r.IsError {
		t.Fatalf("export: %v", r.Content)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected exported file: %v", err)
	}
	_ = c
}

func TestExportProvenance(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-p"})
	call(t, cs, "note_add", map[string]any{"case": "inc-p", "text": "note1"})

	outPath := filepath.Join(t.TempDir(), "timeline.csv")
	r := call(t, cs, "export", map[string]any{
		"case":       "inc-p",
		"format":     "csv",
		"path":       outPath,
		"provenance": true,
	})
	if r.IsError {
		t.Fatalf("export: %v", r.Content)
	}
	manPath := outPath + ".manifest.json"
	b, err := os.ReadFile(manPath)
	if err != nil {
		t.Fatalf("expected manifest file: %v", err)
	}
	var man map[string]any
	if err := json.Unmarshal(b, &man); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if man["exported_file_sha256"] == "" || man["exported_file_sha256"] == nil {
		t.Fatalf("expected exported_file_sha256 in manifest: %v", man)
	}
}
