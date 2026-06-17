package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yusif-v/mimir/internal/catalog"
)

func TestListReturnsSeededEntries(t *testing.T) {
	entries, err := catalog.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	want := map[string]bool{"volatility": false, "yara": false, "bulk_extractor": false}
	for _, e := range entries {
		if _, ok := want[e.Name]; ok {
			want[e.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("catalog missing expected entry %q", name)
		}
	}
}

func TestGetHitAndMiss(t *testing.T) {
	e, ok := catalog.Get("volatility")
	if !ok {
		t.Fatal("expected to find volatility")
	}
	if e.Category != "forensics" {
		t.Errorf("expected category forensics, got %q", e.Category)
	}
	if _, ok := catalog.Get("does-not-exist"); ok {
		t.Error("expected miss for unknown tool")
	}
}

func TestInstallCopiesFiles(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "volatility")
	if err := catalog.Install("volatility", dest); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	for _, f := range []string{"mimir.toml", "Dockerfile"} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Errorf("expected %s to be installed: %v", f, err)
		}
	}
}

func TestInstallUnknownErrors(t *testing.T) {
	if err := catalog.Install("nope", filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error installing unknown tool")
	}
}
