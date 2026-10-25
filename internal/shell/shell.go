// Package shell provides the interactive REPL.
package shell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/yusif-v/mimir/internal/ai"
	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/casearchive"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/ioc"
	"github.com/yusif-v/mimir/internal/plugins"
	"github.com/yusif-v/mimir/internal/sandbox"
	"github.com/yusif-v/mimir/internal/theme"
	"github.com/yusif-v/mimir/internal/tools"
	"github.com/yusif-v/mimir/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// reIP matches an IPv4 address (used by networkHints for zeek conn logs).
var reIP = regexp.MustCompile(`\b(?:25[0-5]|2[0-4]\d|1?\d?\d)\.(?:25[0-5]|2[0-4]\d|1?\d?\d)\.(?:25[0-5]|2[0-4]\d|1?\d?\d)\.(?:25[0-5]|2[0-4]\d|1?\d?\d)\b`)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorBlue   = "\033[34m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
)

// Version is the Mimir release version, shown in the startup banner.
const Version = "0.8.4"

func banner() string {
	return fmt.Sprintf("%sMimir v%s%s — DFIR shell. Type '%shelp%s' for commands, '%sexit%s' to quit.",
		colorCyan, Version, colorReset, colorGreen, colorReset, colorGreen, colorReset)
}

// App ties together all subsystems.
type App struct {
	Config  *config.Config
	Events  *events.Bus
	Cases   *cases.Manager
	Tools   *tools.Registry
	Runner  *tools.Runner
	Output  *tools.OutputCapture
	Plugins *plugins.Manager
	Theme   theme.Theme
	AI      *ai.Shell
	rl      *readline.Instance

	SandboxManager *sandbox.Manager
	SandboxRuntime *sandbox.Runtime
}

// NewApp creates a new shell app with all subsystems wired.
func NewApp(cfg *config.Config) *App {
	bus := events.NewBus()
	sandboxManager := sandbox.NewManager(filepath.Join(cfg.CasesPath, "sandboxes"))
	sandboxRuntime := sandbox.NewRuntime()
	caseManager := cases.NewManager(cfg.CasesPath, bus, sandboxManager, sandboxRuntime)
	toolRegistry := tools.NewRegistry(bus)
	toolRunner := tools.NewRunner(bus)
	outputCapture := tools.NewOutputCapture(bus)

	// Discover tools from the tools directory
	if err := toolRegistry.DiscoverFromPath(cfg.ToolsPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: tool discovery failed: %v\n", err)
	}

	// Initialize AI shell
	var aiShell *ai.Shell
	if cfgErr := cfg.AI.Validate(); cfgErr == nil {
		if s, err := ai.NewShell(cfg.AI); err == nil {
			aiShell = s
		} else {
			fmt.Fprintf(os.Stderr, "warning: AI init failed: %v\n", err)
		}
	}

	// Initialize plugin manager
	pluginManager := plugins.NewManager(bus)
	pluginManager.Discover()

	return &App{
		Config:         cfg,
		Events:         bus,
		Cases:          caseManager,
		Tools:          toolRegistry,
		Runner:         toolRunner,
		Output:         outputCapture,
		Plugins:        pluginManager,
		Theme:          theme.DefaultTheme(),
		AI:             aiShell,
		SandboxManager: sandboxManager,
		SandboxRuntime: sandboxRuntime,
	}
}

// Run starts the interactive REPL.
func (a *App) Run() error {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "",
		HistoryFile:       a.Config.HistoryPath,
		HistoryLimit:      1000,
		HistorySearchFold: true,
		AutoComplete:      &Completer{app: a},
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
	})
	if err != nil {
		return fmt.Errorf("init readline: %w", err)
	}
	a.rl = rl
	defer a.rl.Close()

	fmt.Println(banner())

	for {
		fmt.Println()
		fmt.Println(a.contextLine())
		rl.SetPrompt(a.promptMarker())
		line, err := a.rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			fmt.Println()
			continue // Ctrl+C cancels the current line; never exits
		}
		if errors.Is(err, io.EOF) {
			fmt.Println("Exiting Mimir...")
			return nil
		}
		if err != nil {
			return fmt.Errorf("readline: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if derr := a.dispatch(line); derr != nil {
			if derr.Error() == "exit" {
				fmt.Println("Exiting Mimir...")
				return nil
			}
			fmt.Fprintf(os.Stderr, "%serror:%s %v\n", colorRed, colorReset, derr)
		}
	}
}

// asciiMode is true when the user opted out of icons/color for the prompt.
func asciiMode() bool {
	return os.Getenv("MIMIR_ASCII") != "" || os.Getenv("NO_COLOR") != ""
}

// colorize wraps s in the given ANSI color (if not empty) and appends a reset.
func colorize(s, color string) string {
	if color == "" || os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color + s + "\033[0m"
}

// user_Current returns the current OS username.
func user_Current() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// contextLine renders the bracketed status line (without the prompt marker).
func (a *App) contextLine() string {
	u := "user"
	if name, err := user_Current(); err == nil && name != "" {
		u = name
	}
	t := a.Theme
	if t.Name == "" {
		t = theme.DefaultTheme()
	}

	userCol := theme.ResolveColor(t.Colors.User)
	brandCol := theme.ResolveColor(t.Colors.Brand)
	caseCol := theme.ResolveColor(t.Colors.Case)
	statusOKCol := theme.ResolveColor(t.Colors.StatusOK)
	statusErrCol := theme.ResolveColor(t.Colors.StatusErr)

	var parts []string
	for _, seg := range t.Segments {
		switch seg.Type {
		case "user":
			parts = append(parts, colorize(fmt.Sprintf("[%s]", u), userCol))
		case "brand":
			text := seg.Text
			if text == "" {
				text = "Mimir"
			}
			parts = append(parts, colorize(" "+text, brandCol))
		case "case":
			if c := a.Cases.Current(); c != nil {
				parts = append(parts, colorize(fmt.Sprintf(" [%s]", c.Name), caseCol))
			}
		case "status":
			if c := a.Cases.Current(); c != nil {
				status := "open"
				scol := statusOKCol
				if c.Status != "open" {
					status, scol = "closed", statusErrCol
				}
				parts = append(parts, colorize(fmt.Sprintf(" [%s]", status), scol))
			}
		case "custom":
			parts = append(parts, colorize(seg.Text, string(seg.Color)))
		}
	}

	sep := t.Layout.Separator
	if sep == "" {
		sep = " "
	}
	return strings.Join(parts, sep)
}

// promptMarker returns just the input prompt marker (e.g. "❯ ").
// IMPORTANT: this must be a single line — readline treats "\n" in prompts
// as multi-line prompt data and re-renders it unpredictably (duplicate
// status lines, cursor corruption). The status line is printed separately
// via fmt.Println(contextLine()) in the REPL loop.
func (a *App) promptMarker() string {
	t := a.Theme
	marker := t.Layout.Marker
	if marker == "" {
		marker = "❯"
	}

	if asciiMode() || t.Name == "minimal" || t.Name == "no-color" {
		return "> "
	}
	return colorize(marker, theme.ResolveColor(t.Colors.Brand)) + " "
}

func (a *App) dispatch(line string) error {
	parts := splitArgs(line)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "exit", "quit":
		return fmt.Errorf("exit")
	case "help":
		return a.cmdHelp(args)
	case "status":
		return a.cmdStatus(args)
	case "case":
		return a.cmdCase(args)
	case "cases":
		return a.cmdCases(args)
	case "tools":
		return a.cmdTools(args)
	case "run":
		return a.cmdRun(args)
	case "analyze":
		return a.cmdAnalyze(args)
	case "install":
		return a.cmdInstall(args)
	case "build":
		return a.cmdBuild(args)
	case "use":
		return a.cmdUse(args)
	case "note":
		return a.cmdNote(args)
	case "evidence", "ev":
		return a.cmdEvidence(args)
	case "timeline":
		return a.cmdTimeline(args)
	case "ioc":
		return a.cmdIOC(args)
	case "correlate":
		return a.cmdCorrelate(args)
	case "clear":
		return a.cmdClear(args)
	case "search":
		return a.cmdSearch(args)
	case "grep":
		return a.cmdGrep(args)
	case "export":
		return a.cmdExport(args)
	case "import":
		return a.cmdImport(args)
	case "ai":
		return a.cmdAI(args)
	case "plugin":
		return a.cmdPlugin(args)
	case "theme":
		return a.cmdTheme(args)
	case "sandbox":
		return a.cmdSandbox(args)
	case "sleuthkit":
		return a.cmdDisk(args)
	case "regripper":
		return a.cmdRegHives(args)
	case "report":
		return a.cmdReport(args)
	case "volatility", "mem":
		return a.cmdVolatility(args)
	case "dashboard", "dash":
		return a.cmdDashboard(args)
	default:
		return a.cmdShell(line)
	}
}

