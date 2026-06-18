package shell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
)

func runesToStrings(rs [][]rune) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

func TestMatchSuffixesContract(t *testing.T) {
	got, length := matchSuffixes("g", []string{"go", "git", "git-shell", "grep"})
	if length != 1 {
		t.Fatalf("length = %d, want 1", length)
	}
	want := []string{"o", "it", "it-shell", "rep"}
	gs := runesToStrings(got)
	if len(gs) != len(want) {
		t.Fatalf("got %v, want %v", gs, want)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Errorf("suffix[%d] = %q, want %q", i, gs[i], want[i])
		}
	}
}

func TestMatchSuffixesNoMatch(t *testing.T) {
	got, length := matchSuffixes("zz", []string{"go", "git"})
	if len(got) != 0 || length != 2 {
		t.Fatalf("got %v len %d, want empty len 2", runesToStrings(got), length)
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct{ in, dir, base string }{
		{"evi", "", "evi"},
		{"evidence/", "evidence/", ""},
		{"evidence/foo", "evidence/", "foo"},
		{"a/b/c", "a/b/", "c"},
	}
	for _, c := range tests {
		d, b := splitPath(c.in)
		if d != c.dir || b != c.base {
			t.Errorf("splitPath(%q) = (%q,%q), want (%q,%q)", c.in, d, b, c.dir, c.base)
		}
	}
}

func TestFilePathCompleteDirsGetSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "evidence"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evil.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	got, length := filePathComplete(dir, "ev", os.ReadDir)
	if length != 2 {
		t.Fatalf("length = %d, want 2", length)
	}
	gs := runesToStrings(got)
	// "ev" + suffix reconstructs the full entry; dir gets trailing slash.
	wantSet := map[string]bool{"idence/": true, "il.txt": true}
	if len(gs) != 2 {
		t.Fatalf("got %v, want 2 entries", gs)
	}
	for _, s := range gs {
		if !wantSet[s] {
			t.Errorf("unexpected suffix %q", s)
		}
	}
}

func TestToolNamesIncludeBuiltins(t *testing.T) {
	defs := []*tools.Definition{{Name: "volatility", DockerImage: "x"}}
	bi := []builtins.Meta{{Name: "hash"}, {Name: "strings"}}
	got := toolNames(defs, bi)
	want := map[string]bool{"volatility": true, "hash": true, "strings": true}
	if len(got) != 3 {
		t.Fatalf("got %v, want 3", got)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected tool name %q", n)
		}
	}
}

func TestInstallNamesExcludeInstalled(t *testing.T) {
	installed := []*tools.Definition{{Name: "volatility"}}
	entries := []catalog.Entry{{Name: "volatility"}, {Name: "yara"}}
	got := installNames(installed, entries)
	if len(got) != 1 || got[0] != "yara" {
		t.Fatalf("got %v, want [yara]", got)
	}
}

func TestBuildNamesOnlyDocker(t *testing.T) {
	installed := []*tools.Definition{
		{Name: "volatility", DockerImage: "img"},
		{Name: "localtool", LocalCmd: "lt"},
	}
	got := buildNames(installed)
	if len(got) != 1 || got[0] != "volatility" {
		t.Fatalf("got %v, want [volatility]", got)
	}
}

func TestCaseNames(t *testing.T) {
	cs := []*cases.Case{{Name: "alpha"}, {Name: "beta"}}
	got := caseNames(cs)
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("got %v, want [alpha beta]", got)
	}
}

func newTestSources() sources {
	return sources{
		commands:     commandNames,
		toolNames:    []string{"hash", "volatility"},
		installNames: []string{"yara"},
		buildNames:   []string{"volatility"},
		caseNames:    []string{"alpha", "beta"},
		filePath: func(word string) ([][]rune, int) {
			return [][]rune{[]rune("FILE")}, len([]rune(word))
		},
	}
}

