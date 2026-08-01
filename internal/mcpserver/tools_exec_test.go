package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestToolsListIncludesBuiltins(t *testing.T) {
	cs, _ := newTestServer(t)
	r := call(t, cs, "tools_list", map[string]any{})
	if r.IsError {
		t.Fatalf("tools_list: %v", r.Content)
	}
	if r.StructuredContent == nil {
		t.Fatal("expected structured tool list")
	}
}

func TestToolRunIsAnnotatedDangerous(t *testing.T) {
	cs, _ := newTestServer(t)
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tl := range tools.Tools {
		if tl.Name == "tool_run" {
			found = true
			if tl.Annotations == nil || tl.Annotations.DestructiveHint == nil || !*tl.Annotations.DestructiveHint {
				t.Error("tool_run must carry DestructiveHint")
			}
		}
	}
	if !found {
		t.Error("tool_run not registered")
	}
}

// TestToolRunRecordsTimelineEvent guards FIX 1: tool_run must append a
// tool_run timeline event (and tool usage) exactly like the CLI's
// recordRun, so timeline_read/timeline_correlate/report/export see MCP-run
// tools too. Uses the "decode" builtin — no Docker daemon required.
func TestToolRunRecordsTimelineEvent(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "case_create", map[string]any{"name": "inc-record"}); r.IsError {
		t.Fatalf("case_create: %v", r.Content)
	}
	r := call(t, cs, "tool_run", map[string]any{
		"case": "inc-record",
		"tool": "decode",
		"args": []string{"--base64", "aGVsbG8="},
	})
	if r.IsError {
		t.Fatalf("tool_run: %v", r.Content)
	}

	tr := call(t, cs, "timeline_read", map[string]any{"case": "inc-record", "type": "tool_run"})
	if tr.IsError {
		t.Fatalf("timeline_read: %v", tr.Content)
	}
	raw, err := json.Marshal(tr.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out timelineReadOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal timelineReadOut: %v", err)
	}
	if len(out.Events) == 0 {
		t.Fatal("expected a tool_run event in the timeline after tool_run")
	}
	found := false
	for _, ev := range out.Events {
		if ev.Type == "tool_run" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected at least one tool_run-type event")
	}

	cor := call(t, cs, "timeline_correlate", map[string]any{"case": "inc-record"})
	if cor.IsError {
		t.Fatalf("timeline_correlate: %v", cor.Content)
	}
	craw, err := json.Marshal(cor.StructuredContent)
	if err != nil {
		t.Fatalf("marshal correlate structured content: %v", err)
	}
	var corOut timelineCorrelateOut
	if err := json.Unmarshal(craw, &corOut); err != nil {
		t.Fatalf("unmarshal timelineCorrelateOut: %v", err)
	}
	if corOut.TotalRuns != 1 {
		t.Fatalf("timeline_correlate total_runs = %d, want 1", corOut.TotalRuns)
	}
}

func TestToolRunUnknownToolIsError(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "case_create", map[string]any{"name": "inc-exec"}); r.IsError {
		t.Fatalf("case_create: %v", r.Content)
	}
	r := call(t, cs, "tool_run", map[string]any{"case": "inc-exec", "tool": "does-not-exist"})
	if !r.IsError {
		t.Fatal("expected error for unknown tool")
	}
}

// TestToolRunFailureIncludesStderr guards gap 3: a failed tool run must surface
// a stderr snippet (and exit_ok=false) instead of dropping the diagnostic.
func TestToolRunFailureIncludesStderr(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "case_create", map[string]any{"name": "inc-fail"}); r.IsError {
		t.Fatalf("case_create: %v", r.Content)
	}
	r := call(t, cs, "tool_run", map[string]any{
		"case": "inc-fail",
		"tool": "decode",
		"args": []string{"--base64", "!!!"},
	})
	if r.IsError {
		t.Fatalf("tool_run unexpectedly errored at MCP level: %v", r.Content)
	}
	raw, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out toolRunOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal toolRunOut: %v", err)
	}
	if out.ExitOK {
		t.Fatal("expected exit_ok=false for invalid base64 input")
	}
	if !strings.Contains(out.Stderr, "input is not valid base64") {
		t.Fatalf("expected stderr snippet to mention invalid base64, got %q", out.Stderr)
	}
}

func TestStderrSnippetTruncation(t *testing.T) {
	short := "one line"
	if got := stderrSnippet(short); got != short {
		t.Fatalf("stderrSnippet(%q) = %q, want unchanged", short, got)
	}
	long := strings.Repeat("x", stderrSnippetLen+100)
	got := stderrSnippet(long)
	if !strings.HasPrefix(got, "...[truncated]...\n") {
		t.Fatalf("expected truncation marker, got prefix %q", got[:min(20, len(got))])
	}
	if !strings.HasSuffix(got, strings.Repeat("x", stderrSnippetLen)) {
		t.Fatal("expected snippet to keep the tail")
	}
}

func TestToolBuildUnknownToolIsError(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "tool_build", map[string]any{"tool": "does-not-exist"}); !r.IsError {
		t.Fatal("expected error for unknown tool")
	}
}

func TestToolInstallUnknownToolIsError(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "tool_install", map[string]any{"tool": "does-not-exist"}); !r.IsError {
		t.Fatal("expected error for unknown tool")
	}
}
