// Package tools manages tool registration, execution, and output capture.
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yusif-v/mimir/internal/events"
)

// Definition represents a registered tool.
type Definition struct {
	Name         string            `json:"name" toml:"name"`
	Description  string            `json:"description" toml:"description"`
	Category     string            `json:"category" toml:"category"`
	DockerImage  string            `json:"docker_image,omitempty" toml:"docker_image"`
	LocalCmd     string            `json:"local_cmd,omitempty" toml:"local_cmd"`
	TemplatePath string            `json:"template_path,omitempty"`
	Tags         []string          `json:"tags" toml:"tags"`
	Metadata     map[string]string `json:"metadata" toml:"metadata"`
}

// Registry holds all registered tools.
type Registry struct {
	events *events.Bus
	tools  map[string]*Definition
}

// RunsInDocker returns true if this tool uses a Docker image.
func (d *Definition) RunsInDocker() bool {
	return d.DockerImage != ""
}

// NewRegistry creates a new tool registry.
func NewRegistry(bus *events.Bus) *Registry {
	return &Registry{
		events: bus,
		tools:  make(map[string]*Definition),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t *Definition) {
	r.tools[t.Name] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (*Definition, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all tools, optionally filtered by category.
func (r *Registry) List(category string) []*Definition {
	var result []*Definition
	for _, t := range r.tools {
		if category == "" || t.Category == category {
			result = append(result, t)
		}
	}
	return result
}

// DiscoverFromPath scans a directory for tool templates.
func (r *Registry) DiscoverFromPath(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		templatePath := filepath.Join(path, entry.Name(), "mimir.toml")
		if _, err := os.Stat(templatePath); err == nil {
			// TODO: parse TOML template
			_ = templatePath
		}
	}
	return nil
}

// Runner executes tools locally or in Docker.
type Runner struct {
	events *events.Bus
}

// NewRunner creates a new tool runner.
func NewRunner(bus *events.Bus) *Runner {
	return &Runner{events: bus}
}

// Run executes a tool with the given arguments.
func (r *Runner) Run(tool *Definition, args []string, casePath string) *Result {
	r.events.Emit(events.ToolStarted, map[string]any{
		"tool": tool.Name,
		"args": args,
	})

	result := &Result{
		Tool:      tool.Name,
		Args:      args,
		StartedAt: time.Now(),
	}

	if tool.DockerImage != "" {
		result.Error = fmt.Errorf("docker execution not yet implemented")
		r.events.Emit(events.ToolError, map[string]any{
			"tool":  tool.Name,
			"error": result.Error,
		})
		return result
	}

	if tool.LocalCmd != "" {
		result.Error = fmt.Errorf("local execution not yet implemented")
		result.FinishedAt = time.Now()
		r.events.Emit(events.ToolError, map[string]any{
			"tool":  tool.Name,
			"error": result.Error,
		})
		return result
	}

	result.Error = fmt.Errorf("no execution method for tool '%s'", tool.Name)
	result.FinishedAt = time.Now()
	return result
}

// Result holds the outcome of a tool run.
type Result struct {
	Tool       string
	Args       []string
	Stdout     string
	Stderr     string
	Error      error
	ReturnCode int
	StartedAt  time.Time
	FinishedAt time.Time
}

// Success returns true if the tool ran without errors.
func (r *Result) Success() bool {
	return r.Error == nil && r.ReturnCode == 0
}

// Duration returns how long the tool ran.
func (r *Result) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}

// OutputCapture records tool output to case directories.
type OutputCapture struct {
	events *events.Bus
}

// NewOutputCapture creates a new output capture.
func NewOutputCapture(bus *events.Bus) *OutputCapture {
	return &OutputCapture{events: bus}
}

// Record saves tool output to the case output directory.
func (oc *OutputCapture) Record(toolName, output, casePath string) error {
	outputDir := filepath.Join(casePath, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.txt", toolName, timestamp)
	outputFile := filepath.Join(outputDir, filename)

	if err := os.WriteFile(outputFile, []byte(output), 0644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	oc.events.Emit(events.OutputCaptured, map[string]any{
		"tool": toolName,
		"path": outputFile,
	})
	return nil
}
