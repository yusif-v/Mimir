package mcpserver

import (
	"context"
	"encoding/json"
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
	// StructuredContent carries the typed Out; decode it and assert the
	// note event actually made it into the timeline (not just that some
	// structured payload was returned — a regression that drops event
	// recording but still returns an empty Events slice must fail here).
	if r.StructuredContent == nil {
		t.Fatal("expected structured timeline output")
	}
	raw, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out timelineReadOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal timelineReadOut: %v", err)
	}
	if len(out.Events) == 0 {
		t.Fatal("expected at least one timeline event")
	}
	var sawNote bool
	for _, ev := range out.Events {
		if ev.Type == "note" {
			sawNote = true
			if ev.Text != "suspicious binary" {
				t.Errorf("note event text = %q, want %q", ev.Text, "suspicious binary")
			}
		}
	}
	if !sawNote {
		t.Fatal("expected a note-type event in the timeline")
	}
}

// TestMCPServerDoesNotReopenCaseOnEveryCall guards against the bug where the
// MCP server called Cases.Open() (which appends a "case_opened" event) on
// every tool invocation, so a case that was used for several tool calls ended
// up with a duplicated, misleading "case opened" timeline event per call.
// The server must load the case without side effects instead.
func TestMCPServerDoesNotReopenCaseOnEveryCall(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "case_create", map[string]any{"name": "inc-no-reopen"}); r.IsError {
		t.Fatalf("case_create error: %v", r.Content)
	}
	// Several read/write tool calls against the same case.
	for i := 0; i < 3; i++ {
		if r := call(t, cs, "note_add", map[string]any{"case": "inc-no-reopen", "text": "note"}); r.IsError {
			t.Fatalf("note_add #%d error: %v", i, r.Content)
		}
	}
	r := call(t, cs, "timeline_read", map[string]any{"case": "inc-no-reopen"})
	if r.IsError {
		t.Fatalf("timeline_read error: %v", r.Content)
	}
	raw, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out timelineReadOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal timelineReadOut: %v", err)
	}
	opened := 0
	for _, ev := range out.Events {
		if ev.Type == "case_opened" {
			opened++
		}
	}
	if opened != 0 {
		t.Errorf("expected zero case_opened events (the server must not reopen the case per call), got %d", opened)
	}
}
