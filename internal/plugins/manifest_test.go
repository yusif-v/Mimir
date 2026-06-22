package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	tomlContent := `
name = "test-plugin"
description = "A test plugin"
version = "1.0.0"
entrypoint = "run"
`
	os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(tomlContent), 0644)
	p, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if p.Name != "test-plugin" {
		t.Errorf("name: got %q, want %q", p.Name, "test-plugin")
	}
	if p.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", p.Version, "1.0.0")
	}
}

func TestLoadManifestMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}
