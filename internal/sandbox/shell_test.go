package sandbox

import (
	"bytes"
	"strings"
	"testing"
)

func TestShellCreateCmd(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	var buf bytes.Buffer
	err := sh.Create(&buf, []string{"--os", "ubuntu-22.04", "--file", "/evidence/test.exe"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(buf.String(), "created") {
		t.Errorf("expected 'created' in output: %s", buf.String())
	}
}

func TestShellListCmd(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	// Create a sandbox first
	mgr.Create("current-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "", 300)

	var buf bytes.Buffer
	err := sh.List(&buf, []string{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(buf.String(), "ubuntu-22.04") {
		t.Errorf("expected OS in output: %s", buf.String())
	}
}

func TestIndexOf(t *testing.T) {
	got := indexOf([]string{"run", "sb", "--", "vol", "pslist"}, "--")
	if got != 2 {
		t.Errorf("indexOf: want 2, got %d", got)
	}
	if indexOf([]string{"run", "sb"}, "--") != -1 {
		t.Error("indexOf: expected -1 when '--' absent")
	}
}

func TestRunInlineCommand(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	s := mgr.Create("current-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "", 300)
	// Simulate a started container (dryRun assigns a container ID).
	if err := runtime.CreateContainer(s); err != nil {
		t.Fatalf("CreateContainer dry-run: %v", err)
	}
	mgr.Update(s)

	var buf bytes.Buffer
	// Inline command path: sandbox run <id> -- vol windows.pslist
	err := sh.Run(&buf, []string{s.ID, "--", "vol", "windows.pslist"})
	if err != nil {
		t.Fatalf("Run inline: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "vol windows.pslist") {
		t.Errorf("expected inline command echoed, got: %s", out)
	}
	if !strings.Contains(out, "dry-run output") {
		t.Errorf("expected dry-run exec output, got: %s", out)
	}
}