func (a *App) cmdHelp(args []string) error {
	fmt.Printf("  %shelp%s       show this help\n", colorGreen, colorReset)
	fmt.Printf("  %sexit%s       exit Mimir\n", colorGreen, colorReset)
	fmt.Printf("  %sstatus%s     show current case status\n", colorGreen, colorReset)
	fmt.Printf("  %scase%s       manage cases: -n new, -o open, -c close, list (all cases), templates\n", colorGreen, colorReset)
	fmt.Printf("  %scases%s      alias for %scase list%s\n", colorGreen, colorReset, colorGreen, colorReset)
	fmt.Printf("  %stools%s      list registered tools\n", colorGreen, colorReset)
	fmt.Printf("  %sdashboard%s   interactive TUI: tool explorer with live status\n", colorGreen, colorReset)
	fmt.Printf("  %srun%s        run a tool: run <name> [args...]\n", colorGreen, colorReset)
	fmt.Printf("  %sinstall%s    install a tool from the catalog: install <name>\n", colorGreen, colorReset)
	fmt.Printf("  %sbuild%s      (re)build an installed tool's image: build <name>\n", colorGreen, colorReset)
	fmt.Printf("  %suse%s        select a tool: use <name>\n", colorGreen, colorReset)
	fmt.Printf("  %snote%s       add a note to current case\n", colorGreen, colorReset)
	fmt.Printf("  %sevidence%s   manage evidence: add <path> [--tag a,b], tag, verify, classify [name]\n", colorGreen, colorReset)
	fmt.Printf("  %stimeline%s   show case timeline (-n N tails last N, export [path] [--format csv|json], correlate)\n", colorGreen, colorReset)
	fmt.Printf("  %sioc%s        extract IOCs: ioc <file> | ioc --from-output <name> | ioc diff <a> <b>\n", colorGreen, colorReset)
	fmt.Printf("  %scorrelate%s cross memory/disk/network IOC graph: correlate [--dot]\n", colorGreen, colorReset)
	fmt.Printf("  %sclear%s      clear screen\n", colorGreen, colorReset)
	fmt.Printf("  %ssearch%s     find cases matching a query across all cases; --in-output searches tool output\n", colorGreen, colorReset)
	fmt.Printf("  %sgrep%s       search tool output (alias for search --in-output)\n", colorGreen, colorReset)
	fmt.Printf("  %sexport%s     export case: export [path] [--no-output] [--json] [--provenance]\n", colorGreen, colorReset)
	fmt.Printf("  %simport%s     import a case archive: import <file.tar.gz> [--as name]\n", colorGreen, colorReset)
	fmt.Printf("  %stheme%s      list/set/save themes: theme [list|set <name>|save [path]]\n", colorGreen, colorReset)
	fmt.Printf("  %sai%s           LLM assistant: ask, analyze, suggest, explain\n", colorGreen, colorReset)
	fmt.Printf("  %splugin%s      manage plugins: list, run <name>, info <name>\n", colorGreen, colorReset)
	fmt.Printf("  %ssandbox%s     manage sandboxes: create, run, shell, logs, report, cp, destroy, list\n", colorGreen, colorReset)
	fmt.Printf("  %sdisk%s        disk forensics (SleuthKit): disk <tool> [-i <image>] — mmls, fls, icat, fsstat, ...\n", colorGreen, colorReset)
	fmt.Printf("  %sregripper%s    Windows registry hive analysis: regripper [hive] [-p plugin] [--json]\n", colorGreen, colorReset)
	fmt.Printf("  %sreport%s     generate incident report: report [path] [--format md|pdf] [--no-ai]\n", colorGreen, colorReset)
	fmt.Printf("  %svolatility%s  memory forensics (Volatility 3): volatility <plugin> [-f <dump>] [--offline] [args...]\n", colorGreen, colorReset)
	fmt.Printf("  %sanalyze%s    static malware analysis: analyze <path> [--rules <path>] (auto yara/capa/oletools)\n", colorGreen, colorReset)
	fmt.Printf("\n  %smimir mcp%s   run outside the shell: Run Mimir as an MCP server (stdio, or --http <addr>)\n", colorGreen, colorReset)
	return nil
}

func (a *App) cmdStatus(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		fmt.Println("No case is open.")
		return nil
	}
	fmt.Printf("  Case:   %s%s%s (%s)\n", colorCyan, c.Name, colorReset, c.Status)
	fmt.Printf("  Path:   %s\n", c.Path)
	fmt.Printf("  Tools:  %s\n", strings.Join(c.ToolsUsed, ", "))
	fmt.Printf("  Notes:  %d\n", len(c.Notes))
	return nil
}

func (a *App) cmdCase(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: case -n <name> [-t <template>] | case -o <name> | case -c | case list | case templates")
	}

	action := args[0]

	switch action {
	case "list", "ls":
		return a.cmdCases(args[1:])
	case "-n":
		if len(args) < 2 {
			return fmt.Errorf("usage: case -n <name> [-t <template>]")
		}
		name := args[1]
		templateName := "default"
		for i := 2; i < len(args); i++ {
			if args[i] == "-t" && i+1 < len(args) {
				templateName = args[i+1]
				i++
			}
		}
		c, err := a.Cases.CreateWithTemplate(name, templateName)
		if err != nil {
			return err
		}
		fmt.Printf("Case created: %s\n", c.Path)
	case "-o":
		if len(args) < 2 {
			return fmt.Errorf("usage: case -o <name>")
		}
		c, err := a.Cases.Open(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Case opened: %s\n", c.Path)
	case "-c":
		if err := a.Cases.Close(); err != nil {
			return err
		}
		fmt.Println("Case closed.")
	case "templates":
		names, err := cases.ListTemplates()
		if err != nil {
			return fmt.Errorf("list templates: %w", err)
		}
		if len(names) == 0 {
			fmt.Println("No templates available.")
			return nil
		}
		fmt.Println("Available case templates:")
		for i := range names {
			name := names[i]
			tmpl, err := cases.LoadTemplate(name[:len(name)-5]) // strip .yaml
			if err != nil {
				fmt.Printf("  %s (error loading: %v)\n", name, err)
				continue
			}
			fmt.Printf("  %-25s %s\n", tmpl.Name, tmpl.Description)
		}
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
	return nil
}

func (a *App) cmdCases(args []string) error {
	allCases, err := a.Cases.List()
	if err != nil {
		return err
	}

	statusFilter := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--status" && i+1 < len(args) {
			statusFilter = args[i+1]
			i++
		}
	}

	if len(allCases) == 0 {
		fmt.Println("No cases.")
		return nil
	}

	tbl := ui.Table{
		Headers: []string{"CASE", "STATUS", "TOOLS", "OPENED"},
		Align:   []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignRight, ui.AlignLeft},
	}
	for _, c := range allCases {
		if statusFilter != "" && c.Status != statusFilter {
			continue
		}
		statusIcon := "● open"
		if c.Status == "closed" {
			statusIcon = "○ closed"
		}
		opened := c.CreatedAt
		if len(opened) >= 10 {
			opened = opened[:10]
		}
		tbl.Rows = append(tbl.Rows, []string{
			c.Name,
			statusIcon,
			fmt.Sprintf("%d", len(c.ToolsUsed)),
			opened,
		})
	}
	if len(tbl.Rows) == 0 {
		fmt.Println("No cases found.")
		return nil
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	return nil
}

func statusBadge(s tools.Status) (string, string) {
	switch s {
	case tools.StatusReady:
		return s.String(), colorGreen
	case tools.StatusNotBuilt:
		return s.String(), colorYellow
	case tools.StatusDockerOff:
		return s.String(), colorDim
	case tools.StatusMissing:
		return s.String(), colorRed
	default:
		return s.String(), colorReset
	}
}

func (a *App) cmdTools(args []string) error {
	installed := a.Tools.List("")
	statuses, dockerUp := a.Runner.ResolveStatuses(installed)

	// INSTALLED section
	if len(installed) == 0 {
		fmt.Println("No tools installed.")
	} else {
		fmt.Printf("%sINSTALLED%s\n", colorCyan, colorReset)
		tbl := ui.Table{
			Headers: []string{"TOOL", "MODE", "STATUS", "DESCRIPTION"},
			Align:   []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignLeft, ui.AlignLeft},
		}
		for _, t := range installed {
			mode := "local"
			if t.RunsInDocker() {
				mode = "docker"
			}
			label, color := statusBadge(statuses[t.Name])
			tbl.Rows = append(tbl.Rows, []string{
				ui.Colorize(t.Name, colorGreen),
				mode,
				ui.Colorize(label, color),
				t.Description,
			})
		}
		tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	}

	// AVAILABLE section = catalog minus already-installed
	installedNames := map[string]bool{}
	for _, t := range installed {
		installedNames[t.Name] = true
	}
	entries, err := catalog.List()
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}
	var available []catalog.Entry
	for _, e := range entries {
		if !installedNames[e.Name] {
			available = append(available, e)
		}
	}
	if len(available) > 0 {
		fmt.Printf("\n%sAVAILABLE%s (install <name>)\n", colorCyan, colorReset)
		tbl := ui.Table{
			Headers: []string{"TOOL", "CATEGORY", "DESCRIPTION"},
			Align:   []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignLeft},
		}
		for _, e := range available {
			tbl.Rows = append(tbl.Rows, []string{
				ui.Colorize(e.Name, colorGreen),
				e.Category,
				e.Description,
			})
		}
		tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	}

	// BUILT-IN section: native-Go tools, always ready.
	bi := builtins.List()
	if len(bi) > 0 {
		fmt.Printf("\n%sBUILT-IN%s\n", colorCyan, colorReset)
		tbl := ui.Table{
			Headers: []string{"TOOL", "MODE", "STATUS", "DESCRIPTION"},
			Align:   []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignLeft, ui.AlignLeft},
		}
		for _, m := range bi {
			tbl.Rows = append(tbl.Rows, []string{
				ui.Colorize(m.Name, colorGreen),
				"builtin",
				ui.Colorize("ready", colorGreen),
				m.Description,
			})
		}
		tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	}

	// docker footer
	if dockerUp {
		fmt.Printf("\ndocker: %srunning%s\n", colorGreen, colorReset)
	} else {
		fmt.Printf("\ndocker: %snot available%s\n", colorRed, colorReset)
	}
	return nil
}

