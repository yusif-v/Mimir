package mcpserver

import (
	"context"
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
