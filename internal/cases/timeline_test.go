package cases

import (
	"os"
	"path/filepath"
	"testing"
)

func newTempCase(t *testing.T) *Case {
	t.Helper()
	dir := t.TempDir()
	c := NewCase("t", dir)
	if err := c.Scaffold(); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	return c
}

func TestAppendAndReloadTimeline(t *testing.T) {
	c := newTempCase(t)
	for i := 0; i < 3; i++ {
		ev := TimelineEvent{Type: "note", Timestamp: "2026-06-18T00:00:0" + string(rune('0'+i)) + "Z",
			Payload: map[string]any{"content": "n"}}
		if err := c.AppendEvent(ev); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if got := len(c.Timeline()); got != 3 {
		t.Fatalf("in-memory cache = %d, want 3", got)
	}

	reloaded, err := LoadCase(c.Path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := len(reloaded.Timeline()); got != 3 {
		t.Fatalf("reloaded timeline = %d, want 3", got)
	}
	if reloaded.Timeline()[0].Type != "note" {
		t.Fatalf("type = %q, want note", reloaded.Timeline()[0].Type)
	}
}

func TestLoadTimelineSkipsCorruptLines(t *testing.T) {
	c := newTempCase(t)
	if err := c.AppendEvent(TimelineEvent{Type: "note", Timestamp: "x", Payload: map[string]any{}}); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Corrupt the file by appending a bad line.
	f, _ := os.OpenFile(filepath.Join(c.Path, "timeline.jsonl"), os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("{not valid json\n")
	f.Close()

	reloaded, err := LoadCase(c.Path)
	if err != nil {
		t.Fatalf("reload should tolerate corrupt line: %v", err)
	}
	if got := len(reloaded.Timeline()); got != 1 {
		t.Fatalf("timeline = %d, want 1 (bad line skipped)", got)
	}
}

func TestLoadTimelineMissingFileIsEmpty(t *testing.T) {
	c := newTempCase(t) // no events appended, no timeline.jsonl
	reloaded, err := LoadCase(c.Path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := len(reloaded.Timeline()); got != 0 {
		t.Fatalf("timeline = %d, want 0", got)
	}
}

func TestAddNoteAppendsTimelineEvent(t *testing.T) {
	c := newTempCase(t)
	if err := c.AddNote("found a thing", "analyst"); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if len(c.Notes) != 1 {
		t.Fatalf("Notes = %d, want 1", len(c.Notes))
	}
	tl := c.Timeline()
	if len(tl) != 1 || tl[0].Type != "note" {
		t.Fatalf("timeline = %+v, want one note event", tl)
	}
	if tl[0].Payload["content"] != "found a thing" {
		t.Fatalf("note content = %v", tl[0].Payload["content"])
	}
}