func TestCompleteCommandContext(t *testing.T) {
	got, _ := complete("ru", newTestSources())
	gs := runesToStrings(got)
	if len(gs) != 1 || gs[0] != "n" { // "ru" + "n" = "run"
		t.Fatalf("got %v, want [n]", gs)
	}
}

func TestCompleteRunTool(t *testing.T) {
	got, length := complete("run vol", newTestSources())
	gs := runesToStrings(got)
	if length != 3 || len(gs) != 1 || gs[0] != "atility" {
		t.Fatalf("got %v len %d, want [atility] len 3", gs, length)
	}
}

func TestCompleteRunArgIsFilePath(t *testing.T) {
	got, _ := complete("run hash ev", newTestSources())
	gs := runesToStrings(got)
	if len(gs) != 1 || gs[0] != "FILE" {
		t.Fatalf("got %v, want [FILE] (file-path source)", gs)
	}
}

func TestCompleteCaseFlagThenName(t *testing.T) {
	flags, _ := complete("case -", newTestSources())
	if len(runesToStrings(flags)) != 3 { // -n -o -c
		t.Fatalf("flags = %v, want 3", runesToStrings(flags))
	}
	names, _ := complete("case -o al", newTestSources())
	gs := runesToStrings(names)
	if len(gs) != 1 || gs[0] != "pha" {
		t.Fatalf("got %v, want [pha]", gs)
	}
}

func TestCompleteUnknownCommandIsFilePath(t *testing.T) {
	got, _ := complete("ls ev", newTestSources())
	gs := runesToStrings(got)
	if len(gs) != 1 || gs[0] != "FILE" {
		t.Fatalf("got %v, want [FILE] for passthrough arg", gs)
	}
}

func TestCompleteEmptyLineReturnsCommands(t *testing.T) {
	got, length := complete("", newTestSources())
	if length != 0 {
		t.Fatalf("length = %d, want 0", length)
	}
	if len(got) != len(commandNames) {
		t.Fatalf("got %d candidates, want %d (all commands)", len(got), len(commandNames))
	}
}

func TestCompleteWhitespaceLineNoPanic(t *testing.T) {
	for _, line := range []string{"\t", "   "} {
		got, _ := complete(line, newTestSources())
		if len(got) == 0 {
			t.Errorf("complete(%q): expected command candidates, got none", line)
		}
	}
}

func TestCompleterDoCompletesCommand(t *testing.T) {
	bus := events.NewBus()
	app := &App{
		Tools: tools.NewRegistry(bus),
		Cases: cases.NewManager(t.TempDir(), bus),
	}
	c := &Completer{app: app}
	got, length := c.Do([]rune("ti"), 2)
	gs := runesToStrings(got)
	if length != 2 || len(gs) != 1 || gs[0] != "meline" { // "ti" + "meline"
		t.Fatalf("got %v len %d, want [meline] len 2", gs, length)
	}
}

func TestCompleterDoCompletesRegisteredTool(t *testing.T) {
	bus := events.NewBus()
	reg := tools.NewRegistry(bus)
	reg.Register(&tools.Definition{Name: "volatility", DockerImage: "img"})
	app := &App{Tools: reg, Cases: cases.NewManager(t.TempDir(), bus)}
	c := &Completer{app: app}
	got, _ := c.Do([]rune("run vol"), 7)
	gs := runesToStrings(got)
	if len(gs) != 1 || gs[0] != "atility" {
		t.Fatalf("got %v, want [atility]", gs)
	}
}

func TestMatchSuffixesMultibyte(t *testing.T) {
	got, length := matchSuffixes("日", []string{"日本語", "日記", "x"})
	if length != 1 {
		t.Fatalf("length = %d, want 1 (one rune)", length)
	}
	gs := runesToStrings(got)
	want := []string{"本語", "記"}
	if len(gs) != len(want) {
		t.Fatalf("got %v, want %v", gs, want)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Errorf("suffix[%d] = %q, want %q", i, gs[i], want[i])
		}
	}
}
