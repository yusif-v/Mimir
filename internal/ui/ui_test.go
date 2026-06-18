package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderBoxTable(t *testing.T) {
	tbl := Table{
		Headers: []string{"CASE", "TOOLS"},
		Rows:    [][]string{{"incident-42", "3"}, {"malware-7", "12"}},
		Align:   []Align{AlignLeft, AlignRight},
	}
	var b bytes.Buffer
	tbl.Render(&b, 200, false)
	out := b.String()
	if !strings.Contains(out, "┌") || !strings.Contains(out, "└") || !strings.Contains(out, "│") {
		t.Fatalf("expected box-drawing borders, got:\n%s", out)
	}
	if !strings.Contains(out, "CASE") || !strings.Contains(out, "incident-42") {
		t.Fatalf("missing content:\n%s", out)
	}
	// Right-aligned numeric column: "  3" padded to the width of "TOOLS"/"12".
	if !strings.Contains(out, "│     3 │") && !strings.Contains(out, "│    3 │") {
		t.Fatalf("expected right-aligned TOOLS column, got:\n%s", out)
	}
}

func TestRenderPlainFallback(t *testing.T) {
	tbl := Table{
		Headers: []string{"CASE", "TOOLS"},
		Rows:    [][]string{{"incident-42", "3"}},
		Align:   []Align{AlignLeft, AlignRight},
	}
	var b bytes.Buffer
	tbl.Render(&b, 200, true) // plain
	out := b.String()
	if strings.ContainsAny(out, "┌│└") {
		t.Fatalf("plain output must not contain box chars:\n%s", out)
	}
	if !strings.Contains(out, "CASE") || !strings.Contains(out, "incident-42") {
		t.Fatalf("missing content:\n%s", out)
	}
}

func TestRenderEmptyRows(t *testing.T) {
	tbl := Table{Headers: []string{"A"}, Rows: nil, Align: []Align{AlignLeft}}
	var b bytes.Buffer
	tbl.Render(&b, 80, false) // must not panic
	if b.Len() == 0 {
		t.Fatal("expected at least a header row")
	}
}
