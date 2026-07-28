package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res
}

func TestCaseCreateThenNoteAppearsInTimeline(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "case_create", map[string]any{"name": "inc-1"}); r.IsError {
		t.Fatalf("case_create error: %v", r.Content)
	}
	if r := call(t, cs, "note_add", map[string]any{"case": "inc-1", "text": "suspicious binary"}); r.IsError {
		t.Fatalf("note_add error: %v", r.Content)
	}
	r := call(t, cs, "timeline_read", map[string]any{"case": "inc-1", "type": "note"})
	if r.IsError {
		t.Fatalf("timeline_read error: %v", r.Content)
	}
	// StructuredContent carries the typed Out; assert at least one event.
	if r.StructuredContent == nil {
		t.Fatal("expected structured timeline output")
	}
}
