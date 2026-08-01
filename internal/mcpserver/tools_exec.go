package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/tools"
)

func boolPtr(b bool) *bool { return &b }

// danger annotates tools that execute Docker/host processes or otherwise
// mutate state outside the case directory — clients should confirm before
// invoking them.
var danger = &mcp.ToolAnnotations{
	DestructiveHint: boolPtr(true),
	OpenWorldHint:   boolPtr(true),
}

type toolsListIn struct {
	Category string `json:"category,omitempty" jsonschema:"filter installed tools by category"`
}
type toolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
	Mode        string `json:"mode"` // "docker", "local", or "builtin"
	Status      string `json:"status"`
}
type toolsListOut struct {
	Installed []toolInfo `json:"installed"`
	Available []toolInfo `json:"available"`
	Builtin   []toolInfo `json:"builtin"`
	DockerUp  bool       `json:"docker_up"`
}

type toolRunIn struct {
	Case string   `json:"case" jsonschema:"case name (output is captured here)"`
	Tool string   `json:"tool" jsonschema:"tool name"`
	Args []string `json:"args,omitempty" jsonschema:"tool arguments"`
}
type toolRunOut struct {
	Tool     string `json:"tool"`
	ExitOK   bool   `json:"exit_ok"`
	Output   string `json:"output"`
	Captured string `json:"captured_path,omitempty"`
	Stderr   string `json:"stderr,omitempty"` // snippet included when the run failed
}

type toolBuildIn struct {
	Tool string `json:"tool" jsonschema:"installed tool name to build a Docker image for"`
}
type toolBuildOut struct {
	Tool    string `json:"tool"`
	Built   bool   `json:"built"`
	Log     string `json:"log,omitempty"`
	SmokeOK bool   `json:"smoke_ok,omitempty"`
	Smoke   string `json:"smoke_output,omitempty"`
}

type toolInstallIn struct {
	Tool string `json:"tool" jsonschema:"catalog tool name to install"`
}
type toolInstallOut struct {
	Tool      string `json:"tool"`
	Installed bool   `json:"installed"`
	Built     bool   `json:"built"`
	Log       string `json:"log,omitempty"`
	SmokeOK   bool   `json:"smoke_ok,omitempty"`
	Smoke     string `json:"smoke_output,omitempty"`
}

func (s *Server) registerExecTools() {
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tools_list",
		Description: "List installed, built-in, and installable tools with status.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.toolsList)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tool_run",
		Description: "Run a DFIR tool against evidence; output is captured to the case. Executes Docker/host processes.",
		Annotations: danger}, s.toolRun)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tool_build",
		Description: "Build a Docker-backed tool image.", Annotations: danger}, s.toolBuild)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tool_install",
		Description: "Install a tool template from the embedded catalog.", Annotations: danger}, s.toolInstall)
}

func (s *Server) toolsList(ctx context.Context, req *mcp.CallToolRequest, in toolsListIn) (*mcp.CallToolResult, toolsListOut, error) {
	// The Tools registry is an unsynchronized map; tool_install writes it
	// via DiscoverFromPath/Register while this reads it, so guard with the
	// server mutex to avoid a concurrent map read/write panic under HTTP.
	s.mu.Lock()
	installed := s.app.Tools.List(in.Category)
	statuses, dockerUp := s.app.Runner.ResolveStatuses(installed)
	s.mu.Unlock()

	out := toolsListOut{DockerUp: dockerUp}
	installedNames := map[string]bool{}
	for _, t := range installed {
		installedNames[t.Name] = true
		mode := "local"
		if t.RunsInDocker() {
			mode = "docker"
		}
		out.Installed = append(out.Installed, toolInfo{
			Name: t.Name, Description: t.Description, Category: t.Category,
			Mode: mode, Status: statuses[t.Name].String(),
		})
	}

	entries, err := catalog.List()
	if err != nil {
		return errResult("tools_list: read catalog: %v", err), toolsListOut{}, nil
	}
	for _, e := range entries {
		if installedNames[e.Name] {
			continue
		}
		out.Available = append(out.Available, toolInfo{
			Name: e.Name, Description: e.Description, Category: e.Category,
			Mode: "docker", Status: "not installed",
		})
	}

	for _, m := range builtins.List() {
		out.Builtin = append(out.Builtin, toolInfo{
			Name: m.Name, Description: m.Description, Mode: "builtin", Status: "ready",
		})
	}

	summary := fmt.Sprintf("%d installed, %d available, %d built-in", len(out.Installed), len(out.Available), len(out.Builtin))
	return textResult(summary), out, nil
}

