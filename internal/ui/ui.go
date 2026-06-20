// Package ui renders tabular command output as box tables, degrading to plain
// columns when output is piped or the terminal is too narrow.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
	"golang.org/x/text/width"
)

type Align int

const (
	AlignLeft Align = iota
	AlignRight
)

// Table is a renderable table. Align may be shorter than the column count
// (missing entries default to AlignLeft).
type Table struct {
	Headers []string
	Rows    [][]string
	Align   []Align
}

// IsTTY reports whether f is a terminal.
func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// TermWidth returns the terminal width of f, or 0 if unknown.
func TermWidth(f *os.File) int {
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}

// Colorize wraps s in the ANSI color unless NO_COLOR is set.
func Colorize(s, color string) string {
	if os.Getenv("NO_COLOR") != "" || color == "" {
		return s
	}
	return color + s + "\033[0m"
}

func (t Table) align(col int) Align {
	if col < len(t.Align) {
		return t.Align[col]
	}
	return AlignLeft
}

// colWidths returns the max display width per column across headers + rows.
func (t Table) colWidths() []int {
	n := len(t.Headers)
	w := make([]int, n)
	for i, h := range t.Headers {
		w[i] = displayWidth(h)
	}
	for _, row := range t.Rows {
		for i := 0; i < n && i < len(row); i++ {
			if displayWidth(row[i]) > w[i] {
				w[i] = displayWidth(row[i])
			}
		}
	}
	return w
}

// displayWidth returns the monospace column width of s, ignoring ANSI
// escape sequences (handled separately via Colorize).
func displayWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		// ANSI escape sequences: ESC + [ + ... + m — zero width.
		if r == 0x1b {
			continue
		}
		// Zero-width combinators and similar.
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			w += 2
		default:
			// EastAsianNarrow, EastAsianHalfwidth, EastAsianAmbiguous, Neutral — all 1 column.
			w += 1
		}
	}
	return w
}

// pad pads s to fill the given display width, respecting alignment.
func pad(s string, width int, a Align) string {
	cur := displayWidth(s)
	gap := width - cur
	if gap <= 0 {
		return s
	}
	if a == AlignRight {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// Render writes the table to w. When plain is true (or width is too small for
// the box form), it emits space-aligned columns without borders.
func (t Table) Render(w io.Writer, width int, plain bool) {
	cw := t.colWidths()

	// Total box width = sum(cols) + 3 per column + 1.
	boxWidth := 1
	for _, c := range cw {
		boxWidth += c + 3
	}
	if width > 0 && boxWidth > width {
		plain = true
	}

	if plain {
		t.renderPlain(w, cw)
		return
	}

	border := func(l, m, r string) {
		var sb strings.Builder
		sb.WriteString(l)
		for i, c := range cw {
			sb.WriteString(strings.Repeat("─", c+2))
			if i < len(cw)-1 {
				sb.WriteString(m)
			}
		}
		sb.WriteString(r)
		fmt.Fprintln(w, sb.String())
	}
	rowLine := func(cells []string) {
		var sb strings.Builder
		sb.WriteString("│")
		for i := range cw {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			sb.WriteString(" " + pad(cell, cw[i], t.align(i)) + " │")
		}
		fmt.Fprintln(w, sb.String())
	}

	border("┌", "┬", "┐")
	rowLine(t.Headers)
	border("├", "┼", "┤")
	for _, row := range t.Rows {
		rowLine(row)
	}
	border("└", "┴", "┘")
}

func (t Table) renderPlain(w io.Writer, cw []int) {
	line := func(cells []string) {
		var parts []string
		for i := range cw {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			parts = append(parts, pad(cell, cw[i], t.align(i)))
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(parts, "  "), " "))
	}
	line(t.Headers)
	for _, row := range t.Rows {
		line(row)
	}
}
