package casearchive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/events"
)

func TestExportImportRoundTrip(t *testing.T) {
	base := t.TempDir()
	mgr := cases.NewManager(base, events.NewBus())
	c, err := mgr.Create("c1")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(c.Path, "evidence", "a.bin"), []byte("abc"), 0644)

	out := filepath.Join(t.TempDir(), "c1.tar.gz")
	if err := Export(c.Path, out, true); err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("archive missing: %v", err)
	}

	dest := t.TempDir()
	name, err := Import(out, dest, "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if name != "c1" {
		t.Fatalf("want name c1, got %q", name)
	}
	if _, err := os.Stat(filepath.Join(dest, "c1", "evidence", "a.bin")); err != nil {
		t.Fatalf("evidence not restored: %v", err)
	}

	// Conflict → auto-suffix
	name2, err := Import(out, dest, "")
	if err != nil {
		t.Fatalf("import 2: %v", err)
	}
	if name2 == "c1" {
		t.Fatalf("expected non-colliding name, got %q", name2)
	}
}