func (s *Server) toolRun(ctx context.Context, req *mcp.CallToolRequest, in toolRunIn) (*mcp.CallToolResult, toolRunOut, error) {
	if in.Tool == "" {
		return errResult("tool_run: tool is required"), toolRunOut{}, nil
	}

	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		var result *tools.Result
		if builtins.Has(in.Tool) {
			result = s.app.Runner.RunBuiltin(in.Tool, in.Args)
		} else {
			def, ok := s.app.Tools.Get(in.Tool)
			if !ok {
				return nil, fmt.Errorf("tool not found: %s", in.Tool)
			}
			// Local (non-Docker) tools that are missing their binary would
			// otherwise trigger Runner.Run's auto-install fallback, which
			// shells out to sudo apt-get/brew with cmd.Stdout=os.Stdout —
			// fatal on the stdio MCP transport (corrupts JSON-RPC framing)
			// and can hang waiting on an interactive sudo prompt. Refuse
			// up front instead of calling Run in that case.
			if !def.RunsInDocker() {
				statuses, _ := s.app.Runner.ResolveStatuses([]*tools.Definition{def})
				if statuses[def.Name] == tools.StatusMissing {
					return nil, fmt.Errorf("tool %q is not installed; install it via the Mimir CLI first", in.Tool)
				}
			}
			result = s.app.Runner.Run(def, in.Args, c.Path)
		}

		out := toolRunOut{Tool: in.Tool, ExitOK: result.Success(), Output: result.Stdout}
		if result.Stderr != "" {
			out.Output += "\n" + result.Stderr
		}
		if !result.Success() {
			stderrMsg := result.Stderr
			if stderrMsg == "" && result.Error != nil {
				stderrMsg = result.Error.Error()
			}
			out.Stderr = stderrSnippet(stderrMsg)
		}

		outputRel := ""
		if path, rerr := s.app.Output.Record(in.Tool, result.Stdout, c.Path); rerr == nil {
			if rel, relErr := filepath.Rel(c.Path, path); relErr == nil {
				outputRel = rel
			} else {
				outputRel = path
			}
			out.Captured = outputRel
		}

		// Mirror recordRun (internal/shell/shell.go) so MCP-run tools appear
		// identically in timeline_read, timeline_correlate, report, and
		// export --provenance.
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
		// Match recordRun's error handling: append/save failures are
		// surfaced as warnings on stderr, never as a Go error that would
		// mask the tool result the caller already has (the tool itself
		// ran; only bookkeeping failed).
		if err := c.AppendEvent(cases.TimelineEvent{
			Type:      "tool_run",
			Timestamp: time.Now().Format(time.RFC3339),
			Payload:   payload,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: append timeline: %v\n", err)
		}
		c.AddToolUsage(in.Tool)
		if err := c.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: save case: %v\n", err)
		}

		return out, nil
	})
	if err != nil {
		return errResult("tool_run: %v", err), toolRunOut{}, nil
	}
	out := v.(toolRunOut)
	summary := fmt.Sprintf("%s: exit_ok=%v", out.Tool, out.ExitOK)
	if out.Stderr != "" {
		summary += fmt.Sprintf(", stderr: %s", out.Stderr)
	}
	return textResult(summary), out, nil
}

// stderrSnippetLen bounds how much of a failing tool's stderr is echoed back to
// the MCP client so a noisy traceback can't flood the tool result.
const stderrSnippetLen = 2000

// stderrSnippet keeps the tail of a failing tool's stderr, marking any
// truncation so the client knows the output was cut.
func stderrSnippet(s string) string {
	if len(s) <= stderrSnippetLen {
		return s
	}
	return "...[truncated]...\n" + s[len(s)-stderrSnippetLen:]
}

func (s *Server) toolBuild(ctx context.Context, req *mcp.CallToolRequest, in toolBuildIn) (*mcp.CallToolResult, toolBuildOut, error) {
	if in.Tool == "" {
		return errResult("tool_build: tool is required"), toolBuildOut{}, nil
	}
	s.mu.Lock()
	def, ok := s.app.Tools.Get(in.Tool)
	s.mu.Unlock()
	if !ok {
		return errResult("tool_build: %s is not installed — install it first", in.Tool), toolBuildOut{}, nil
	}
	if !def.RunsInDocker() {
		return errResult("tool_build: %s is a local tool — nothing to build", in.Tool), toolBuildOut{}, nil
	}
	if !s.app.Runner.DockerReachable() {
		return errResult("tool_build: docker is not available"), toolBuildOut{}, nil
	}
	log, smoke, smokeOK, err := buildAndSmoke(def)
	if err != nil {
		return errResult("tool_build: %v", err), toolBuildOut{}, nil
	}
	out := toolBuildOut{Tool: in.Tool, Built: true, Log: log, Smoke: smoke, SmokeOK: smokeOK}
	if !smokeOK {
		return errResult("tool_build: %s: image built but smoke test failed (docker run %s %s)", in.Tool, def.DockerImage, def.Version), out, nil
	}
	return textResult(fmt.Sprintf("%s: image built", in.Tool)), out, nil
}

