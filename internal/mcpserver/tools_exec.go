package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
}

type toolBuildIn struct {
	Tool string `json:"tool" jsonschema:"installed tool name to build a Docker image for"`
}
type toolBuildOut struct {
	Tool  string `json:"tool"`
	Built bool   `json:"built"`
	Log   string `json:"log,omitempty"`
}

type toolInstallIn struct {
	Tool string `json:"tool" jsonschema:"catalog tool name to install"`
}
type toolInstallOut struct {
	Tool      string `json:"tool"`
	Installed bool   `json:"installed"`
	Built     bool   `json:"built"`
	Log       string `json:"log,omitempty"`
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
	installed := s.app.Tools.List(in.Category)
	statuses, dockerUp := s.app.Runner.ResolveStatuses(installed)

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
			result = s.app.Runner.Run(def, in.Args, c.Path)
		}

		out := toolRunOut{Tool: in.Tool, ExitOK: result.Success(), Output: result.Stdout}
		if result.Stderr != "" {
			out.Output += "\n" + result.Stderr
		}
		if path, rerr := s.app.Output.Record(in.Tool, result.Stdout, c.Path); rerr == nil {
			if rel, relErr := filepath.Rel(c.Path, path); relErr == nil {
				out.Captured = rel
			} else {
				out.Captured = path
			}
		}
		return out, nil
	})
	if err != nil {
		return errResult("tool_run: %v", err), toolRunOut{}, nil
	}
	out := v.(toolRunOut)
	summary := fmt.Sprintf("%s: exit_ok=%v", out.Tool, out.ExitOK)
	return textResult(summary), out, nil
}

func (s *Server) toolBuild(ctx context.Context, req *mcp.CallToolRequest, in toolBuildIn) (*mcp.CallToolResult, toolBuildOut, error) {
	if in.Tool == "" {
		return errResult("tool_build: tool is required"), toolBuildOut{}, nil
	}
	def, ok := s.app.Tools.Get(in.Tool)
	if !ok {
		return errResult("tool_build: %s is not installed — install it first", in.Tool), toolBuildOut{}, nil
	}
	if !def.RunsInDocker() {
		return errResult("tool_build: %s is a local tool — nothing to build", in.Tool), toolBuildOut{}, nil
	}
	if !s.app.Runner.DockerReachable() {
		return errResult("tool_build: docker is not available"), toolBuildOut{}, nil
	}
	log, err := buildImage(filepath.Dir(def.TemplatePath), def.DockerImage)
	if err != nil {
		return errResult("tool_build: %v", err), toolBuildOut{}, nil
	}
	out := toolBuildOut{Tool: in.Tool, Built: true, Log: log}
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
	if err := s.app.Tools.DiscoverFromPath(s.app.Config.ToolsPath); err != nil {
		return errResult("tool_install: register tool: %v", err), toolInstallOut{}, nil
	}
	def, ok := s.app.Tools.Get(in.Tool)
	if !ok {
		return errResult("tool_install: %s not found after install", in.Tool), toolInstallOut{}, nil
	}

	out := toolInstallOut{Tool: in.Tool, Installed: true}
	if def.RunsInDocker() {
		if !s.app.Runner.DockerReachable() {
			return textResult(fmt.Sprintf("%s: template installed, but Docker is not available — use tool_build once Docker is running", in.Tool)), out, nil
		}
		log, err := buildImage(filepath.Dir(def.TemplatePath), def.DockerImage)
		if err != nil {
			return errResult("tool_install: build image: %v", err), toolInstallOut{}, nil
		}
		out.Built = true
		out.Log = log
	}
	return textResult(fmt.Sprintf("%s: installed and ready", in.Tool)), out, nil
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
