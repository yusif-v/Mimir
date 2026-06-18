package shell

import (
	"os"
	"sort"
	"strings"

	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/tools"
)

// commandNames are the REPL's built-in command verbs, completed at word 0.
var commandNames = []string{
	"help", "exit", "quit", "status", "case", "cases", "tools",
	"run", "install", "build", "use", "note", "clear", "timeline",
}

// matchSuffixes implements the readline AutoCompleter.Do contract: for each
// candidate that starts with word, return the suffix after word; the second
// return is the rune length of word (the shared prefix readline will replace).
func matchSuffixes(word string, candidates []string) ([][]rune, int) {
	wr := []rune(word)
	var out [][]rune
	for _, c := range candidates {
		if strings.HasPrefix(c, word) {
			out = append(out, []rune(c)[len(wr):])
		}
	}
	return out, len(wr)
}

// splitPath divides a path token into its directory part (with trailing slash,
// or "") and the trailing base segment being completed.
func splitPath(word string) (dir, base string) {
	i := strings.LastIndex(word, "/")
	if i < 0 {
		return "", word
	}
	return word[:i+1], word[i+1:]
}

// filePathComplete completes the trailing path segment of word against the
// directory it names, relative to cwd. Directory entries get a trailing slash.
func filePathComplete(cwd, word string, readDir func(string) ([]os.DirEntry, error)) ([][]rune, int) {
	dirPart, basePart := splitPath(word)
	entries, err := readDir(joinPath(cwd, dirPart))
	if err != nil {
		return nil, len([]rune(basePart))
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() {
			n += "/"
		}
		names = append(names, n)
	}
	return matchSuffixes(basePart, names)
}

// joinPath joins cwd and a relative dir part without importing filepath at call
// sites; kept tiny and explicit so completion stays predictable.
func joinPath(cwd, dirPart string) string {
	if dirPart == "" {
		return cwd
	}
	if strings.HasPrefix(dirPart, "/") {
		return dirPart
	}
	return strings.TrimRight(cwd, "/") + "/" + dirPart
}

func toolNames(defs []*tools.Definition, bi []builtins.Meta) []string {
	out := make([]string, 0, len(defs)+len(bi))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	for _, m := range bi {
		out = append(out, m.Name)
	}
	sort.Strings(out)
	return out
}

func installNames(installed []*tools.Definition, entries []catalog.Entry) []string {
	have := map[string]bool{}
	for _, d := range installed {
		have[d.Name] = true
	}
	var out []string
	for _, e := range entries {
		if !have[e.Name] {
			out = append(out, e.Name)
		}
	}
	sort.Strings(out)
	return out
}

func buildNames(installed []*tools.Definition) []string {
	var out []string
	for _, d := range installed {
		if d.RunsInDocker() {
			out = append(out, d.Name)
		}
	}
	sort.Strings(out)
	return out
}

func caseNames(cs []*cases.Case) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

// sources holds the resolved candidate lists for one completion pass, plus a
// file-path closure (injected so the pure logic needs no real filesystem/TTY).
type sources struct {
	commands     []string
	toolNames    []string
	installNames []string
	buildNames   []string
	caseNames    []string
	filePath     func(word string) ([][]rune, int)
}

// complete parses the line and returns completions for the word at the cursor,
// per the context rules in the v0.3 spec.
func complete(line string, s sources) ([][]rune, int) {
	fields := strings.Fields(line)
	trailing := line == "" || strings.HasSuffix(line, " ")

	var completed []string
	var word string
	if trailing {
		completed = fields
		word = ""
	} else {
		completed = fields[:len(fields)-1]
		word = fields[len(fields)-1]
	}
	pos := len(completed)

	if pos == 0 {
		return matchSuffixes(word, s.commands)
	}

	switch completed[0] {
	case "run", "use":
		if pos == 1 {
			return matchSuffixes(word, s.toolNames)
		}
		return s.filePath(word)
	case "install":
		if pos == 1 {
			return matchSuffixes(word, s.installNames)
		}
		return nil, len([]rune(word))
	case "build":
		if pos == 1 {
			return matchSuffixes(word, s.buildNames)
		}
		return nil, len([]rune(word))
	case "case":
		if pos == 1 {
			return matchSuffixes(word, []string{"-n", "-o", "-c"})
		}
		if pos == 2 && (completed[1] == "-o" || completed[1] == "-c") {
			return matchSuffixes(word, s.caseNames)
		}
		return nil, len([]rune(word))
	case "timeline":
		if pos == 1 {
			return matchSuffixes(word, []string{"-n"})
		}
		return nil, len([]rune(word))
	default:
		return s.filePath(word)
	}
}
