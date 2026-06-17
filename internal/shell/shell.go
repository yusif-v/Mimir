// Package shell provides the interactive REPL.
package shell

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/yusif-v/mimir/internal/cases"
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

func (a *App) cmdTools(args []string) error {
	toolList := a.Tools.List("")
	if len(toolList) == 0 {
		fmt.Println("No tools registered.")
		return nil
	}
	for _, t := range toolList {
		mode := "local"
		modeColor := colorCyan
		if t.RunsInDocker() {
			mode = "docker"
			modeColor = colorYellow
		}
		fmt.Printf("  %s%-20s%s [%s%-6s%s] %s\n",
			colorGreen, t.Name, colorReset,
			modeColor, mode, colorReset,
			t.Description)
	}
	return nil
}

func (a *App) cmdRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: run <tool_name> [args...]")
	}

	toolName := args[0]
	toolArgs := args[1:]

	tool, ok := a.Tools.Get(toolName)
	if !ok {
		return fmt.Errorf("tool not found: %s", toolName)
	}

	casePath := ""
	if c := a.Cases.Current(); c != nil {
		casePath = c.Path
	}

	result := a.Runner.Run(tool, toolArgs, casePath)
	if result.Success() {
		if result.Stdout != "" {
			fmt.Print(result.Stdout)
		}
		if c := a.Cases.Current(); c != nil {
			c.AddToolUsage(toolName)
			c.Save()
		}
	} else if result.Error != nil {
		return result.Error
	}
	return nil
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
	c.AddNote(content, "analyst")
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
