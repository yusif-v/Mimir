package config

import (
	"os"
	"path/filepath"
	"testing"
)

// chdirTemp moves into an isolated cwd so Load() can't pick up a real
// mimir.yaml from the repo, and restores the original cwd on cleanup.
func chdirTemp(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func TestDefaultConfigDerivesPathsFromHome(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.HomeDir == "" {
		t.Fatal("HomeDir should not be empty")
	}
	if cfg.CasesPath != filepath.Join(cfg.HomeDir, "investigations") {
		t.Errorf("CasesPath = %q, want %q", cfg.CasesPath, filepath.Join(cfg.HomeDir, "investigations"))
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, want INFO", cfg.LogLevel)
	}
}

func TestLoadEnvHomeOverrideRederivesPaths(t *testing.T) {
	chdirTemp(t)
	home := t.TempDir()
	t.Setenv("MIMIR_HOME", home)

	cfg := Load()

	if cfg.HomeDir != home {
		t.Errorf("HomeDir = %q, want %q", cfg.HomeDir, home)
	}
	if cfg.CasesPath != filepath.Join(home, "investigations") {
		t.Errorf("CasesPath = %q, want derived from MIMIR_HOME", cfg.CasesPath)
	}
	if cfg.ToolsPath != filepath.Join(home, "tools") {
		t.Errorf("ToolsPath = %q, want derived from MIMIR_HOME", cfg.ToolsPath)
	}
	// Load() must create the runtime directories.
	if _, err := os.Stat(cfg.CasesPath); err != nil {
		t.Errorf("expected CasesPath created: %v", err)
	}
}

func TestLoadEnvCasesPathOverride(t *testing.T) {
	chdirTemp(t)
	home := t.TempDir()
	custom := filepath.Join(t.TempDir(), "custom-cases")
	t.Setenv("MIMIR_HOME", home)
	t.Setenv("MIMIR_CASES_PATH", custom)

	cfg := Load()

	if cfg.CasesPath != custom {
		t.Errorf("CasesPath = %q, want %q (MIMIR_CASES_PATH should win over MIMIR_HOME)", cfg.CasesPath, custom)
	}
}

func TestLoadEnvLogLevelOverride(t *testing.T) {
	chdirTemp(t)
	t.Setenv("MIMIR_HOME", t.TempDir())
	t.Setenv("MIMIR_LOG_LEVEL", "DEBUG")

	cfg := Load()

	if cfg.LogLevel != "DEBUG" {
		t.Errorf("LogLevel = %q, want DEBUG", cfg.LogLevel)
	}
}

func TestMergeAppliesOnlyNonEmptyFields(t *testing.T) {
	base := &Config{
		HomeDir:  "/base/home",
		LogLevel: "INFO",
		LogPath:  "/base/logs",
	}
	base.merge(&Config{
		HomeDir:  "/override/home",
		LogLevel: "", // empty must NOT override
	})

	if base.HomeDir != "/override/home" {
		t.Errorf("HomeDir = %q, want /override/home", base.HomeDir)
	}
	if base.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, empty override must be ignored", base.LogLevel)
	}
	if base.LogPath != "/base/logs" {
		t.Errorf("LogPath = %q, untouched field must be preserved", base.LogPath)
	}
}

func TestWriteDefaultRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := WriteDefault(path); err != nil {
		t.Fatalf("WriteDefault: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("written config is empty")
	}
	// Header comment must be present.
	if got := string(data[:8]); got != "# Mimir " {
		t.Errorf("expected header comment, got %q", got)
	}
}