// cmdDashboard enters an interactive Bubble Tea dashboard with the tool explorer.
func (a *App) cmdDashboard(args []string) error {
	installed := a.Tools.List("")
	statuses, _ := a.Runner.ResolveStatuses(installed)

	model := ui.NewToolExplorer(installed, statuses, a.toolRunCounts())
	model.SetResolver(func() (map[string]tools.Status, bool) {
		return a.Runner.ResolveStatuses(installed)
	})

	// Enter full-screen TUI mode. Quitting yields the final model, which the
	// explorer uses to hand back a tool chosen via Enter for immediate run.
	prog := tea.NewProgram(model, tea.WithAltScreen())
	final, err := prog.Run()
	if err != nil {
		return err
	}
	explorer, ok := final.(*ui.ToolExplorer)
	if !ok {
		return nil
	}
	if name := explorer.SelectedTool(); name != "" {
		return a.runToolByName(name, nil)
	}
	return nil
}

// toolRunCounts returns how many times each tool was executed in the current
// case, derived from tool_run timeline events.
func (a *App) toolRunCounts() map[string]int {
	counts := map[string]int{}
	c := a.Cases.Current()
	if c == nil {
		return counts
	}
	for _, ev := range c.Timeline() {
		if ev.Type != "tool_run" {
			continue
		}
		name, ok := ev.Payload["tool"].(string)
		if !ok || name == "" {
			continue
		}
		counts[name]++
	}
	return counts
}

func (a *App) cmdRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: run <tool_name> [args...]")
	}
	toolArgs, err := a.resolveRunArgs(args[1:])
	if err != nil {
		return err
	}
	return a.runToolByName(args[0], toolArgs)
}

// resolveRunArgs rewrites relative path arguments to absolute paths inside the
// open case. This lets `run hash evidence/sample.bin` and `run hash sample.bin`
// work from any working directory, while absolute paths are passed through
// unchanged. Non-path arguments (flags, plugin names, numbers) are left alone.
func (a *App) resolveRunArgs(args []string) ([]string, error) {
	out := make([]string, len(args))
	for i, arg := range args {
		resolved, err := a.resolveRunArg(arg)
		if err != nil {
			return nil, err
		}
		out[i] = resolved
	}
	return out, nil
}