func (s *Server) toolInstall(ctx context.Context, req *mcp.CallToolRequest, in toolInstallIn) (*mcp.CallToolResult, toolInstallOut, error) {
	if in.Tool == "" {
		return errResult("tool_install: tool is required"), toolInstallOut{}, nil
	}
	if _, ok := catalog.Get(in.Tool); !ok {
		return errResult("tool_install: unknown tool: %s", in.Tool), toolInstallOut{}, nil
	}

	destDir := filepath.Join(s.app.Config.ToolsPath, in.Tool)
	if _, err := os.Stat(destDir); err == nil {
		return errResult("tool_install: %s already installed — use tool_build to rebuild", in.Tool), toolInstallOut{}, nil
	}

	if err := catalog.Install(in.Tool, destDir); err != nil {
		return errResult("tool_install: %v", err), toolInstallOut{}, nil
	}
	s.mu.Lock()
	discoverErr := s.app.Tools.DiscoverFromPath(s.app.Config.ToolsPath)
	def, ok := s.app.Tools.Get(in.Tool)
	s.mu.Unlock()
	if discoverErr != nil {
		return errResult("tool_install: register tool: %v", discoverErr), toolInstallOut{}, nil
	}
	if !ok {
		return errResult("tool_install: %s not found after install", in.Tool), toolInstallOut{}, nil
	}

	out := toolInstallOut{Tool: in.Tool, Installed: true}
	if def.RunsInDocker() {
		if !s.app.Runner.DockerReachable() {
			return textResult(fmt.Sprintf("%s: template installed, but Docker is not available — use tool_build once Docker is running", in.Tool)), out, nil
		}
		log, smoke, smokeOK, err := buildAndSmoke(def)
		if err != nil {
			return errResult("tool_install: build image: %v", err), toolInstallOut{}, nil
		}
		out.Built = true
		out.Log = log
		out.Smoke = smoke
		out.SmokeOK = smokeOK
	}
	return textResult(fmt.Sprintf("%s: installed and ready", in.Tool)), out, nil
}

// buildAndSmoke builds a tool's Docker image and, when the template declares a
// version probe (docker.version), runs it in a throwaway container to confirm
// the image is actually usable. A broken download or a binary/glibc mismatch
// must fail the build rather than silently leave a dead image behind.
func buildAndSmoke(def *tools.Definition) (log, smoke string, smokeOK bool, err error) {
	log, err = buildImage(filepath.Dir(def.TemplatePath), def.DockerImage)
	if err != nil {
		return log, "", false, err
	}
	if def.Version == "" {
		return log, "", true, nil
	}
	smoke, err = runSmokeTest(def.DockerImage, def.Version)
	if err != nil {
		return log, smoke, false, err
	}
	return log, smoke, true, nil
}

// runSmokeTest starts a throwaway container from image with versionArg (e.g.
// "--version"), expecting it to print version info and exit 0. Combined output
// is returned so a failure can be diagnosed from the MCP tool result.
func runSmokeTest(image, versionArg string) (string, error) {
	cmd := exec.Command("docker", "run", "--rm", image, versionArg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("smoke test `docker run --rm %s %s`: %w", image, versionArg, err)
	}
	return string(out), nil
}

// buildImage builds a Docker image from a template directory by shelling out
// to the docker CLI, capturing combined output for the caller instead of
// streaming to os.Stdout/Stderr (MCP tool handlers must never write to
// stdout, which carries the JSON-RPC stream).
func buildImage(templateDir, imageTag string) (string, error) {
	cmd := exec.Command("docker", "build", "--no-cache", "-t", imageTag, templateDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return string(output), fmt.Errorf("docker build failed (exit %d): %s", exitErr.ExitCode(), output)
		}
		return string(output), fmt.Errorf("running docker build: %w", err)
	}
	return string(output), nil
}
