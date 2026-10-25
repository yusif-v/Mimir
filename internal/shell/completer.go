package shell

import (
	"os"
	"sort"
	"strings"

	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/tools"
)

// Completer is the readline AutoCompleter; it assembles live candidate sources
// from the App on each pass and delegates to the pure complete().
type Completer struct {
	app *App
}

func (c *Completer) Do(line []rune, pos int) ([][]rune, int) {
	cwd, _ := os.Getwd()
	installed := c.app.Tools.List("")
	catEntries, _ := catalog.List()
	names, _ := c.app.Cases.Names()

	s := sources{
		commands:     commandNames,
		subcommands:  subcommands,
		toolNames:    toolNames(installed, builtins.List()),
		installNames: installNames(installed, catEntries),
		buildNames:   buildNames(installed),
		caseNames:    names,
		diskTools:    diskToolNames(),
		filePath: func(word string) ([][]rune, int) {
			return filePathComplete(cwd, word, os.ReadDir)
		},
	}
	return complete(string(line[:pos]), s)
}

// commandNames are the REPL's canonical command domains, completed at word 0.
var commandNames = []string{
	"help", "exit", "case", "evidence", "ioc", "timeline", "tool",
	"memory", "disk", "registry", "report", "export", "import", "note",
	"correlate", "search", "grep", "plugin", "theme", "sandbox", "ai", "clear",
}

// subcommands maps each domain to the actions it accepts at word 1. Domains
// without a fixed action list (memory, disk, registry, report, ...) are absent.
var subcommands = map[string][]string{
	"case":     {"create", "open", "close", "list", "status", "templates"},
	"evidence": {"list", "add", "tag", "verify", "classify"},
	"ioc":      {"list", "extract", "from-output", "diff"},
	"timeline": {"list", "export", "build", "correlate"},
	"tool":     {"list", "run", "install", "build", "info", "analyze", "dashboard"},
	"plugin":   {"list", "run", "info"},
	"theme":    {"list", "set", "save"},
	"sandbox":  {"create", "run", "shell", "logs", "report", "cp", "destroy", "list"},
	"ai":       {"ask", "analyze", "suggest", "explain", "summarize", "config", "models", "use"},
}

// diskToolNames returns the SleuthKit binaries the disk command accepts.
func diskToolNames() []string {
	out := make([]string, 0, len(tskTools))
	for name := range tskTools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
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

// sources holds the resolved candidate lists for one completion pass, plus a
// file-path closure (injected so the pure logic needs no real filesystem/TTY).
type sources struct {
	commands     []string
	subcommands  map[string][]string
	toolNames    []string
	installNames []string
	buildNames   []string
	caseNames    []string
	diskTools    []string
	filePath     func(word string) ([][]rune, int)
}

// complete parses the line and returns completions for the word at the cursor,
// completing domain actions first and then action-specific arguments.
func complete(line string, s sources) ([][]rune, int) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return matchSuffixes("", s.commands)
	}
	// Past the guard fields is non-empty, so an empty line can't reach here;
	// a trailing space means the cursor starts a fresh (empty) word.
	trailing := strings.HasSuffix(line, " ")

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

	if actions, ok := s.subcommands[completed[0]]; ok && pos == 1 {
		return matchSuffixes(word, actions)
	}

	switch completed[0] {
	case "case":
		if pos == 2 && (completed[1] == "open" || completed[1] == "create") {
			return matchSuffixes(word, s.caseNames)
		}
		if pos == 2 && completed[1] == "list" {
			return matchSuffixes(word, []string{"--status"})
		}
		return nil, len([]rune(word))
	case "tool":
		if pos == 2 {
			switch completed[1] {
			case "run", "info":
				return matchSuffixes(word, s.toolNames)
			case "install":
				return matchSuffixes(word, s.installNames)
			case "build":
				return matchSuffixes(word, s.buildNames)
			case "analyze":
				return s.filePath(word)
			}
			return nil, len([]rune(word))
		}
		if pos == 3 && completed[1] == "run" {
			return s.filePath(word)
		}
		return nil, len([]rune(word))
	case "ioc":
		if pos == 2 {
			switch completed[1] {
			case "extract", "diff":
				return s.filePath(word)
			case "from-output":
				return matchSuffixes(word, s.toolNames)
			}
		}
		return nil, len([]rune(word))
	case "evidence":
		if pos == 2 && completed[1] == "add" {
			return s.filePath(word)
		}
		return nil, len([]rune(word))
	case "timeline":
		if pos == 2 && completed[1] == "list" {
			return matchSuffixes(word, []string{"-n", "--type", "--grep"})
		}
		return nil, len([]rune(word))
	case "disk":
		if pos == 1 {
			return matchSuffixes(word, s.diskTools)
		}
		return s.filePath(word)
	case "memory", "registry":
		return s.filePath(word)
	default:
		return s.filePath(word)
	}
}
