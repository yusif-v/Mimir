package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
)

func decodeResult(res *mcp.CallToolResult, out any) error {
	if res.StructuredContent == nil {
		return nil
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func TestSearchCrossCase(t *testing.T) {
	cs, app := newTestServer(t)

	c1, err := app.Cases.Create("incident-42")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := c1.AddNote("beaconing to 10.0.0.9 every 60s", "analyst"); err != nil {
		t.Fatalf("note: %v", err)
	}
	if err := c1.AppendIOC(cases.IOCRecord{Type: "ipv4", Value: "10.0.0.9", Source: "test", Time: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("ioc: %v", err)
	}
	if err := c1.Save(); err != nil {
		t.Fatalf("save c1: %v", err)
	}

	c2, err := app.Cases.Create("phishing-campaign")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := c2.AddNote("qakbot uses a beaconing C2", "analyst"); err != nil {
		t.Fatalf("note: %v", err)
	}
	if err := c2.Save(); err != nil {
		t.Fatalf("save c2: %v", err)
	}
	outDir := filepath.Join(c2.Path, "output")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "strings.txt"), []byte("evil beacon domain\n"), 0644); err != nil {
		t.Fatalf("write output: %v", err)
	}

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "beacon"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	var out searchOut
	if err := decodeResult(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var c1Note, c2Note bool
	for _, h := range out.Hits {
		switch {
		case h.Case == "incident-42" && h.Where == "note":
			c1Note = true
		case h.Case == "phishing-campaign" && h.Where == "note":
			c2Note = true
		}
	}
	if !c1Note || !c2Note {
		t.Fatalf("expected notes in both cases, got hits: %+v", out.Hits)
	}

	res2, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "10.0.0.9"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var out2 searchOut
	if err := decodeResult(res2, &out2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var iocHit, noteHit bool
	for _, h := range out2.Hits {
		switch h.Where {
		case "ioc":
			iocHit = true
		case "note":
			noteHit = true
		}
	}
	if !iocHit || !noteHit {
		t.Fatalf("expected ioc + note hits, got: %+v", out2.Hits)
	}
}

func TestSearchInOutput(t *testing.T) {
	cs, app := newTestServer(t)

	c, err := app.Cases.Create("case-a")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	outDir := filepath.Join(c.Path, "output")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "scan.txt"), []byte("nothing useful here\n"), 0644); err != nil {
		t.Fatalf("write output: %v", err)
	}

	// Without in_output, the only match is the case name itself.
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "useful"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var out searchOut
	if err := decodeResult(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Hits) != 0 {
		t.Fatalf("expected 0 hits without in_output, got %+v", out.Hits)
	}

	// With in_output, the file content match appears.
	res2, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "useful", "in_output": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var out2 searchOut
	if err := decodeResult(res2, &out2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found bool
	for _, h := range out2.Hits {
		if h.Where == "output" && strings.Contains(h.Detail, "scan.txt") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected output hit, got %+v", out2.Hits)
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	cs, _ := newTestServer(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for missing query")
	}
}
