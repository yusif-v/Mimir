package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestCmdIOCFromOutputAcceptsToolName(t *testing.T) {
	app, c := newPathTestApp(t)
	outputDir := filepath.Join(c.Path, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldName := "hash_20250101_000000.txt"
	newName := "hash_20250102_000000.txt"
	oldPath := filepath.Join(outputDir, oldName)
	newPath := filepath.Join(outputDir, newName)
	if err := os.WriteFile(oldPath, []byte("old indicator 1.1.1.1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new indicator 9.9.9.9"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if err := app.cmdIOC([]string{"from-output", "hash"}); err != nil {
		t.Fatalf("ioc from-output hash: %v", err)
	}
	iocs := c.IOCs()
	if !hasIOC(iocs, "9.9.9.9") {
		t.Fatalf("expected newest hash output to be used, got %+v", iocs)
	}
	if hasIOC(iocs, "1.1.1.1") {
		t.Fatalf("unexpected indicator from older output: %+v", iocs)
	}

	// An exact timestamped filename still selects that specific output file.
	if err := app.cmdIOC([]string{"from-output", oldName}); err != nil {
		t.Fatalf("ioc from-output %s: %v", oldName, err)
	}
	if !hasIOC(c.IOCs(), "1.1.1.1") {
		t.Fatalf("exact output filename was not used: %+v", c.IOCs())
	}
}

func hasIOC(records []cases.IOC, value string) bool {
	for _, r := range records {
		if r.Value == value {
			return true
		}
	}
	return false
}

func TestCmdEvidenceAddExpandsTilde(t *testing.T) {
	app, c := newPathTestApp(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	tildeFile := filepath.Join(home, "tilde.bin")
	if err := os.WriteFile(tildeFile, []byte("tilde-content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdEvidence([]string{"add", "~/tilde.bin"}); err != nil {
		t.Fatalf("evidence add ~/tilde.bin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Path, "evidence", "tilde.bin")); err != nil {
		t.Fatalf("tilde evidence not copied: %v", err)
	}

	absFile := filepath.Join(t.TempDir(), "abs.bin")
	if err := os.WriteFile(absFile, []byte("abs-content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdEvidence([]string{"add", absFile}); err != nil {
		t.Fatalf("evidence add %s: %v", absFile, err)
	}

	names := map[string]bool{}
	for _, rec := range c.Evidence() {
		names[rec.Name] = true
	}
	if !names["tilde.bin"] || !names["abs.bin"] {
		t.Fatalf("expected tilde and absolute evidence entries, got %+v", c.Evidence())
	}
}
