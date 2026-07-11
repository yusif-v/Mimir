// Package sandbox manages Docker-based isolated environments for file analysis.
package sandbox

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// Shell provides CLI command handlers for sandbox operations.
type Shell struct {
	manager *Manager
	runtime *Runtime
	// Capture, if set, is called after a tool runs inside a sandbox
	// so the host (the App) can append a timeline event + auto-IOC.
	// It is wired by the App to the current case. Optional.
	Capture func(tool, command, output string) error
}

// NewShell creates a new Shell command handler.
func NewShell(manager *Manager, runtime *Runtime) *Shell {
	return &Shell{
		manager: manager,
		runtime: runtime,
	}
}

// Create handles "sandbox create" command.
func (sh *Shell) Create(w io.Writer, args []string) error {
	var os, image, targetFile string
	timeout := 300

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--os":
			if i+1 < len(args) {
				os = args[i+1]
				i++
			}
		case "--image":
			if i+1 < len(args) {
				image = args[i+1]
				i++
			}
		case "--file":
			if i+1 < len(args) {
				targetFile = args[i+1]
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &timeout)
				i++
			}
		}
	}

	if os == "" {
		os = "ubuntu-22.04"
	}
	if image == "" {
		image = fmt.Sprintf("mimir-sandbox:%s", os)
	}

	s := sh.manager.Create("current-case", os, image, targetFile, timeout)

	// Try to create Docker container
	if err := sh.runtime.CreateContainer(s); err != nil {
		fmt.Fprintf(w, "Warning: %v\n", err)
		fmt.Fprintf(w, "Sandbox created (ID: %s) but container could not be started.\n", s.ID)
	} else {
		fmt.Fprintf(w, "Sandbox created: %s (OS: %s, Image: %s)\n", s.ID, s.OS, s.Image)
		if targetFile != "" {
			fmt.Fprintf(w, "Target file: %s\n", targetFile)
		}
	}

	sh.manager.Update(s)
	return nil
}

// Run handles "sandbox run" command.
// Usage:
//
//	sandbox run <id>            -> runs the preset TargetFile
//	sandbox run <id> -- <cmd>   -> runs an arbitrary command/args in the container
func (sh *Shell) Run(w io.Writer, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sandbox run <id> [-- <command> [args...]]")
	}
	id := args[0]
	rest := args[1:]

	// Split optional "--" separator for an inline command.
	var inlineCmd string
	if i := indexOf(rest, "--"); i >= 0 {
		parts := rest[i+1:]
		inlineCmd = strings.Join(parts, " ")
		rest = rest[:i]
	}

	s, ok := sh.manager.Get(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}

	if s.ContainerID == "" {
		return fmt.Errorf("sandbox has no container — create it first")
	}

	s.Status = SandboxRunning
	sh.manager.Update(s)

	// Start capture
	if err := sh.runtime.StartCapture(s.ContainerID); err != nil {
		fmt.Fprintf(w, "Warning: capture failed to start: %v\n", err)
	}

	if inlineCmd != "" {
		// Inline command path: run arbitrary args via sh -c.
		fmt.Fprintf(w, "Running in sandbox %s: %s\n", s.ID, inlineCmd)
		output, err := sh.runtime.Exec(s.ContainerID, inlineCmd)
		if err != nil {
			fmt.Fprintf(w, "Execution error: %v\n", err)
		}
		fmt.Fprintf(w, "Output:\n%s\n", output)
		// Generalize capture (G2/G6): every sandbox tool run appends a
		// tool_run timeline event + auto-IOC, the same way the dedicated
		// `volatility` command does.
		if sh.Capture != nil {
			tool := firstWord(inlineCmd)
			if cerr := sh.Capture(tool, inlineCmd, output); cerr != nil {
				fmt.Fprintf(w, "Warning: capture failed: %v\n", cerr)
			}
		}
	} else if s.TargetFile != "" {
		// Preset target-file path.
		fmt.Fprintf(w, "Executing %s in sandbox %s...\n", s.TargetFile, s.ID)
		output, err := sh.runtime.RunTarget(s.ContainerID, s.TargetFile)
		if err != nil {
			fmt.Fprintf(w, "Execution error: %v\n", err)
		}
		fmt.Fprintf(w, "Output:\n%s\n", output)
	} else {
		fmt.Fprintf(w, "Sandbox %s has no target file and no command given.\n", s.ID)
	}

	s.Status = SandboxStopped
	s.DestroyedAt = time.Now().Format(time.RFC3339)
	sh.manager.Update(s)

	fmt.Fprintf(w, "Sandbox %s finished.\n", s.ID)
	return nil
}

// indexOf returns the position of the first "needle" in hay, or -1.
func indexOf(hay []string, needle string) int {
	for i, v := range hay {
		if v == needle {
			return i
		}
	}
	return -1
}

