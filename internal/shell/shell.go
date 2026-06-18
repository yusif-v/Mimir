// Package shell provides the interactive REPL.
package shell

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
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

// App ties together all subsystems.
type App struct {
	Config  *config.Config
	Events  *events.Bus
	Cases   *cases.Manager
	Tools   *tools.Registry
	Runner  *tools.Runner
	Output  *tools.OutputCapture
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
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("%sMimir v0.1.0%s — DFIR shell. Type '%shelp%s' for commands, '%sexit%s' to quit.\n",
		colorCyan, colorReset, colorGreen, colorReset, colorGreen, colorReset)

	for {
		prompt := a.buildPrompt()
		fmt.Print(prompt)

		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := a.dispatch(line); err != nil {
			if err.Error() == "exit" {
				fmt.Println("Exiting Mimir...")
				return nil
			}
			fmt.Fprintf(os.Stderr, "%serror:%s %v\n", colorRed, colorReset, err)
		}
	}

	return scanner.Err()
}

func (a *App) buildPrompt() string {
	currentUser, _ := user.Current()
	username := currentUser.Username
	if username == "" {
		username = "user"
	}

	if a.Cases.Current() != nil {
		return fmt.Sprintf("%s[%s]%s%s[mimir]%s%s[%s]%s |> ",
			colorGreen, username, colorReset,
			colorCyan, colorReset,
			colorYellow, a.Cases.Current().Name, colorReset,
		)
	}
	return fmt.Sprintf("%s[%s]%s%s[mimir]%s |> ",
		colorGreen, username, colorReset,
		colorCyan, colorReset,
	)
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
	if !result.Success() && result.Stderr != "" {
		payload["stderr"] = result.Stderr
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

func (a *App) cmdClear(args []string) error {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	return nil
}

func (a *App) cmdShell(line string) error {
	// Shell passthrough
	cmd := exec.Command("sh", "-c", line)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
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
