package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceAddThenList(t *testing.T) {
	cs, app := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-e"})
	f := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(f, []byte("MZmalware"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := call(t, cs, "evidence_add", map[string]any{"case": "inc-e", "path": f, "tags": []any{"malware"}}); r.IsError {
		t.Fatalf("evidence_add: %v", r.Content)
	}
	r := call(t, cs, "evidence_list", map[string]any{"case": "inc-e"})
	if r.IsError {
		t.Fatalf("evidence_list: %v", r.Content)
	}
	_ = app
}
