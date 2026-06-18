package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		ToolsPath: filepath.Join(dir, "tools"),
		CasesPath: filepath.Join(dir, "cases"),
	}
	return NewApp(cfg)
}

func TestCmdInstallUnknownTool(t *testing.T) {
	app := newTestApp(t)
	err := app.cmdInstall([]string{"definitely-not-a-tool"})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}
}

func TestCmdInstallRequiresArg(t *testing.T) {
	app := newTestApp(t)
	if err := app.cmdInstall(nil); err == nil {
		t.Fatal("expected usage error with no args")
	}
}

func TestCmdBuildNotInstalled(t *testing.T) {
	app := newTestApp(t)
	err := app.cmdBuild([]string{"volatility"})
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected not-installed error, got %v", err)
	}
}

func TestCmdRunRecordsTimeline(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{
		Config: &config.Config{CasesPath: base},
		Events: bus,
		Cases:  cases.NewManager(base, bus),
		Tools:  tools.NewRegistry(bus),
		Runner: tools.NewRunner(bus),
		Output: tools.NewOutputCapture(bus),
	}

	if _, err := app.Cases.Create("c1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, err := app.Cases.Open("c1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Evidence file to hash.
	ev := filepath.Join(c.Path, "evidence", "f.txt")
	if err := os.WriteFile(ev, []byte("abc"), 0644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	if err := app.cmdRun([]string{"hash", ev}); err != nil {
		t.Fatalf("cmdRun: %v", err)
	}

	// An output file was written.
	outs, _ := os.ReadDir(filepath.Join(c.Path, "output"))
	if len(outs) == 0 {
		t.Fatal("expected an output file")
	}

	// A tool_run event is on the timeline.
	var found bool
	for _, e := range c.Timeline() {
		if e.Type == "tool_run" && e.Payload["tool"] == "hash" {
			found = true
			if e.Payload["success"] != true {
				t.Errorf("expected success=true, got %v", e.Payload["success"])
			}
		}
	}
	if !found {
		t.Fatalf("no tool_run event found; timeline=%+v", c.Timeline())
	}
}

// TestCmdBuildDockerDown verifies that building an installed docker tool while
// the Docker daemon is unreachable yields the friendly docker-down error rather
// than a raw `docker build` failure. Skips when Docker is actually available.
func TestCmdBuildDockerDown(t *testing.T) {
	app := newTestApp(t)
	if app.Runner.DockerReachable() {
		t.Skip("docker daemon is reachable; this test exercises the docker-down path")
	}
	// Install the volatility template so it is registered as a docker tool.
	dest := filepath.Join(app.Config.ToolsPath, "volatility")
	if err := catalog.Install("volatility", dest); err != nil {
		t.Fatalf("seed install failed: %v", err)
	}
	if err := app.Tools.DiscoverFromPath(app.Config.ToolsPath); err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	err := app.cmdBuild([]string{"volatility"})
	if err == nil || !strings.Contains(err.Error(), "docker is not available") {
		t.Fatalf("expected docker-not-available error, got %v", err)
	}
}
