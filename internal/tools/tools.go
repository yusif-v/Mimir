// Package tools manages tool registration, execution, and output capture.
package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/pelletier/go-toml/v2"

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
	// Parsed from mimir.toml
	Volumes    []VolumeMapping `toml:"-"`
	Entrypoint string          `toml:"-"`
	WorkDir    string          `toml:"-"`
}

// VolumeMapping maps a host directory to a container directory.
type VolumeMapping struct {
	Host      string `toml:"host"`
	Container string `toml:"container"`
	Mode      string `toml:"mode"` // "ro" or "rw"
}

// ToolTemplate is the mimir.toml format.
type ToolTemplate struct {
	Tool struct {
		Name        string   `toml:"name"`
		Description string   `toml:"description"`
		Category    string   `toml:"category"`
		Tags        []string `toml:"tags"`
	} `toml:"tool"`
	Docker struct {
		Image      string `toml:"image"`
		Entrypoint string `toml:"entrypoint"`
		WorkDir    string `toml:"workdir"`
		Volumes    []struct {
			Host      string `toml:"host"`
			Container string `toml:"container"`
			Mode      string `toml:"mode"`
		} `toml:"volumes"`
	} `toml:"docker"`
}

// RunsInDocker returns true if this tool uses a Docker image.
func (d *Definition) RunsInDocker() bool {
	return d.DockerImage != ""
}

// Registry holds all registered tools.
type Registry struct {
	events *events.Bus
	tools  map[string]*Definition
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

// DiscoverFromPath scans a directory for tool templates (mimir.toml files).
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
		data, err := os.ReadFile(templatePath)
		if err != nil {
			continue
		}

		def, err := ParseTemplate(data, templatePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", templatePath, err)
			continue
		}
		r.Register(def)
	}
	return nil
}

// ParseTemplate parses a mimir.toml file into a Definition.
func ParseTemplate(data []byte, path string) (*Definition, error) {
	var tmpl ToolTemplate
	if err := toml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("parse toml: %w", err)
	}

	def := &Definition{
		Name:        tmpl.Tool.Name,
		Description: tmpl.Tool.Description,
		Category:    tmpl.Tool.Category,
		DockerImage: tmpl.Docker.Image,
		Tags:        tmpl.Tool.Tags,
		TemplatePath: path,
		Entrypoint:  tmpl.Docker.Entrypoint,
		WorkDir:     tmpl.Docker.WorkDir,
		Metadata:    map[string]string{},
	}

	for _, v := range tmpl.Docker.Volumes {
		mode := v.Mode
		if mode == "" {
			mode = "rw"
		}
		def.Volumes = append(def.Volumes, VolumeMapping{
			Host:      v.Host,
			Container: v.Container,
			Mode:      mode,
		})
	}

	return def, nil
}

// Runner executes tools locally or in Docker.
type Runner struct {
	events      *events.Bus
	docker      *client.Client
	dockerAvail bool
}

// NewRunner creates a new tool runner.
func NewRunner(bus *events.Bus) *Runner {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	dockerAvail := err == nil

	return &Runner{
		events:      bus,
		docker:      cli,
		dockerAvail: dockerAvail,
	}
}

// DockerAvailable returns true if a Docker client was constructed. Note this
// reflects client setup only, not daemon reachability — use DockerReachable to
// confirm the daemon is actually running.
func (r *Runner) DockerAvailable() bool {
	return r.dockerAvail
}

// DockerReachable pings the Docker daemon to confirm it is actually running,
// not merely that a client was constructed.
func (r *Runner) DockerReachable() bool {
	if !r.dockerAvail || r.docker == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := r.docker.Ping(ctx)
	return err == nil
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

	if tool.RunsInDocker() {
		if !r.DockerReachable() {
			result.Error = fmt.Errorf("docker is not available — is Docker running?")
			result.FinishedAt = time.Now()
			r.events.Emit(events.ToolError, map[string]any{
				"tool":  tool.Name,
				"error": result.Error,
			})
			return result
		}
		return r.runDocker(tool, args, casePath, result)
	}

	if tool.LocalCmd != "" {
		return r.runLocal(tool.LocalCmd, args, result)
	}

	result.Error = fmt.Errorf("no execution method for tool '%s'", tool.Name)
	result.FinishedAt = time.Now()
	return result
}

