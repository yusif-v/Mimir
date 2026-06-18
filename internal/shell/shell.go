// Package shell provides the interactive REPL.
package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
	"github.com/yusif-v/mimir/internal/ui"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
)

// Version is the Mimir release version, shown in the startup banner.
const Version = "0.3.0"

func banner() string {
	return fmt.Sprintf("%sMimir v%s%s — DFIR shell. Type '%shelp%s' for commands, '%sexit%s' to quit.",
		colorCyan, Version, colorReset, colorGreen, colorReset, colorGreen, colorReset)
}

// App ties together all subsystems.
type App struct {
	Config *config.Config
	Events *events.Bus
	Cases  *cases.Manager
	Tools  *tools.Registry
	Runner *tools.Runner
	Output *tools.OutputCapture
	rl     *readline.Instance
}

// NewApp creates a new shell app with all subsystems wired.
func NewApp(cfg *config.Config) *App {
	bus := events.NewBus()
	caseManager := cases.NewManager(cfg.CasesPath, bus)
	toolRegistry := tools.NewRegistry(bus)
	toolRunner := tools.NewRunner(bus)
	outputCapture := tools.NewOutputCapture(bus)

	// Discover tools from the tools directory
	if err := toolRegistry.DiscoverFromPath(cfg.ToolsPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: tool discovery failed: %v\n", err)
	}

	return &App{
		Config: cfg,
		Events: bus,
		Cases:  caseManager,
		Tools:  toolRegistry,
		Runner: toolRunner,
		Output: outputCapture,
	}
}

// Run starts the interactive REPL.
func (a *App) Run() error {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            a.buildPrompt(),
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
		fmt.Println(a.contextLine())
		a.rl.SetPrompt(a.buildPrompt())

		line, err := a.rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
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

// colorize wraps s unless NO_COLOR is set.
func colorize(s, color string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color + s + colorReset
}

// user_Current returns the current OS username.
func user_Current() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// contextLine is the Starship-style segment line printed above the input marker.
func (a *App) contextLine() string {
	u := "user"
	if name, err := user_Current(); err == nil && name != "" {
		u = name
	}
	ascii := asciiMode()
	seg := func(icon, text, color string) string {
		if ascii {
			return text
		}
		return colorize(icon+" "+text, color)
	}

	parts := []string{
		seg("", u, colorGreen),
		seg("", "mimir", colorCyan),
	}
	if c := a.Cases.Current(); c != nil {
		parts = append(parts, seg("", c.Name, colorYellow))
		status := "open"
		scol := colorGreen
		if c.Status != "open" {
			status, scol = "closed", colorDim
		}
		parts = append(parts, seg("", status, scol))
	}
	sep := "  "
	if ascii {
		sep = " · "
	}
	return strings.Join(parts, sep)
}

func (a *App) buildPrompt() string {
	if asciiMode() {
		return "|> "
	}
	return colorize("❯", colorGreen) + " "
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
	case "clear":
		return a.cmdClear(args)
	default:
		return a.cmdShell(line)
	}
}

