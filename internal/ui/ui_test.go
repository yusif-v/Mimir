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

func TestRenderDegradesWhenWidthTooSmall(t *testing.T) {
	tbl := Table{
		Headers: []string{"CASE", "TOOLS"},
		Rows:    [][]string{{"incident-42", "3"}},
		Align:   []Align{AlignLeft, AlignRight},
	}
	var b bytes.Buffer
	// plain=false, but width far too small for the box form → must degrade to plain.
	tbl.Render(&b, 5, false)
	out := b.String()
	if strings.ContainsAny(out, "┌│└") {
		t.Fatalf("narrow width must degrade to plain (no box chars):\n%s", out)
	}
	if !strings.Contains(out, "incident-42") {
		t.Fatalf("content missing after degradation:\n%s", out)
	}
}

func TestRenderDoubleWidthCells(t *testing.T) {
	// Status-icon glyphs (●, ○) are double-width — padding must use display
	// width, not byte length, or the box borders go ragged.
	tbl := Table{
		Headers: []string{"CASE", "STATUS"},
		Rows: [][]string{
			{"incident-42", "● open"},
			{"malware-7", "○ closed"},
		},
		Align: []Align{AlignLeft, AlignLeft},
	}
	var b bytes.Buffer
	tbl.Render(&b, 200, false)
	out := b.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// Collect the column-separator position from the header row (line index 1).
	sepPos := -1
	for _, line := range lines {
		if !strings.HasPrefix(line, "│") {
			continue
		}
		plain := stripANSI(line)
		start := strings.Index(plain, "│")
		end := strings.LastIndex(plain, "│")
		if start < 0 || end <= start {
			continue
		}
		inner := plain[start+1 : end]
		sep := strings.Index(inner, "│")
		if sep < 0 {
			continue
		}
		if sepPos == -1 {
			sepPos = sep
		} else if sep != sepPos {
			t.Fatalf("column separator misaligned: row has %d, first row had %d\nfull output:\n%s", sep, sepPos, out)
		}
	}
	if sepPos < 0 {
		t.Fatal("could not find any data rows in output")
	}
}

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
