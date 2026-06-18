package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
)

func TestParseTemplate(t *testing.T) {
	data := `
[tool]
name = "volatility"
description = "Memory forensics"
category = "forensics"
tags = ["memory", "windows"]

[docker]
image = "dfir-volatility:latest"
workdir = "/output"

[[docker.volumes]]
host = "evidence"
container = "/evidence"
mode = "ro"

[[docker.volumes]]
host = "output"
container = "/output"
mode = "rw"
`
	def, err := tools.ParseTemplate([]byte(data), "/tmp/test/mimir.toml")
	if err != nil {
		t.Fatalf("ParseTemplate failed: %v", err)
	}

	if def.Name != "volatility" {
		t.Errorf("expected name 'volatility', got '%s'", def.Name)
	}
	if def.DockerImage != "dfir-volatility:latest" {
		t.Errorf("expected image 'dfir-volatility:latest', got '%s'", def.DockerImage)
	}
	if def.Category != "forensics" {
		t.Errorf("expected category 'forensics', got '%s'", def.Category)
	}
	if len(def.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(def.Tags))
	}
	if len(def.Volumes) != 2 {
		t.Errorf("expected 2 volumes, got %d", len(def.Volumes))
	}
	if def.Volumes[0].Host != "evidence" || def.Volumes[0].Container != "/evidence" || def.Volumes[0].Mode != "ro" {
		t.Errorf("unexpected volume[0]: %+v", def.Volumes[0])
	}
	if def.WorkDir != "/output" {
		t.Errorf("expected workdir '/output', got '%s'", def.WorkDir)
	}
}

func TestParseTemplateMinimal(t *testing.T) {
	data := `
[tool]
name = "my-tool"
description = "A minimal tool"

[docker]
image = "my-image:latest"
`
	def, err := tools.ParseTemplate([]byte(data), "/tmp/test/mimir.toml")
	if err != nil {
		t.Fatalf("ParseTemplate failed: %v", err)
	}

	if def.Name != "my-tool" {
		t.Errorf("expected name 'my-tool', got '%s'", def.Name)
	}
	if def.Category != "" {
		t.Errorf("expected empty category, got '%s'", def.Category)
	}
	if len(def.Volumes) != 0 {
		t.Errorf("expected 0 volumes, got %d", len(def.Volumes))
	}
}

func TestDiscoverFromPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a tool directory with a valid mimir.toml
	toolDir := filepath.Join(tmpDir, "volatility")
	os.MkdirAll(toolDir, 0755)
	os.WriteFile(filepath.Join(toolDir, "mimir.toml"), []byte(`
[tool]
name = "volatility"
description = "Memory forensics"
category = "forensics"

[docker]
image = "dfir-volatility:latest"
`), 0644)

	// Create another tool
	toolDir2 := filepath.Join(tmpDir, "strings")
	os.MkdirAll(toolDir2, 0755)
	os.WriteFile(filepath.Join(toolDir2, "mimir.toml"), []byte(`
[tool]
name = "strings"
description = "Extract strings from files"
category = "forensics"

[docker]
image = "alpine:latest"
`), 0644)

	bus := events.NewBus()
	reg := tools.NewRegistry(bus)

	if err := reg.DiscoverFromPath(tmpDir); err != nil {
		t.Fatalf("DiscoverFromPath failed: %v", err)
	}

	if _, ok := reg.Get("volatility"); !ok {
		t.Error("expected to find 'volatility' tool")
	}
	if _, ok := reg.Get("strings"); !ok {
		t.Error("expected to find 'strings' tool")
	}
}

func TestDiscoverFromPathMissing(t *testing.T) {
	bus := events.NewBus()
	reg := tools.NewRegistry(bus)

	// Should not error on missing directory
	if err := reg.DiscoverFromPath("/nonexistent/path"); err != nil {
		t.Errorf("expected no error for missing path, got: %v", err)
	}
}

func TestRunnerDockerAvailable(t *testing.T) {
	bus := events.NewBus()
	runner := tools.NewRunner(bus)

	// Just check it doesn't panic — Docker may or may not be running
	_ = runner.DockerAvailable()
}

func TestOutputCaptureRecord(t *testing.T) {
	bus := events.NewBus()
	caseDir := t.TempDir()
	oc := tools.NewOutputCapture(bus)

	path, err := oc.Record("test-tool", "test output", caseDir)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty output path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != "test output" {
		t.Errorf("output = %q, want %q", string(data), "test output")
	}
}

func TestRunBuiltin(t *testing.T) {
	bus := events.NewBus()
	r := tools.NewRunner(bus)

	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(fp, []byte("abc"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res := r.RunBuiltin("hash", []string{fp})
	if !res.Success() {
		t.Fatalf("expected success, err=%v code=%d", res.Error, res.ReturnCode)
	}
	if res.Tool != "hash" {
		t.Errorf("Tool = %q, want hash", res.Tool)
	}
	if res.Stdout == "" {
		t.Error("expected hash output")
	}
}

func TestRunBuiltinFailure(t *testing.T) {
	bus := events.NewBus()
	r := tools.NewRunner(bus)
	res := r.RunBuiltin("hash", []string{"/no/such/file"})
	if res.Success() {
		t.Fatal("expected failure for missing file")
	}
	if res.ReturnCode == 0 {
		t.Error("expected non-zero return code")
	}
}