// firstWord returns the first whitespace-separated token of s.
func firstWord(s string) string {
	for i, r := range s {
		if r == ' ' || r == '	' || r == '\n' {
			return s[:i]
		}
	}
	return s
}

// Shell handles "sandbox shell" command — opens interactive shell in container.
func (sh *Shell) Shell(w io.Writer, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sandbox shell <id>")
	}
	id := args[0]
	s, ok := sh.manager.Get(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	if s.ContainerID == "" {
		return fmt.Errorf("sandbox has no container")
	}
	fmt.Fprintf(w, "Connecting to sandbox %s (Ctrl+D to detach)...\n", s.ID)
	return sh.runtime.Shell(s.ContainerID)
}

// Logs handles "sandbox logs" command.
func (sh *Shell) Logs(w io.Writer, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sandbox logs <id>")
	}
	id := args[0]
	s, ok := sh.manager.Get(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	logs, err := sh.runtime.GetLogs(s.ContainerID)
	if err != nil {
		return err
	}
	fmt.Fprint(w, logs)
	return nil
}

// Report handles "sandbox report" command.
func (sh *Shell) Report(w io.Writer, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sandbox report <id>")
	}
	id := args[0]
	s, ok := sh.manager.Get(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	fmt.Fprintf(w, "=== Sandbox Report: %s ===\n", s.ID)
	fmt.Fprintf(w, "OS:          %s\n", s.OS)
	fmt.Fprintf(w, "Image:       %s\n", s.Image)
	fmt.Fprintf(w, "Status:      %s\n", s.Status)
	fmt.Fprintf(w, "Created:     %s\n", s.CreatedAt)
	fmt.Fprintf(w, "Target:      %s\n", s.TargetFile)
	fmt.Fprintf(w, "Files:       %d\n", len(s.Files))
	fmt.Fprintf(w, "Network:     %s\n", s.Network)
	fmt.Fprintf(w, "Timeout:     %ds\n", s.Timeout)
	return nil
}

// CopyFile handles "sandbox cp" command.
func (sh *Shell) CopyFile(w io.Writer, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: sandbox cp <file> <sandbox-id>")
	}
	srcPath := args[0]
	id := args[1]
	s, ok := sh.manager.Get(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	destPath := "/tmp/" + filepath.Base(srcPath)
	if err := sh.runtime.CopyFile(s.ContainerID, srcPath, destPath); err != nil {
		return err
	}
	s.Files = append(s.Files, srcPath)
	sh.manager.Update(s)
	fmt.Fprintf(w, "Copied %s -> %s:%s\n", srcPath, id, destPath)
	return nil
}

// Destroy handles "sandbox destroy" command.
func (sh *Shell) Destroy(w io.Writer, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sandbox destroy <id>")
	}
	id := args[0]
	s, ok := sh.manager.Get(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	if s.ContainerID != "" {
		if err := sh.runtime.StopContainer(s.ContainerID); err != nil {
			fmt.Fprintf(w, "Warning: stop failed: %v\n", err)
		}
		if err := sh.runtime.RemoveContainer(s.ContainerID); err != nil {
			fmt.Fprintf(w, "Warning: remove failed: %v\n", err)
		}
	}
	if err := sh.manager.Destroy(id); err != nil {
		return err
	}
	fmt.Fprintf(w, "Sandbox %s destroyed.\n", id)
	return nil
}

// List handles "sandbox list" command.
func (sh *Shell) List(w io.Writer, args []string) error {
	sandboxes := sh.manager.List("current-case")
	if len(sandboxes) == 0 {
		fmt.Fprintln(w, "No active sandboxes.")
		return nil
	}
	for _, s := range sandboxes {
		fmt.Fprintf(w, "%-20s %-12s %-8s %s\n", s.ID, s.OS, s.Status, s.CreatedAt)
	}
	return nil
}

// Dispatch routes sandbox subcommands to their handlers.
func (sh *Shell) Dispatch(w io.Writer, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sandbox <create|run|shell|logs|report|cp|destroy|list> [args...]")
	}
	switch args[0] {
	case "create":
		return sh.Create(w, args[1:])
	case "run":
		return sh.Run(w, args[1:])
	case "shell":
		return sh.Shell(w, args[1:])
	case "logs":
		return sh.Logs(w, args[1:])
	case "report":
		return sh.Report(w, args[1:])
	case "cp":
		return sh.CopyFile(w, args[1:])
	case "destroy":
		return sh.Destroy(w, args[1:])
	case "list":
		return sh.List(w, args[1:])
	default:
		return fmt.Errorf("unknown sandbox command: %s", strings.Join(args, " "))
	}
}
