// Package shell provides the interactive REPL.
package shell

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
)

// App ties together all subsystems.
type App struct {
	Config    *config.Config
	Events    *events.Bus
	Cases     *cases.Manager
	Tools     *tools.Registry
	Runner    *tools.Runner
	Output    *tools.OutputCapture
	CurrentCase *cases.Case
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
	fmt.Println("Mimir v0.1.0 — DFIR shell. Type 'help' for commands, 'exit' to quit.")

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
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
		return fmt.Sprintf("[%s][mimir][%s] |> ", username, a.Cases.Current().Name)
	}
	return fmt.Sprintf("[%s][mimir] |> ", username)
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
	fmt.Println("Built-in commands:")
	cmds := []string{"help", "exit", "status", "case", "cases", "tools", "run", "use", "note", "clear"}
	for _, c := range cmds {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

func (a *App) cmdStatus(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		fmt.Println("No case is open.")
		return nil
	}
	fmt.Printf("Case: %s (%s)\n", c.Name, c.Status)
	fmt.Printf("Path: %s\n", c.Path)
	fmt.Printf("Tools used: %s\n", strings.Join(c.ToolsUsed, ", "))
	fmt.Printf("Notes: %d\n", len(c.Notes))
	return nil
}

func (a *App) cmdCase(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: case -n <name> | case -o <name> | case -c")
	}

	action := args[0]
	name := args[1]

	switch action {
	case "-n":
		c, err := a.Cases.Create(name)
		if err != nil {
			return err
		}
		fmt.Printf("Case created: %s\n", c.Path)
	case "-o":
		c, err := a.Cases.Open(name)
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
	cases, err := a.Cases.List()
	if err != nil {
		return err
	}
	if len(cases) == 0 {
		fmt.Println("No cases found.")
		return nil
	}
	for _, c := range cases {
		fmt.Printf("  [%6s] %s  (%s)\n", c.Status, c.Name, c.Path)
	}
	return nil
}

func (a *App) cmdTools(args []string) error {
	tools := a.Tools.List("")
	if len(tools) == 0 {
		fmt.Println("No tools registered.")
		return nil
	}
	for _, t := range tools {
		mode := "local"
		if t.RunsInDocker() {
			mode = "docker"
		}
		fmt.Printf("  %-20s [%-6s] %s\n", t.Name, mode, t.Description)
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
	fmt.Printf("Selected tool: %s — %s\n", tool.Name, tool.Description)
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
	fmt.Print("\033[H\033[2J")
	return nil
}

func (a *App) cmdShell(line string) error {
	// Shell passthrough — TODO: implement os/exec
	return fmt.Errorf("shell passthrough not yet implemented: %s", line)
}

// splitArgs splits a command line string into arguments.
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
