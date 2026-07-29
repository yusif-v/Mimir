package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIOCExtractFromFile(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-ioc"})
	f := filepath.Join(t.TempDir(), "log.txt")
	os.WriteFile(f, []byte("contact 8.8.8.8 and evil.example.com"), 0o644)
	r := call(t, cs, "ioc_extract", map[string]any{"case": "inc-ioc", "path": f})
	if r.IsError {
		t.Fatalf("ioc_extract: %v", r.Content)
	}
}

func TestIOCExtractFromOutput(t *testing.T) {
	cs, app := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-ioc-out"})
	c, err := app.Cases.Open("inc-ioc-out")
	if err != nil {
		t.Fatalf("open case: %v", err)
	}
	outDir := filepath.Join(c.Path, "output")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "capture.txt"), []byte("beacon to 1.2.3.4"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
	r := call(t, cs, "ioc_extract", map[string]any{"case": "inc-ioc-out", "from_output": "capture.txt"})
	if r.IsError {
		t.Fatalf("ioc_extract --from-output: %v", r.Content)
	}
}

func TestIOCExtractRequiresPathOrFromOutput(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-ioc-none"})
	r := call(t, cs, "ioc_extract", map[string]any{"case": "inc-ioc-none"})
	if !r.IsError {
		t.Fatalf("expected error when neither path nor from_output given")
	}
}

func TestIOCExtractFromOutputRejectsTraversal(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-ioc-trav"})
	for _, bad := range []string{"../../../etc/passwd", "sub/dir/file", "/etc/passwd"} {
		r := call(t, cs, "ioc_extract", map[string]any{"case": "inc-ioc-trav", "from_output": bad})
		if !r.IsError {
			t.Errorf("from_output=%q should be rejected as path traversal", bad)
		}
	}
}

func TestIOCDiff(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-diff"})
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	os.WriteFile(fa, []byte("1.1.1.1 shared.example.com"), 0o644)
	os.WriteFile(fb, []byte("shared.example.com 2.2.2.2"), 0o644)
	r := call(t, cs, "ioc_diff", map[string]any{"case": "inc-diff", "file_a": fa, "file_b": fb})
	if r.IsError {
		t.Fatalf("ioc_diff: %v", r.Content)
	}
}

func TestCorrelate(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-corr"})
	f := filepath.Join(t.TempDir(), "log.txt")
	os.WriteFile(f, []byte("contact 8.8.8.8"), 0o644)
	call(t, cs, "ioc_extract", map[string]any{"case": "inc-corr", "path": f})
	r := call(t, cs, "correlate", map[string]any{"case": "inc-corr"})
	if r.IsError {
		t.Fatalf("correlate: %v", r.Content)
	}
}

func TestCorrelateDot(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-corr-dot"})
	f := filepath.Join(t.TempDir(), "log.txt")
	os.WriteFile(f, []byte("contact 8.8.8.8"), 0o644)
	call(t, cs, "ioc_extract", map[string]any{"case": "inc-corr-dot", "path": f})
	r := call(t, cs, "correlate", map[string]any{"case": "inc-corr-dot", "dot": true})
	if r.IsError {
		t.Fatalf("correlate --dot: %v", r.Content)
	}
	found := false
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if strings.Contains(tc.Text, "digraph correlate") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected digraph output in correlate --dot summary, content: %v", r.Content)
	}
}