func (a *App) cmdHelp(args []string) error {
	fmt.Printf("  %shelp%s       show this help\n", colorGreen, colorReset)
	fmt.Printf("  %sexit%s       exit Mimir\n", colorGreen, colorReset)
	fmt.Printf("  %sstatus%s     show current case status\n", colorGreen, colorReset)
	fmt.Printf("  %scase%s       manage cases (-n new, -o open, -c close)\n", colorGreen, colorReset)
	fmt.Printf("  %scases%s      list all cases\n", colorGreen, colorReset)
	fmt.Printf("  %stools%s      list registered tools\n", colorGreen, colorReset)
	fmt.Printf("  %srun%s        run a tool: run <name> [args...]\n", colorGreen, colorReset)
	fmt.Printf("  %sinstall%s    install a tool from the catalog: install <name>\n", colorGreen, colorReset)
	fmt.Printf("  %sbuild%s      (re)build an installed tool's image: build <name>\n", colorGreen, colorReset)
	fmt.Printf("  %suse%s        select a tool: use <name>\n", colorGreen, colorReset)
	fmt.Printf("  %snote%s       add a note to current case\n", colorGreen, colorReset)
	fmt.Printf("  %sevidence%s   manage evidence: add <path> [--tag a,b], tag, verify\n", colorGreen, colorReset)
	fmt.Printf("  %stimeline%s   show case timeline (-n N tails last N)\n", colorGreen, colorReset)
	fmt.Printf("  %sclear%s      clear screen\n", colorGreen, colorReset)
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
		return fmt.Errorf("usage: case -n <name> | case -o <name> | case -c")
	}

	action := args[0]

	switch action {
	case "-n":
		if len(args) < 2 {
			return fmt.Errorf("usage: case -n <name>")
		}
		c, err := a.Cases.Create(args[1])
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
	if len(allCases) == 0 {
		fmt.Println("No cases found.")
		return nil
	}
	for _, c := range allCases {
		statusColor := colorGreen
		if c.Status == "closed" {
			statusColor = colorDim
		}
		fmt.Printf("  %s[%s]%s %s  (%s)\n", statusColor, c.Status, colorReset, c.Name, c.Path)
	}
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
		for _, t := range installed {
			mode := "local"
			if t.RunsInDocker() {
				mode = "docker"
			}
			label, color := statusBadge(statuses[t.Name])
			fmt.Printf("  %s%-16s%s [%s] %s%-10s%s %s\n",
				colorGreen, t.Name, colorReset,
				mode,
				color, label, colorReset,
				t.Description)
		}
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
		for _, e := range available {
			fmt.Printf("  %s%-16s%s %-12s %s\n",
				colorGreen, e.Name, colorReset,
				e.Category, e.Description)
		}
	}

	// BUILT-IN section: native-Go tools, always ready.
	bi := builtins.List()
	if len(bi) > 0 {
		fmt.Printf("\n%sBUILT-IN%s\n", colorCyan, colorReset)
		for _, m := range bi {
			fmt.Printf("  %s%-16s%s [builtin] %sready%s      %s\n",
				colorGreen, m.Name, colorReset,
				colorGreen, colorReset,
				m.Description)
		}
	}

	// docker footer
	if dockerUp {
		fmt.Printf("\ndocker: %srunning%s\n", colorGreen, colorReset)
	} else {
		fmt.Printf("\ndocker: %snot available%s\n", colorRed, colorReset)
	}
	return nil
}

func (a *App) cmdRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: run <tool_name> [args...]")
	}

	toolName := args[0]
	toolArgs := args[1:]

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

func (a *App) cmdClear(args []string) error {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	return nil
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
// callers.
func buildImage(templateDir, imageTag string) error {
	cmd := exec.Command("docker", "build", "-t", imageTag, templateDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	default:
		return fmt.Errorf("usage: evidence [add <path> [--tag a,b] | tag <name> <tag>... | verify [name]]")
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
	if srcAbs != destAbs { // external → copy in
		if existing, err := os.Stat(dest); err == nil && existing.Mode().IsRegular() {
			if h1, _ := hashFile(src); func() string { h2, _ := hashFile(dest); return h2 }() != h1 {
				return fmt.Errorf("evidence %q already exists with a different hash — refusing to overwrite", name)
			}
		}
		if err := copyFile(src, dest); err != nil {
			return fmt.Errorf("copy evidence: %w", err)
		}
	}
	sum, err := hashFile(dest)
	if err != nil {
		return fmt.Errorf("hash evidence: %w", err)
	}
	now := time.Now().Format(time.RFC3339)
	if err := c.AppendEvidence(cases.EvidenceRecord{
		Op: "add", Name: name, SHA256: sum, Size: info.Size(), Source: src, Tags: tags, Time: now,
	}); err != nil {
		return err
	}
	_ = c.AppendEvent(cases.TimelineEvent{
		Type: "evidence_added", Timestamp: now,
		Payload: map[string]any{"name": name, "sha256": sum, "tags": tags},
	})
	fmt.Printf("added evidence %s%s%s  %s\n", colorGreen, name, colorReset, sum[:12])
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
	_ = c.AppendEvent(cases.TimelineEvent{
		Type: "evidence_tagged", Timestamp: now,
		Payload: map[string]any{"name": name, "tags": tags},
	})
	fmt.Printf("tagged %s: %s\n", name, strings.Join(tags, ", "))
	return nil
}

func (a *App) evidenceVerify(c *cases.Case, args []string) error {
	want := ""
	if len(args) > 0 {
		want = args[0]
	}
	ok := true
	for _, e := range c.Evidence() {
		if want != "" && e.Name != want {
			continue
		}
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
	if !ok {
		return fmt.Errorf("evidence verification found problems")
	}
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
