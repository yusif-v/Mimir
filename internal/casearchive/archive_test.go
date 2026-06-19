package casearchive

import (
	"archive/tar"
	"compress/gzip"
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

func TestImportRejectsZipSlip(t *testing.T) {
	// Build a tar.gz whose entry escapes the target dir via ../
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "evil.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	// top-level dir entry so the importer derives a name, then an escaping file
	body := []byte("pwned")
	hdr := &tar.Header{Name: "evilcase/../../escape.txt", Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	tw.Write(body)
	tw.Close()
	gz.Close()
	f.Close()

	dest := t.TempDir()
	if _, err := Import(archivePath, dest, ""); err == nil {
		t.Fatal("expected Import to reject zip-slip archive, got nil error")
	}
	// And confirm nothing was written outside dest
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); err == nil {
		t.Fatal("zip-slip wrote a file outside the target dir")
	}
}