func (r *Runner) runDocker(tool *Definition, args []string, casePath string, result *Result) *Result {
	ctx := context.Background()

	// Build volume mounts
	var binds []string
	for _, v := range tool.Volumes {
		// Resolve ${CASE_PATH} placeholder
		hostPath := v.Host
		if hostPath == "${CASE_PATH}" || hostPath == "evidence" {
			hostPath = filepath.Join(casePath, "evidence")
		} else if hostPath == "output" {
			hostPath = filepath.Join(casePath, "output")
		}

		// Ensure host directory exists
		os.MkdirAll(hostPath, 0755)

		mode := v.Mode
		if mode == "" {
			mode = "rw"
		}
		binds = append(binds, fmt.Sprintf("%s:%s:%s", hostPath, v.Container, mode))
	}

	// If no volumes defined, auto-mount case directories
	if len(binds) == 0 && casePath != "" {
		evidenceDir := filepath.Join(casePath, "evidence")
		outputDir := filepath.Join(casePath, "output")
		os.MkdirAll(evidenceDir, 0755)
		os.MkdirAll(outputDir, 0755)
		binds = append(binds,
			fmt.Sprintf("%s:/evidence:ro", evidenceDir),
			fmt.Sprintf("%s:/output:rw", outputDir),
		)
	}

	// Container config
	containerConfig := &container.Config{
		Image: tool.DockerImage,
		Cmd:   args,
	}
	if tool.WorkDir != "" {
		containerConfig.WorkingDir = tool.WorkDir
	}
	if tool.Entrypoint != "" {
		containerConfig.Entrypoint = []string{tool.Entrypoint}
	}

	hostConfig := &container.HostConfig{
		Binds: binds,
	}

	// Create container
	resp, err := r.docker.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		result.Error = fmt.Errorf("docker create: %w", err)
		result.FinishedAt = time.Now()
		r.events.Emit(events.ToolError, map[string]any{
			"tool":  tool.Name,
			"error": result.Error,
		})
		return result
	}
	containerID := resp.ID
	defer func() {
		// Clean up container after run
		r.docker.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	}()

	// Start container
	if err := r.docker.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		result.Error = fmt.Errorf("docker start: %w", err)
		result.FinishedAt = time.Now()
		r.events.Emit(events.ToolError, map[string]any{
			"tool":  tool.Name,
			"error": result.Error,
		})
		return result
	}

	// Wait for container to finish
	statusCh, errCh := r.docker.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			result.Error = fmt.Errorf("docker wait: %w", err)
			result.FinishedAt = time.Now()
			r.events.Emit(events.ToolError, map[string]any{
				"tool":  tool.Name,
				"error": result.Error,
			})
			return result
		}
	case status := <-statusCh:
		result.ReturnCode = int(status.StatusCode)
	}

	// Capture stdout/stderr
	logs, err := r.docker.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		result.Error = fmt.Errorf("docker logs: %w", err)
	} else {
		var buf bytes.Buffer
		io.Copy(&buf, logs)
		result.Stdout = buf.String()
	}

	result.FinishedAt = time.Now()

	if result.ReturnCode == 0 {
		r.events.Emit(events.ToolFinished, map[string]any{
			"tool":   tool.Name,
			"output": result.Stdout,
		})
	} else {
		result.Error = fmt.Errorf("container exited with code %d", result.ReturnCode)
		r.events.Emit(events.ToolError, map[string]any{
			"tool":  tool.Name,
			"error": result.Error,
		})
	}

	return result
}

func (r *Runner) runLocal(cmd string, args []string, result *Result) *Result {
	fullArgs := append([]string{cmd}, args...)
	command := exec.Command(fullArgs[0], fullArgs[1:]...)

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		result.Error = err
		result.ReturnCode = 1
	}
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.FinishedAt = time.Now()

	if result.Error == nil {
		r.events.Emit(events.ToolFinished, map[string]any{
			"tool":   result.Tool,
			"output": result.Stdout,
		})
	} else {
		r.events.Emit(events.ToolError, map[string]any{
			"tool":  result.Tool,
			"error": result.Error,
		})
	}
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

// Status describes whether an installed tool is usable right now.
type Status int

const (
	StatusReady     Status = iota // docker image present, or local cmd on PATH
	StatusNotBuilt                // docker tool, image missing
	StatusDockerOff               // docker tool, daemon unreachable
	StatusMissing                 // local tool, cmd not on PATH
)

// String returns a short label for the status.
func (s Status) String() string {
	switch s {
	case StatusReady:
		return "ready"
	case StatusNotBuilt:
		return "not built"
	case StatusDockerOff:
		return "docker off"
	case StatusMissing:
		return "missing"
	default:
		return "unknown"
	}
}

// normalizeTag appends :latest when an image reference has no explicit tag.
func normalizeTag(ref string) string {
	if ref == "" || strings.Contains(ref, ":") {
		return ref
	}
	return ref + ":latest"
}

// resolveStatus is the pure decision: given the set of available image tags,
// daemon reachability, and whether a local cmd was found, return a Status.
func resolveStatus(d *Definition, imageSet map[string]bool, dockerUp, localFound bool) Status {
	if d.RunsInDocker() {
		if !dockerUp {
			return StatusDockerOff
		}
		if imageSet[normalizeTag(d.DockerImage)] {
			return StatusReady
		}
		return StatusNotBuilt
	}
	if localFound {
		return StatusReady
	}
	return StatusMissing
}

// ResolveStatuses returns a status per tool name, plus whether docker is up.
// It performs a single ImageList call (no per-tool round-trips).
func (r *Runner) ResolveStatuses(defs []*Definition) (map[string]Status, bool) {
	statuses := make(map[string]Status, len(defs))
	imageSet := map[string]bool{}
	dockerUp := r.dockerAvail

	if dockerUp {
		images, err := r.docker.ImageList(context.Background(), image.ListOptions{})
		if err != nil {
			dockerUp = false
		} else {
			for _, img := range images {
				for _, tag := range img.RepoTags {
					imageSet[tag] = true
				}
			}
		}
	}

	for _, d := range defs {
		localFound := false
		if !d.RunsInDocker() && d.LocalCmd != "" {
			if _, err := exec.LookPath(d.LocalCmd); err == nil {
				localFound = true
			}
		}
		statuses[d.Name] = resolveStatus(d, imageSet, dockerUp, localFound)
	}
	return statuses, dockerUp
}
