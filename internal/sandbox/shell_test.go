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
