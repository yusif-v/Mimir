package shell

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/config"
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
