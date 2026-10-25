package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/events"
)

func newPathTestApp(t *testing.T) (*App, *cases.Case) {
	t.Helper()
	app := newTestApp(t)
	if _, err := app.Cases.Create("path-resolution"); err != nil {
		t.Fatalf("create case: %v", err)
	}
	c, err := app.Cases.Open("path-resolution")
	if err != nil {
		t.Fatalf("open case: %v", err)
	}
	if err := os.WriteFile(filepath.Join(c.Path, "evidence", "sample.txt"), []byte("abc"), 0644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}
	return app, c
}

func TestResolveRunArgsCaseRelative(t *testing.T) {
	app, c := newPathTestApp(t)
	abs := filepath.Join(c.Path, "evidence", "sample.txt")

	// Run from outside the case so a cwd-relative fallback cannot mask a bug.
	t.Chdir(t.TempDir())

	// Flags and their values are left untouched; both spellings of the evidence
	// path resolve to the same absolute file.
	got, err := app.resolveRunArgs([]string{"-n", "4", "evidence/sample.txt", "sample.txt", abs})
	if err != nil {
		t.Fatalf("resolveRunArgs: %v", err)
	}
	want := []string{"-n", "4", abs, abs, abs}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCmdRunResolvesCaseRelativePaths(t *testing.T) {
	app, c := newPathTestApp(t)
	abs := filepath.Join(c.Path, "evidence", "sample.txt")
	t.Chdir(t.TempDir())

	for _, arg := range []string{"evidence/sample.txt", "sample.txt", abs} {
		if err := app.cmdRun([]string{"hash", arg}); err != nil {
			t.Errorf("run hash %s: %v", arg, err)
		}
	}
}

func TestCmdRunRelativePathWithoutCase(t *testing.T) {
	app := &App{Cases: cases.NewManager(t.TempDir(), events.NewBus(), nil, nil)}
	t.Chdir(t.TempDir())

	err := app.cmdRun([]string{"hash", "evidence/sample.txt"})
	if err == nil {
		t.Fatal("expected an error for a relative path with no case open")
	}
	msg := err.Error()
	if !strings.Contains(msg, "evidence/sample.txt") {
		t.Errorf("error does not name the missing path: %q", msg)
	}
	if !strings.Contains(msg, "no case is open") {
		t.Errorf("error does not explain that no case is open: %q", msg)
	}
	if strings.HasPrefix(msg, "stat ") {
		t.Errorf("error is a bare stat error: %q", msg)
	}
}

func TestResolveRunArgKeepsMissingAbsolutePath(t *testing.T) {
	app := &App{Cases: cases.NewManager(t.TempDir(), events.NewBus(), nil, nil)}
	abs := filepath.Join(t.TempDir(), "missing.bin")

	got, err := app.resolveRunArg(abs)
	if err != nil {
		t.Fatalf("absolute path should be passed through unchanged: %v", err)
	}
	if got != abs {
		t.Errorf("got %q, want %q", got, abs)
	}
}