func (a *App) resolveRunArg(arg string) (string, error) {
	if arg == "" || filepath.IsAbs(arg) {
		return arg, nil
	}

	c := a.Cases.Current()
	if c != nil {
		// `evidence/file` resolves under the case root; a bare basename falls
		// through to the evidence directory.
		for _, base := range []string{c.Path, filepath.Join(c.Path, "evidence")} {
			candidate := filepath.Join(base, arg)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}

	// Preserve the existing cwd-relative behaviour for files that really are
	// relative to where Mimir was started.
	if _, err := os.Stat(arg); err == nil {
		return arg, nil
	}

	if !looksLikePathArg(arg) {
		return arg, nil
	}
	if c == nil {
		return "", fmt.Errorf("file not found: %s (no case is open — open a case or use an absolute path)", arg)
	}
	return "", fmt.Errorf("file not found in case %q: %s (looked in %s and its evidence directory)", c.Name, arg, c.Path)
}

// looksLikePathArg distinguishes a path-shaped tool argument from a flag value
// or other bare token, so only real paths produce a resolution error.
func looksLikePathArg(arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	if strings.ContainsAny(arg, `/\`) || arg == "." || arg == ".." || strings.HasPrefix(arg, ".") {
		return true
	}
	return filepath.Ext(arg) != ""
}

// runToolByName runs a named tool with the given args, prints its output, and
// records the run into the open case (timeline + output file + tool usage).
func (a *App) runToolByName(toolName string, toolArgs []string) error {
	casePath := ""
	if c := a.Cases.Current(); c != nil {
		casePath = c.Path
	}

	var result *tools.Result
	if builtins.Has(toolName) {
		result = a.Runner.RunBuiltin(toolName, toolArgs)
	} else {
		tool, ok := a.Tools.Get(toolName)
		if !ok {
			return fmt.Errorf("tool not found: %s", toolName)
		}
		result = a.Runner.Run(tool, toolArgs, casePath)
	}

	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}

	// Record into the open case (output file + timeline event), even on failure.
	if c := a.Cases.Current(); c != nil {
		a.recordRun(c, result)
		c.AddToolUsage(toolName)
		if err := c.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s save case: %v\n", colorYellow, colorReset, err)
		}
	}

	if !result.Success() && result.Error != nil {
		return result.Error
	}
	return nil
}

// recordRun saves output to the case and appends a tool_run timeline event.
// Failures are surfaced as warnings — never silently swallowed — but do not
// abort the run result already shown to the analyst.
func (a *App) recordRun(c *cases.Case, result *tools.Result) {
	outputRel := ""
	if outputPath, err := a.Output.Record(result.Tool, result.Stdout, c.Path); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s capture output: %v\n", colorYellow, colorReset, err)
	} else if rel, err := filepath.Rel(c.Path, outputPath); err == nil {
		outputRel = rel
	} else {
		outputRel = outputPath
	}

	payload := map[string]any{
		"tool":        result.Tool,
		"args":        result.Args,
		"return_code": result.ReturnCode,
		"duration_ms": result.Duration().Milliseconds(),
		"output_file": outputRel,
		"success":     result.Success(),
	}
	if !result.Success() {
		stderrMsg := result.Stderr
		if stderrMsg == "" && result.Error != nil {
			stderrMsg = result.Error.Error()
		}
		if stderrMsg != "" {
			payload["stderr"] = stderrMsg
		}
	}
	if err := c.AppendEvent(cases.TimelineEvent{
		Type:      "tool_run",
		Timestamp: time.Now().Format(time.RFC3339),
		Payload:   payload,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s append timeline: %v\n", colorYellow, colorReset, err)
	}
}

func (a *App) cmdUse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: use <tool_name>")
	}
	tool, ok := a.Tools.Get(args[0])
	if !ok {
		return fmt.Errorf("tool not found: %s", args[0])
	}
	fmt.Printf("Selected tool: %s%s%s — %s\n", colorCyan, tool.Name, colorReset, tool.Description)
	return nil
}

func (a *App) cmdNote(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: note <text>")
	}
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}
	content := strings.Join(args, " ")
	if err := c.AddNote(content, "analyst"); err != nil {
		return err
	}
	return c.Save()
}

// filterTimeline keeps events matching all supplied filters. types is a set of
// event-type names (empty = any); grep is a case-insensitive substring matched
// against the event type and its payload values (empty = any).
func filterTimeline(evs []cases.TimelineEvent, types []string, grep string) []cases.TimelineEvent {
	typeSet := map[string]bool{}
	for _, t := range types {
		typeSet[t] = true
	}
	grep = strings.ToLower(grep)
	var out []cases.TimelineEvent
	for _, ev := range evs {
		if len(typeSet) > 0 && !typeSet[ev.Type] {
			continue
		}
		if grep != "" && !eventMatches(ev, grep) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func eventMatches(ev cases.TimelineEvent, lowerSub string) bool {
	if strings.Contains(strings.ToLower(ev.Type), lowerSub) {
		return true
	}
	for _, v := range ev.Payload {
		if strings.Contains(strings.ToLower(fmt.Sprint(v)), lowerSub) {
			return true
		}
	}
	return false
}

func (a *App) cmdTimeline(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}

	// Handle "timeline build --auto" subcommand: correlate events into a narrative.
	if len(args) > 0 && args[0] == "build" {
		narr, err := c.Narrative()
		if err != nil {
			return err
		}
		outPath := filepath.Join(c.Path, "output", "narrative.md")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("narrative: create dir: %w", err)
		}
		if err := os.WriteFile(outPath, []byte(narr), 0644); err != nil {
			return fmt.Errorf("narrative: write: %w", err)
		}
		fmt.Printf("%sNarrative written:%s %s\n", colorGreen, colorReset, outPath)
		return nil
	}

	// Handle "timeline correlate" subcommand (G5): group tool runs by tool
	// and show which tools contributed which indicators — a cross-tool view.
	if len(args) > 0 && args[0] == "correlate" {
		return a.timelineCorrelate(c)
	}

	// Handle "timeline export" subcommand
	if len(args) > 0 && args[0] == "export" {
		format := "csv"
		path := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--format" && i+1 < len(args) {
				format = args[i+1]
				i++
			} else if path == "" {
				path = args[i]
			}
		}
		if err := c.ExportTimeline(format, path); err != nil {
			return err
		}
		fmt.Printf("Timeline exported to %s\n", path)
		return nil
	}

	evs := c.Timeline()

	var types []string
	grep := ""
	tail := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type":
			if i+1 < len(args) {
				types = strings.Split(args[i+1], ",")
				i++
			}
		case "--grep":
			if i+1 < len(args) {
				grep = args[i+1]
				i++
			}
		case "-n":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &tail)
				i++
			}
		}
	}

	evs = filterTimeline(evs, types, grep)
	if tail > 0 && tail < len(evs) {
		evs = evs[len(evs)-tail:]
	}

	if len(evs) == 0 {
		fmt.Println("Timeline is empty.")
		return nil
	}

	for _, ev := range evs {
		ts := ev.Timestamp
		if t, err := time.Parse(time.RFC3339, ev.Timestamp); err == nil {
			ts = t.Format("15:04:05")
		}
		switch ev.Type {
		case "tool_run":
			color := colorGreen
			if ev.Payload["success"] != true {
				color = colorRed
			}
			fmt.Printf("%s  %s▶%s run %v %v → code %v (%vms) %s[%v]%s\n",
				ts, color, colorReset,
				ev.Payload["tool"], ev.Payload["args"],
				ev.Payload["return_code"], ev.Payload["duration_ms"],
				colorDim, ev.Payload["output_file"], colorReset)
		case "note":
			fmt.Printf("%s  %s✎%s note: %v\n", ts, colorCyan, colorReset, ev.Payload["content"])
		case "case_opened":
			fmt.Printf("%s  %s•%s case opened\n", ts, colorYellow, colorReset)
		case "case_closed":
			fmt.Printf("%s  %s•%s case closed\n", ts, colorYellow, colorReset)
		default:
			fmt.Printf("%s  %s\n", ts, ev.Type)
		}
	}
	return nil
}

// timelineCorrelate (G5) groups tool runs by tool and reports which tools
// contributed findings, plus a per-tool indicator count. It gives a
// cross-tool overview of how the case was built.
func (a *App) timelineCorrelate(c *cases.Case) error {
	runs := 0
	byTool := map[string]int{}
	var toolRuns []string
	for _, ev := range c.Timeline() {
		if ev.Type != "tool_run" {
			continue
		}
		runs++
		tool, _ := ev.Payload["tool"].(string)
		cmd := ev.ToolRunCommand()
		if tool == "" {
			tool = "unknown"
		}
		byTool[tool]++
		if cmd != "" {
			toolRuns = append(toolRuns, cmd)
		}
	}
	if runs == 0 {
		fmt.Println("No tool runs in timeline yet — run a tool first (volatility, sandbox run -- <tool>, ...).")
		return nil
	}
	fmt.Printf("%sTool-run correlation (G5)%s — %d runs across %d tool(s):\n",
		colorCyan, colorReset, runs, len(byTool))
	tbl := ui.Table{
		Headers: []string{"TOOL", "RUNS"},
		Align:   []ui.Align{ui.AlignLeft, ui.AlignRight},
	}
	for tool, n := range byTool {
		tbl.Rows = append(tbl.Rows, []string{tool, fmt.Sprintf("%d", n)})
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))

	// Per-type indicator tally (cross-domain summary).
	byType := map[string]int{}
	for _, ic := range c.IOCs() {
		byType[ic.Type]++
	}
	fmt.Printf("\n%sIndicators by type%s\n", colorCyan, colorReset)
	itbl := ui.Table{
		Headers: []string{"TYPE", "COUNT"},
		Align:   []ui.Align{ui.AlignLeft, ui.AlignRight},
	}
	for typ, n := range byType {
		itbl.Rows = append(itbl.Rows, []string{typ, fmt.Sprintf("%d", n)})
	}
	itbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	return nil
}
func (a *App) cmdIOC(args []string) error {
	if len(args) == 0 {
		return a.iocList()
	}
	if args[0] == "diff" {
		return a.cmdIOCDiff(args[1:])
	}
	c := a.Cases.Current()
	var data []byte
	var source string
	if args[0] == "--from-output" {
		if len(args) < 2 {
			return fmt.Errorf("usage: ioc --from-output <name>")
		}
		if c == nil {
			return fmt.Errorf("no case is open")
		}
		source = args[1]
		b, err := os.ReadFile(filepath.Join(c.Path, "output", source))
		if err != nil {
			return fmt.Errorf("read output %q: %w", source, err)
		}
		data = b
	} else {
		source = args[0]
		b, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read %q: %w", source, err)
		}
		data = b
	}

	inds := ioc.Extract(data)
	if len(inds) == 0 {
		fmt.Println("No indicators found.")
		return nil
	}

	tbl := ui.Table{Headers: []string{"TYPE", "VALUE"}, Align: []ui.Align{ui.AlignLeft, ui.AlignLeft}}
	counts := map[string]int{}
	now := time.Now().Format(time.RFC3339)
	for _, ind := range inds {
		tbl.Rows = append(tbl.Rows, []string{ind.Type, ind.Value})
		counts[ind.Type]++
		if c != nil {
			if err := c.AppendIOC(cases.IOCRecord{Type: ind.Type, Value: ind.Value, Source: source, Time: now}); err != nil {
				fmt.Fprintf(os.Stderr, "%swarning:%s append ioc: %v\n", colorYellow, colorReset, err)
			}
		}
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	if c != nil {
		if err := c.AppendEvent(cases.TimelineEvent{
			Type: "ioc_extracted", Timestamp: now,
			Payload: map[string]any{"source": source, "counts": counts, "total": len(inds)},
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s append timeline: %v\n", colorYellow, colorReset, err)
		}
		fmt.Printf("→ %d indicators tracked\n", len(inds))
	}
	return nil
}

// cmdIOCDiff compares the indicator sets extracted from two sources
// (output files or raw text) and reports what is added, removed, or
// shared. Useful for diffing two memory snapshots or two tool runs
// (G7 in docs/gap-analysis.md).
func (a *App) cmdIOCDiff(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ioc diff <fileA> <fileB>  (compare extracted indicator sets)")
	}
	read := func(p string) (map[string]bool, error) {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", p, err)
		}
		set := map[string]bool{}
		for _, ind := range ioc.Extract(b) {
			set[ind.Type+":"+ind.Value] = true
		}
		return set, nil
	}
	aSet, err := read(args[0])
	if err != nil {
		return err
	}
	bSet, err := read(args[1])
	if err != nil {
		return err
	}
	var added, removed, shared []string
	for k := range aSet {
		if bSet[k] {
			shared = append(shared, k)
		} else {
			removed = append(removed, k)
		}
	}
	for k := range bSet {
		if !aSet[k] {
			added = append(added, k)
		}
	}
	fmt.Printf("%sΔ between %s and %s%s\n", colorCyan, args[0], args[1], colorReset)
	fmt.Printf("  %sadded (only in B):%s   %d\n", colorGreen, colorReset, len(added))
	fmt.Printf("  %sremoved (only in A):%s %d\n", colorYellow, colorReset, len(removed))
	fmt.Printf("  %sshared:%s            %d\n", colorBlue, colorReset, len(shared))
	for _, k := range added {
		fmt.Printf("    + %s\n", k)
	}
	for _, k := range removed {
		fmt.Printf("    - %s\n", k)
	}
	return nil
}

func (a *App) cmdCorrelate(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}
	iocs := c.IOCs()
	if len(iocs) == 0 {
		fmt.Println("No IOCs to correlate — extract some first (ioc, volatility, sandbox run -- <tool>).")
		return nil
	}
	dot := false
	for _, arg := range args {
		if arg == "--dot" {
			dot = true
		}
	}

	// Classify each IOC's domain by its source tool.
	domainOf := func(src string) string {
		l := strings.ToLower(src)
		switch {
		case strings.Contains(l, "volatility"), strings.Contains(l, "mem"):
			return "memory"
		case strings.Contains(l, "sleuthkit"), strings.Contains(l, "icat"), strings.Contains(l, "fls"):
			return "disk"
		case strings.Contains(l, "zeek"), strings.Contains(l, "tshark"):
			return "network"
		default:
			return "other"
		}
	}

	// value -> set of domains it appeared in.
	byValue := map[string]map[string]bool{}
	byValueTypes := map[string]string{}
	for _, ic := range iocs {
		d := domainOf(ic.Source)
		if byValue[ic.Value] == nil {
			byValue[ic.Value] = map[string]bool{}
			byValueTypes[ic.Value] = ic.Type
		}
		byValue[ic.Value][d] = true
	}

	fmt.Printf("%sCorrelation graph (G12)%s — %d indicators across domains:\n", colorCyan, colorReset, len(iocs))
	domCount := map[string]int{}
	for _, dmap := range byValue {
		for d := range dmap {
			domCount[d]++
		}
	}
	dtbl := ui.Table{Headers: []string{"DOMAIN", "INDICATORS"}, Align: []ui.Align{ui.AlignLeft, ui.AlignRight}}
	for _, d := range []string{"memory", "disk", "network", "other"} {
		if domCount[d] > 0 {
			dtbl.Rows = append(dtbl.Rows, []string{d, fmt.Sprintf("%d", domCount[d])})
		}
	}
	dtbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))

	// Cross-domain IOCs: the high-value signal (same artifact seen in 2+ domains).
	var cross []string
	for v, dmap := range byValue {
		if len(dmap) >= 2 {
			cross = append(cross, v)
		}
	}
	if len(cross) > 0 {
		fmt.Printf("\n%sCross-domain indicators (%d)%s — seen in 2+ domains:\n", colorYellow, len(cross), colorReset)
		for _, v := range cross {
			doms := []string{}
			for d := range byValue[v] {
				doms = append(doms, d)
			}
			fmt.Printf("  %s [%s] %s\n", byValueTypes[v], strings.Join(doms, ","), v)
		}
	} else {
		fmt.Printf("\n%sNo cross-domain indicators yet.%s\n", colorDim, colorReset)
	}

	if dot {
		fmt.Printf("\ndigraph correlate {\n")
		for v, dmap := range byValue {
			label := strings.ReplaceAll(v, "\"", "")
			fmt.Printf("  \"%s\" [label=\"%s\\n(%s)\"];\n", label, label, byValueTypes[v])
			for d := range dmap {
				fmt.Printf("  \"%s\" -> \"domain:%s\";\n", label, d)
			}
		}
		fmt.Printf("}\n")
	}
	return nil
}

func (a *App) iocList() error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}
	iocs := c.IOCs()
	if len(iocs) == 0 {
		fmt.Println("No IOCs tracked. Extract with: ioc <file>")
		return nil
	}
	tbl := ui.Table{Headers: []string{"TYPE", "VALUE", "SOURCE"}, Align: []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignLeft}}
	for _, i := range iocs {
		tbl.Rows = append(tbl.Rows, []string{i.Type, i.Value, i.Source})
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	return nil
}

func (a *App) cmdSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: search <query> [--in-output]")
	}

	searchInOutput := false
	var queryParts []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--in-output" {
			searchInOutput = true
			continue
		}
		queryParts = append(queryParts, args[i])
	}
	if len(queryParts) == 0 {
		return fmt.Errorf("usage: search <query> [--in-output]")
	}
	q := strings.Join(queryParts, " ")

	if searchInOutput {
		c := a.Cases.Current()
		if c == nil {
			return fmt.Errorf("no case is open")
		}
		results := c.SearchOutput(q)
		if len(results) == 0 {
			fmt.Println("No matching output lines.")
			return nil
		}
		for _, r := range results {
			fmt.Printf("%s:%d: %s\n", r.File, r.Line, r.Content)
		}
		return nil
	}

	all, err := a.Cases.List()
	if err != nil {
		return fmt.Errorf("list cases: %w", err)
	}
	tbl := ui.Table{Headers: []string{"CASE", "MATCHED IN"}, Align: []ui.Align{ui.AlignLeft, ui.AlignLeft}}
	for _, c := range all {
		if ok, where := caseMatches(c, q); ok {
			tbl.Rows = append(tbl.Rows, []string{c.Name, where})
		}
	}
	if len(tbl.Rows) == 0 {
		fmt.Println("No matching cases.")
		return nil
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	return nil
}

func (a *App) cmdGrep(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: grep <pattern> [--in-output]")
	}
	return a.cmdSearch(append(args, "--in-output"))
}

// caseMatches reports whether the lowercased query appears in the case and a
// hint naming the field(s) it matched.
func caseMatches(c *cases.Case, lowerQuery string) (bool, string) {
	var where []string
	mark := func(field, hay string) {
		if strings.Contains(strings.ToLower(hay), lowerQuery) && !containsShell(where, field) {
			where = append(where, field)
		}
	}
	mark("name", c.Name)
	for _, n := range c.Notes {
		mark("notes", n.Content)
	}
	for _, tl := range c.ToolsUsed {
		mark("tools", tl)
	}
	for k, v := range c.Metadata {
		mark("metadata", k+" "+v)
	}
	for _, e := range c.Evidence() {
		mark("evidence", e.Name+" "+strings.Join(e.Tags, " "))
	}
	for _, i := range c.IOCs() {
		mark("ioc", i.Value)
	}
	return len(where) > 0, strings.Join(where, ", ")
}

func containsShell(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (a *App) cmdClear(args []string) error {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	return nil
}

func (a *App) cmdExport(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}
	jsonMode := false
	includeOutput := true
	provenance := false
	out := ""
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonMode = true
		case "--no-output":
			includeOutput = false
		case "--provenance":
			provenance = true
		default:
			out = arg
		}
	}
	if jsonMode {
		if out == "" {
			out = c.Name + ".json"
		}
		if err := casearchive.ExportJSON(c, out); err != nil {
			return err
		}
		fmt.Printf("exported %s\n", out)
		return nil
	}
	if out == "" {
		out = c.Name + ".tar.gz"
	}
	if provenance {
		if err := casearchive.ExportFull(c, out, includeOutput, Version); err != nil {
			return err
		}
		fmt.Printf("exported %s (with provenance manifest: file hashes, tool runs, indicators)\n", out)
		return nil
	}
	if err := casearchive.Export(c.Path, out, includeOutput); err != nil {
		return err
	}
	fmt.Printf("exported %s\n", out)
	return nil
}

func (a *App) cmdImport(args []string) error {
	asName := ""
	archive := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--as" && i+1 < len(args) {
			asName = args[i+1]
			i++
			continue
		}
		archive = args[i]
	}
	if archive == "" {
		return fmt.Errorf("usage: import <archive.tar.gz> [--as <name>]")
	}
	name, err := casearchive.Import(archive, a.Config.CasesPath, asName)
	if err != nil {
		return err
	}
	fmt.Printf("imported case %s%s%s\n", colorGreen, name, colorReset)
	return nil
}

func (a *App) cmdTheme(args []string) error {
	if len(args) == 0 {
		return a.themeList()
	}
	switch args[0] {
	case "set":
		if len(args) < 2 {
			return fmt.Errorf("usage: theme set <name>")
		}
		t := theme.BuiltinTheme(args[1])
		a.Theme = t
		fmt.Printf("theme set to %s%s%s\n", colorGreen, t.Name, colorReset)
		return nil
	case "save":
		path := a.Config.ConfigPath()
		if len(args) >= 2 {
			path = args[1]
		}
		return theme.Save(a.Theme, path)
	default:
		return fmt.Errorf("usage: theme [list|set <name>|save [path]]")
	}
}

func (a *App) cmdSandbox(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sandbox <create|run|shell|logs|report|cp|destroy|list> [args...]")
	}
	sh := sandbox.NewShell(a.SandboxManager, a.SandboxRuntime)
	sh.Capture = func(tool, command, output string) error {
		if c := a.Cases.Current(); c != nil {
			return captureInto(c, tool, command, "", output)
		}
		return nil
	}
	var buf bytes.Buffer
	cmd := args[0]
	switch cmd {
	case "create":
		return sh.Create(&buf, args[1:])
	case "run":
		return sh.Run(&buf, args[1:])
	case "shell":
		return sh.Shell(&buf, args[1:])
	case "logs":
		return sh.Logs(&buf, args[1:])
	case "report":
		return sh.Report(&buf, args[1:])
	case "cp":
		return sh.CopyFile(&buf, args[1:])
	case "destroy":
		return sh.Destroy(&buf, args[1:])
	case "list":
		return sh.List(&buf, args[1:])
	default:
		return fmt.Errorf("unknown sandbox command: %s", cmd)
	}
}

func (a *App) themeList() error {
	current := a.Theme.Name
	fmt.Printf("  %sthemes%s\n", colorGreen, colorReset)
	for _, t := range theme.BuiltinThemes() {
		marker := "  "
		if t.Name == current {
			marker = colorGreen + "▶ " + colorReset
		}
		fmt.Printf("  %s%s%s  %s\n", marker, t.Name, colorReset, t.Description)
	}
	fmt.Printf("\nUse: theme set <name>\n")
	return nil
}

func (a *App) cmdAI(args []string) error {
	if a.AI == nil {
		return fmt.Errorf("AI not configured. Set ai.provider and ai.model in config.yaml or via env vars")
	}

	if len(args) == 0 {
		return a.aiHelp()
	}

	logAI := func(query, response string) {
		if c := a.Cases.Current(); c != nil {
			c.AppendEvent(cases.TimelineEvent{
				Type:      "ai_query",
				Timestamp: time.Now().Format(time.RFC3339),
				Payload:   map[string]any{"query": query, "provider": a.AI.ProviderName()},
			})
			if response != "" {
				c.AppendEvent(cases.TimelineEvent{
					Type:      "ai_response",
					Timestamp: time.Now().Format(time.RFC3339),
					Payload:   map[string]any{"response": response},
				})
			}
		}
	}

	switch args[0] {
	case "ask":
		question := strings.Join(args[1:], " ")
		if question == "" {
			return fmt.Errorf("usage: ai ask <question>")
		}
		messages := a.buildAIMessages()
		logAI(question, "")
		fmt.Printf("%s[AI %s]%s\n", colorCyan, a.AI.ProviderName(), colorReset)
		return a.AI.Ask(context.Background(), question, messages)

	case "analyze":
		messages := a.buildAIMessages()
		if messages == nil {
			return fmt.Errorf("no case data to analyze. Open a case first")
		}
		logAI("analyze case", "")
		fmt.Printf("%s[AI %s]%s\n", colorCyan, a.AI.ProviderName(), colorReset)
		return a.AI.Analyze(context.Background(), messages)

	case "suggest":
		messages := a.buildAIMessages()
		if messages == nil {
			return fmt.Errorf("no case data for suggestions. Open a case first")
		}
		logAI("suggest next steps", "")
		fmt.Printf("%s[AI %s]%s\n", colorCyan, a.AI.ProviderName(), colorReset)
		return a.AI.Suggest(context.Background(), messages)

	case "explain":
		if len(args) < 2 {
			return fmt.Errorf("usage: ai explain <text>")
		}
		output := strings.Join(args[1:], " ")
		logAI("explain: "+output[:min(80, len(output))], "")
		return a.AI.Explain(context.Background(), output, nil)

	case "summarize":
		return a.aiSummarize()

	case "config":
		return a.aiConfig(args[1:])

	case "models":
		return a.aiModels()

	case "use":
		return a.aiUse(args[1:])

	default:
		return a.aiHelp()
	}
}

func (a *App) buildAIMessages() []ai.Message {
	c := a.Cases.Current()
	if c == nil {
		return nil
	}
	opts := ai.DefaultContextOptions()
	if a.AI != nil {
		// Local/self-hosted providers get unmasked context; remote providers
		// get PII-safe (evidence-filename-masked) context.
		opts.Local = a.AI.IsLocal()
	}
	return ai.BuildContext(c, opts)
}

func (a *App) cmdPlugin(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: plugin <list|run|info> [name]")
	}
	switch args[0] {
	case "list":
		names := a.Plugins.List()
		if len(names) == 0 {
			fmt.Println("No plugins discovered.")
			return nil
		}
		for _, name := range names {
			p, _ := a.Plugins.Get(name)
			if p != nil {
				fmt.Printf("  %-20s %s\n", p.Name, p.Description)
			}
		}
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: plugin run <name> [args...]")
		}
		p, ok := a.Plugins.Get(args[1])
		if !ok {
			return fmt.Errorf("plugin not found: %s", args[1])
		}
		binary := filepath.Join(a.Config.PluginsPath, p.Name, p.Entrypoint)
		cmd := exec.Command(binary, args[2:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name, err)
		}
	case "info":
		if len(args) < 2 {
			return fmt.Errorf("usage: plugin info <name>")
		}
		p, ok := a.Plugins.Get(args[1])
		if !ok {
			return fmt.Errorf("plugin not found: %s", args[1])
		}
		fmt.Printf("  Name:        %s\n", p.Name)
		fmt.Printf("  Description: %s\n", p.Description)
		fmt.Printf("  Version:     %s\n", p.Version)
		fmt.Printf("  Enabled:     %v\n", p.Enabled)
	default:
		return fmt.Errorf("unknown plugin action: %s", args[0])
	}
	return nil
}

func (a *App) aiHelp() error {
	fmt.Printf("  %sai ask <q>%s       ask a question (with case context)\n", colorGreen, colorReset)
	fmt.Printf("  %sai analyze%s       summarize current case findings\n", colorGreen, colorReset)
	fmt.Printf("  %sai suggest%s       recommend next investigation steps\n", colorGreen, colorReset)
	fmt.Printf("  %sai explain <t>%s   explain tool output or concept\n", colorGreen, colorReset)
	fmt.Printf("  %sai summarize%s     generate a case narrative\n", colorGreen, colorReset)
	fmt.Printf("  %sai config%s        show AI configuration\n", colorGreen, colorReset)
	fmt.Printf("  %sai models%s        list models the provider can serve\n", colorGreen, colorReset)
	fmt.Printf("  %sai use <m>%s       switch active model (add --save to persist)\n", colorGreen, colorReset)
	fmt.Printf("\nProvider: %s%s%s   Model: %s%s%s\n", colorCyan, a.AI.ProviderName(), colorReset, colorCyan, a.AI.Model(), colorReset)
	return nil
}

func (a *App) aiConfig(args []string) error {
	cfg := a.Config.AI
	if len(args) == 0 {
		redacted := cfg.Redact()
		fmt.Printf("  provider:    %s%s%s\n", colorCyan, redacted.Provider, colorReset)
		fmt.Printf("  model:       %s%s%s\n", colorCyan, redacted.Model, colorReset)
		fmt.Printf("  base_url:    %s%s%s\n", colorCyan, redacted.BaseURL, colorReset)
		fmt.Printf("  api_key:     %s%s%s\n", colorCyan, redacted.APIKey, colorReset)
		fmt.Printf("  max_tokens:  %s%d%s\n", colorCyan, cfg.MaxTokens, colorReset)
		fmt.Printf("  temperature: %s%.2f%s\n", colorCyan, cfg.Temperature, colorReset)
		fmt.Printf("  ctx_window:  %s%d%s\n", colorCyan, cfg.ContextWindow, colorReset)
		fmt.Printf("  timeout:     %s%ds%s\n", colorCyan, cfg.Timeout, colorReset)
		return nil
	}
	return fmt.Errorf("usage: ai config (read-only, edit config.yaml to change)")
}

// aiModels lists the models the active provider can serve.
func (a *App) aiModels() error {
	models, err := a.AI.ListModels(context.Background())
	if err != nil {
		if err == ai.ErrModelDiscoveryUnsupported {
			fmt.Printf("Model discovery is not supported by the %q provider.\n", a.AI.ProviderName())
			fmt.Printf("Current model: %s%s%s\n", colorCyan, a.AI.Model(), colorReset)
			return nil
		}
		return err
	}
	if len(models) == 0 {
		fmt.Println("No models reported by the provider.")
		return nil
	}
	current := a.AI.Model()
	for _, m := range models {
		marker := "  "
		if m.Name == current {
			marker = colorGreen + "* " + colorReset
		}
		if m.Size > 0 {
			fmt.Printf("%s%-40s %s\n", marker, m.Name, humanSize(m.Size))
		} else {
			fmt.Printf("%s%s\n", marker, m.Name)
		}
	}
	return nil
}

// aiUse switches the active model at runtime; --save persists it to config.yaml.
func (a *App) aiUse(args []string) error {
	var model string
	save := false
	for _, arg := range args {
		if arg == "--save" {
			save = true
		} else if model == "" {
			model = arg
		}
	}
	if model == "" {
		return fmt.Errorf("usage: ai use <model> [--save]")
	}

	// Validate against the provider's model list when discovery is available.
	if models, err := a.AI.ListModels(context.Background()); err == nil {
		found := false
		for _, m := range models {
			if m.Name == model {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("model %q not available from provider %q (run 'ai models' to list)", model, a.AI.ProviderName())
		}
	}

	a.AI.SetModel(model)
	a.Config.AI.Model = model
	fmt.Printf("Active model: %s%s%s\n", colorCyan, model, colorReset)

	if save {
		if err := a.Config.Save(); err != nil {
			return fmt.Errorf("model switched for this session, but save failed: %w", err)
		}
		fmt.Printf("Saved to %s\n", a.Config.ConfigPath())
	}
	return nil
}

// humanSize renders a byte count as a human-readable string.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// aiSummarize prints an AI-generated case narrative to stdout.
func (a *App) aiSummarize() error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}
	if a.AI == nil {
		return fmt.Errorf("AI not configured")
	}
	summary, err := a.AI.Summarize(context.Background(), c)
	if err != nil {
		return err
	}
	fmt.Printf("%s[AI %s] Summary%s\n\n%s\n", colorCyan, a.AI.ProviderName(), colorReset, summary)
	return nil
}

// cmdReport generates a full incident report for the current case.
func (a *App) cmdReport(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}

	opts := cases.ReportOptions{Format: cases.ReportMarkdown}
	noAI := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-ai":
			noAI = true
		case "--format":
			if i+1 < len(args) {
				opts.Format = cases.ReportFormat(args[i+1])
				i++
			}
		case "-o", "--output":
			if i+1 < len(args) {
				opts.Path = args[i+1]
				i++
			}
		default:
			if opts.Path == "" && !strings.HasPrefix(args[i], "-") {
				opts.Path = args[i]
			}
		}
	}

	var summary string
	if !noAI && a.AI != nil {
		s, err := a.AI.Summarize(context.Background(), c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s AI summary skipped: %v\n", colorYellow, colorReset, err)
		} else {
			summary = s
		}
	}

	md, err := cases.BuildReport(c, summary)
	if err != nil {
		return err
	}

	// Determine output path.
	outPath := opts.Path
	if outPath == "" {
		outPath = filepath.Join(c.Path, "output", "report.md")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("report: create output dir: %w", err)
	}

	// PDF: optional pandoc Docker; degrade to Markdown with a warning.
	if opts.Format == cases.ReportPDF {
		if d, perr := runPandoc(outPath, md); perr != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s PDF unavailable (%v); wrote Markdown instead\n", colorYellow, colorReset, perr)
			outPath = strings.TrimSuffix(outPath, ".pdf") + ".md"
		} else {
			fmt.Printf("%sReport written:%s %s\n", colorGreen, colorReset, d)
			if err := c.AppendEvent(cases.TimelineEvent{Type: "report_generated", Timestamp: time.Now().Format(time.RFC3339), Payload: map[string]any{"path": d, "format": "pdf"}}); err != nil {
				fmt.Fprintf(os.Stderr, "%swarning:%s append timeline: %v\n", colorYellow, colorReset, err)
			}
			return nil
		}
	}

	if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
		return fmt.Errorf("report: write: %w", err)
	}
	fmt.Printf("%sReport written:%s %s\n", colorGreen, colorReset, outPath)
	if err := c.AppendEvent(cases.TimelineEvent{Type: "report_generated", Timestamp: time.Now().Format(time.RFC3339), Payload: map[string]any{"path": outPath, "format": "md"}}); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s append timeline: %v\n", colorYellow, colorReset, err)
	}
	return nil
}

// runPandoc converts Markdown to PDF via the pandoc Docker image, returning the
// output path. It is best-effort: callers degrade to Markdown on any failure.
func runPandoc(mdPath, md string) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker not available")
	}
	tmp := mdPath
	if strings.HasSuffix(tmp, ".pdf") {
		tmp = strings.TrimSuffix(tmp, ".pdf") + ".md"
	}
	if err := os.WriteFile(tmp, []byte(md), 0644); err != nil {
		return "", err
	}
	pdfPath := strings.TrimSuffix(tmp, ".md") + ".pdf"
	src := filepath.Base(tmp)
	cmd := exec.Command("docker", "run", "--rm",
		"-v", filepath.Dir(tmp)+":/data",
		"pandoc/core", "pandoc", "/data/"+src, "-o", "/data/"+filepath.Base(pdfPath))
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v: %s", err, out)
	}
	return pdfPath, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *App) cmdShell(line string) error {
	// Shell passthrough. The operator runs arbitrary shell commands by design,
	// so `sh -c` is intentional (pipes/globs/redirects). We only refine error
	// reporting: a subprocess that exits non-zero already printed its own
	// message to stderr, so we don't rewrap that as a Mimir `error:`.
	cmd := exec.Command("sh", "-c", line)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil // subprocess self-reported on stderr
		}
		return err // failed to launch sh, etc.
	}
	return nil
}

// buildImage builds a Docker image from a template directory by shelling out
// to the docker CLI. Isolated so the build backend can change without touching
// callers. Build output is streamed to the operator so a failing Dockerfile
// surfaces its own error, the same way an interactive `docker build` would.
func buildImage(templateDir, imageTag string) error {
	fmt.Fprintf(os.Stderr, "Building image %s from %s...\n", imageTag, templateDir)
	cmd := exec.Command("docker", "build", "--no-cache", "-t", imageTag, templateDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// docker already streamed the build error to stderr; don't rewrap it.
			return fmt.Errorf("docker build failed (exit %d)", exitErr.ExitCode())
		}
		return fmt.Errorf("running docker build: %w", err)
	}
	return nil
}

func (a *App) cmdInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: install <tool_name>")
	}
	name := args[0]
	if _, ok := catalog.Get(name); !ok {
		return fmt.Errorf("unknown tool: %s (run 'tools' to see available)", name)
	}

	destDir := filepath.Join(a.Config.ToolsPath, name)
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("%s already installed — use 'build %s' to rebuild", name, name)
	}

	if err := catalog.Install(name, destDir); err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	fmt.Printf("Template installed: %s\n", destDir)

	// Register the new template so the registry knows its image tag + path.
	if err := a.Tools.DiscoverFromPath(a.Config.ToolsPath); err != nil {
		return fmt.Errorf("register tool: %w", err)
	}
	def, ok := a.Tools.Get(name)
	if !ok {
		return fmt.Errorf("tool %s not found after install", name)
	}

	if def.RunsInDocker() {
		if !a.Runner.DockerReachable() {
			fmt.Printf("%sTemplate installed, but Docker is not available — image not built.%s\n", colorYellow, colorReset)
			fmt.Printf("Start Docker, then run '%sbuild %s%s' to build the image.\n", colorGreen, name, colorReset)
			return nil
		}
		fmt.Printf("%sBuilding image %s...%s\n", colorCyan, def.DockerImage, colorReset)
		if err := buildImage(filepath.Dir(def.TemplatePath), def.DockerImage); err != nil {
			return fmt.Errorf("build image: %w", err)
		}
	}
	fmt.Printf("%s%s installed and ready.%s\n", colorGreen, name, colorReset)
	return nil
}

func (a *App) cmdBuild(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: build <tool_name>")
	}
	name := args[0]
	def, ok := a.Tools.Get(name)
	if !ok {
		return fmt.Errorf("%s is not installed — run 'install %s' first", name, name)
	}
	if !def.RunsInDocker() {
		return fmt.Errorf("%s is a local tool — nothing to build", name)
	}
	if !a.Runner.DockerReachable() {
		return fmt.Errorf("docker is not available — start Docker and retry 'build %s'", name)
	}
	fmt.Printf("%sBuilding image %s...%s\n", colorCyan, def.DockerImage, colorReset)
	if err := buildImage(filepath.Dir(def.TemplatePath), def.DockerImage); err != nil {
		return fmt.Errorf("build image: %w", err)
	}
	fmt.Printf("%s%s rebuilt.%s\n", colorGreen, name, colorReset)
	return nil
}

func (a *App) cmdEvidence(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}
	if len(args) == 0 {
		return a.evidenceList(c)
	}
	switch args[0] {
	case "add":
		return a.evidenceAdd(c, args[1:])
	case "tag":
		return a.evidenceTag(c, args[1:])
	case "verify":
		return a.evidenceVerify(c, args[1:])
	case "classify":
		return a.evidenceClassify(c, args[1:])
	default:
		return fmt.Errorf("usage: evidence [add <path> [--tag a,b] | tag <name> <tag>... | verify [name] | classify [name]]")
	}
}

func (a *App) evidenceAdd(c *cases.Case, args []string) error {
	var src string
	var tags []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--tag" && i+1 < len(args) {
			tags = strings.Split(args[i+1], ",")
			i++
			continue
		}
		src = args[i]
	}
	if src == "" {
		return fmt.Errorf("usage: evidence add <path> [--tag a,b]")
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}
	name := filepath.Base(src)
	dest := filepath.Join(c.Path, "evidence", name)
	srcAbs, _ := filepath.Abs(src)
	destAbs, _ := filepath.Abs(dest)
	sum, err := hashFile(src)
	if err != nil {
		return fmt.Errorf("hash evidence: %w", err)
	}

	// Check for duplicate by hash before copying
	if existing := c.FindEvidenceByHash(sum); existing != nil {
		return fmt.Errorf("evidence already exists: %s (added %s)", existing.Name, existing.AddedAt)
	}

	if srcAbs != destAbs { // external source → copy into evidence/
		if existing, statErr := os.Stat(dest); statErr == nil && existing.Mode().IsRegular() {
			destSum, err := hashFile(dest)
			if err != nil {
				return fmt.Errorf("hash existing evidence: %w", err)
			}
			if destSum != sum {
				return fmt.Errorf("evidence %q already exists with a different hash — refusing to overwrite", name)
			}
			// identical content already present → idempotent, skip copy
		} else if err := copyFile(src, dest); err != nil {
			return fmt.Errorf("copy evidence: %w", err)
		}
	}
	now := time.Now().Format(time.RFC3339)
	if err := c.AppendEvidence(cases.EvidenceRecord{
		Op: "add", Name: name, SHA256: sum, Size: info.Size(), Source: src, Tags: tags, Time: now,
	}); err != nil {
		return err
	}
	if err := c.AppendEvent(cases.TimelineEvent{
		Type: "evidence_added", Timestamp: now,
		Payload: map[string]any{"name": name, "sha256": sum, "tags": tags},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s append timeline: %v\n", colorYellow, colorReset, err)
	}
	short := sum
	if len(short) > 12 {
		short = short[:12]
	}
	fmt.Printf("added evidence %s%s%s  %s\n", colorGreen, name, colorReset, short)

	// Low-friction tool suggestion (G4): point the analyst at the right tool.
	if sug := suggestTool(filepath.Join(c.Path, "evidence", name)); len(sug) > 0 {
		fmt.Printf("%ssuggested tool(s):%s %s\n", colorDim, colorReset, strings.Join(sug, ", "))
	}

	// Opt-in auto-AI analysis of the added evidence (non-blocking).
	if a.AI != nil && a.Config.AI.AutoAnalyzeEvidence {
		go func() {
			analysis, err := a.AI.Summarize(context.Background(), c)
			if err != nil {
				return
			}
			_ = c.AppendEvent(cases.TimelineEvent{
				Type:      "ai_response",
				Timestamp: time.Now().Format(time.RFC3339),
				Payload:   map[string]any{"trigger": "evidence_add", "name": name, "analysis": analysis},
			})
		}()
	}
	return nil
}

func (a *App) evidenceTag(c *cases.Case, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: evidence tag <name> <tag>...")
	}
	name, tags := args[0], args[1:]
	now := time.Now().Format(time.RFC3339)
	if err := c.AppendEvidence(cases.EvidenceRecord{Op: "tag", Name: name, Tags: tags, Time: now}); err != nil {
		return err
	}
	if err := c.AppendEvent(cases.TimelineEvent{
		Type: "evidence_tagged", Timestamp: now,
		Payload: map[string]any{"name": name, "tags": tags},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s append timeline: %v\n", colorYellow, colorReset, err)
	}
	fmt.Printf("tagged %s: %s\n", name, strings.Join(tags, ", "))
	return nil
}

func (a *App) evidenceVerify(c *cases.Case, args []string) error {
	want := ""
	if len(args) > 0 {
		want = args[0]
	}
	ok := true
	checked := 0
	for _, e := range c.Evidence() {
		if want != "" && e.Name != want {
			continue
		}
		checked++
		path := filepath.Join(c.Path, "evidence", e.Name)
		sum, err := hashFile(path)
		if err != nil {
			fmt.Printf("%s%-20s MISSING%s\n", colorRed, e.Name, colorReset)
			ok = false
			continue
		}
		if sum != e.SHA256 {
			fmt.Printf("%s%-20s MISMATCH%s\n", colorRed, e.Name, colorReset)
			ok = false
		} else {
			fmt.Printf("%s%-20s ok%s\n", colorGreen, e.Name, colorReset)
		}
	}
	if want != "" && checked == 0 {
		fmt.Printf("evidence not found: %s\n", want)
		return fmt.Errorf("evidence not found: %s", want)
	}
	if !ok {
		return fmt.Errorf("evidence verification found problems")
	}
	return nil
}

// suggestTool maps an evidence file to the Mimir tool(s) best suited to
// analyze it, by extension + magic bytes. Used by `evidence classify` and
// surfaced automatically on `evidence add` to reduce analyst friction.
func suggestTool(path string) []string {
	name := strings.ToLower(filepath.Base(path))
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".raw"), strings.HasSuffix(lower, ".mem"),
		strings.Contains(lower, "dump"), strings.Contains(lower, "memory"):
		return []string{"volatility"}
	case strings.HasSuffix(lower, ".hive"), strings.Contains(name, "ntuser"),
		strings.Contains(name, "system"), strings.Contains(name, "software"),
		strings.Contains(name, "sam"):
		return []string{"regripper"}
	case strings.HasSuffix(lower, ".dd"), strings.HasSuffix(lower, ".img"),
		strings.HasSuffix(lower, ".e01"), strings.HasSuffix(lower, ".vmdk"),
		strings.HasSuffix(lower, ".raw") == false && (strings.Contains(lower, "disk") || strings.Contains(name, "image")):
		return []string{"sleuthkit"}
	case strings.HasSuffix(lower, ".pcap"), strings.HasSuffix(lower, ".pcapng"):
		return []string{"tshark", "zeek"}
	case strings.HasSuffix(lower, ".evt"), strings.HasSuffix(lower, ".evtx"):
		return []string{"chainsaw"}
	case strings.HasSuffix(lower, ".doc"), strings.HasSuffix(lower, ".docm"),
		strings.HasSuffix(lower, ".xls"), strings.HasSuffix(lower, ".xlsm"),
		strings.HasSuffix(lower, ".ppt"), strings.HasSuffix(lower, ".pptm"),
		strings.HasSuffix(lower, ".rtf"), strings.HasSuffix(lower, ".docx"),
		strings.HasSuffix(lower, ".xlsx"), strings.HasSuffix(lower, ".pptx"):
		return []string{"oletools"}
	case strings.HasSuffix(lower, ".lnk"):
		return []string{"builtin: lnkparse"}
	case strings.HasSuffix(lower, ".exe"), strings.HasSuffix(lower, ".dll"),
		strings.HasSuffix(lower, ".sys"):
		return []string{"capa", "builtin: peinfo"}
	case strings.HasSuffix(lower, ".yar"), strings.HasSuffix(lower, ".yara"):
		return []string{"yara"}
	}
	// Magic-byte fallback.
	if data, err := os.ReadFile(path); err == nil && len(data) >= 4 {
		if bytes.HasPrefix(data, []byte("MZ")) {
			return []string{"capa", "builtin: peinfo"}
		}
		if bytes.HasPrefix(data, []byte("OLE")) {
			return []string{"oletools"}
		}
	}
	return nil
}

func (a *App) evidenceClassify(c *cases.Case, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	var path string
	if name == "" {
		evs := c.Evidence()
		if len(evs) == 0 {
			return fmt.Errorf("no evidence to classify — add a file first")
		}
		path = filepath.Join(c.Path, "evidence", evs[len(evs)-1].Name)
		name = evs[len(evs)-1].Name
	} else {
		path = filepath.Join(c.Path, "evidence", name)
	}
	sug := suggestTool(path)
	if len(sug) == 0 {
		fmt.Printf("%s%s%s — no specific tool suggestion (generic analysis: ioc, search).\n", colorCyan, name, colorReset)
		return nil
	}
	fmt.Printf("%s%s%s → suggested tool(s): %s\n", colorCyan, name, colorReset, strings.Join(sug, ", "))
	return nil
}

func (a *App) evidenceList(c *cases.Case) error {
	ev := c.Evidence()
	if len(ev) == 0 {
		fmt.Println("No evidence tracked. Add with: evidence add <path>")
		return nil
	}
	tbl := ui.Table{
		Headers: []string{"NAME", "SHA256", "SIZE", "TAGS"},
		Align:   []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignRight, ui.AlignLeft},
	}
	for _, e := range ev {
		short := e.SHA256
		if len(short) > 12 {
			short = short[:12]
		}
		tbl.Rows = append(tbl.Rows, []string{e.Name, short, fmt.Sprintf("%d", e.Size), strings.Join(e.Tags, ",")})
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// splitArgs splits a command line string into arguments, respecting quotes.
func splitArgs(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(ch)
			}
		} else if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
		} else if ch == ' ' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// captureInto appends a tool_run timeline event, auto-extracts IOCs from
// the output, and (for Volatility plugins) records plugin-specific
// hints. It is the SINGLE capture path: both the dedicated
// `volatility` command and the generic `sandbox run -- <tool>` path
// call it, so every tool benefits uniformly (fixes the gap where only
// the volatility command recorded findings — G2/G6 in docs/gap-analysis.md).
//
// tool is the tool label (e.g. "volatility", "icat"), command is the
// full command line, dump is the source evidence name (may be ""), and
// out is the captured stdout.
func captureInto(c *cases.Case, tool, command, dump, out string) error {
	if c == nil {
		return fmt.Errorf("captureInto: nil case")
	}
	now := time.Now().Format(time.RFC3339)

	if err := c.AppendEvent(cases.TimelineEvent{
		Type:      "tool_run",
		Timestamp: now,
		Payload: map[string]any{
			"tool":    tool,
			"command": command,
			"dump":    dump,
			"output":  truncate(out, 2000),
		},
	}); err != nil {
		return fmt.Errorf("append timeline: %w", err)
	}

	// Generic IOCs.
	inds := ioc.Extract([]byte(out))
	for _, ind := range inds {
		if err := c.AppendIOC(cases.IOCRecord{
			Type:   ind.Type,
			Value:  ind.Value,
			Source: tool + ":" + command + dumpSuffix(dump),
			Time:   now,
		}); err != nil {
			return fmt.Errorf("append ioc: %w", err)
		}
	}

	// capa ATT&CK-mapped capability hints.
	if tool == "capa" {
		for _, h := range capaHints(out) {
			if err := c.AppendIOC(cases.IOCRecord{
				Type:   h.typ,
				Value:  h.val,
				Source: tool + ":" + command + dumpSuffix(dump),
				Time:   now,
			}); err != nil {
				return fmt.Errorf("append capa hint: %w", err)
			}
		}
	}

	// Volatility plugin-aware hints (commands / NTLM hashes).
	if strings.Contains(strings.ToLower(command), "vol ") || strings.ToLower(tool) == "volatility" {
		for _, h := range volatilityHints(command, out) {
			if err := c.AppendIOC(cases.IOCRecord{
				Type:   h.typ,
				Value:  h.val,
				Source: tool + ":" + command + dumpSuffix(dump),
				Time:   now,
			}); err != nil {
				return fmt.Errorf("append hint: %w", err)
			}
		}
	}

	// Network tool-aware hints (zeek conn logs / tshark -T fields):
	// surface src/dst hosts + URIs the generic regex would miss.
	lowCmd := strings.ToLower(command)
	if strings.Contains(lowCmd, "zeek") || strings.Contains(lowCmd, "tshark") {
		for _, h := range networkHints(command, out) {
			if err := c.AppendIOC(cases.IOCRecord{
				Type:   h.typ,
				Value:  h.val,
				Source: tool + ":" + command + dumpSuffix(dump),
				Time:   now,
			}); err != nil {
				return fmt.Errorf("append net-hint: %w", err)
			}
		}
	}

	// SleuthKit tool-aware hints (mmls partitions / fls deleted entries).
	if tool == "sleuthkit" {
		for _, h := range diskHints(strings.Fields(command)[0], out) {
			if err := c.AppendIOC(cases.IOCRecord{
				Type:   h.typ,
				Value:  h.val,
				Source: tool + ":" + command + dumpSuffix(dump),
				Time:   now,
			}); err != nil {
				return fmt.Errorf("append disk-hint: %w", err)
			}
		}
	}

	// RegRipper hints (USB devices, autorun/service paths, Run keys).
	if tool == "regripper" {
		for _, h := range regripperHints(out) {
			if err := c.AppendIOC(cases.IOCRecord{
				Type:   h.typ,
				Value:  h.val,
				Source: tool + ":" + command + dumpSuffix(dump),
				Time:   now,
			}); err != nil {
				return fmt.Errorf("append reg-hint: %w", err)
			}
		}
	}
	return nil
}

// dumpSuffix renders the dump name for an IOC Source string.
func dumpSuffix(dump string) string {
	if dump == "" {
		return ""
	}
	return " (" + dump + ")"
}

// volNetHint is a network indicator extracted from zeek/tshark output.
type volNetHint struct {
	typ string
	val string
}

// networkHints extracts network indicators the generic IOC regex
// would miss: zeek conn.log src/dst hosts + tshark -T fields
// (uri / http.host / dns.qry.name). Returns typed indicators.
func networkHints(command, out string) []volNetHint {
	var hints []volNetHint
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		// zeek conn.log: tab-separated, fields include src/dst IP + ports.
		if strings.Contains(t, "	") {
			for _, f := range strings.Split(t, "	") {
				if reIP.MatchString(f) {
					hints = append(hints, volNetHint{typ: "ip", val: f})
				}
			}
			continue
		}
		// tshark -T fields -e http.host / -e dns.qry.name / -e http.request.uri
		if strings.HasPrefix(t, "http.host") || strings.HasPrefix(t, "dns.qry.name") {
			parts := strings.SplitN(t, " ", 2)
			if len(parts) == 2 && parts[1] != "" {
				hints = append(hints, volNetHint{typ: "domain", val: strings.TrimSpace(parts[1])})
			}
		}
		if strings.HasPrefix(t, "http.request.uri") {
			parts := strings.SplitN(t, " ", 2)
			if len(parts) == 2 && parts[1] != "" {
				hints = append(hints, volNetHint{typ: "url", val: strings.TrimSpace(parts[1])})
			}
		}
	}
	return hints
}
