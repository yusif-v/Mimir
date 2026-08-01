package builtins

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	azip "github.com/alexmullins/zip"
)

// makeZip builds a zip archive in a temp dir. enc maps entry names to the
// AES-256 password used for that entry; entries absent from enc are plain.
func makeZip(t *testing.T, entries map[string][]byte, enc map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := azip.NewWriter(f)
	for name, data := range entries {
		var w io.Writer
		if pw, ok := enc[name]; ok {
			w, err = zw.Encrypt(name, pw)
		} else {
			w, err = zw.Create(name)
		}
		if err != nil {
			t.Fatalf("zip entry %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("zip file close: %v", err)
	}
	return p
}

// makeZipSlip builds a zip whose entry tries to escape the destination dir.
func makeZipSlip(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "slip.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := azip.NewWriter(f)
	fh := &azip.FileHeader{Name: "../evil.txt", Method: azip.Deflate}
	w, err := zw.CreateHeader(fh)
	if err != nil {
		t.Fatalf("create slip entry: %v", err)
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		t.Fatalf("write slip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	f.Close()
	return p
}

// ---------------------------------------------------------------------------
// ZIP list
// ---------------------------------------------------------------------------

func TestUnzipListPlainZip(t *testing.T) {
	p := makeZip(t, map[string][]byte{
		"doc/report.txt": []byte("hello world"),
		"meta.json":      []byte(`{"a":1}`),
	}, nil)
	out, err := Run("unzip", []string{p})
	if err != nil {
		t.Fatalf("unzip: %v", err)
	}
	for _, want := range []string{"ZIP Archive", "Entries: 2", "doc/report.txt", "meta.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestUnzipListAESZip(t *testing.T) {
	p := makeZip(t, map[string][]byte{
		"secret.txt": []byte("classified"),
		"notes.txt":  []byte("public"),
	}, map[string]string{"secret.txt": "hunter2"})
	out, err := Run("unzip", []string{p})
	if err != nil {
		t.Fatalf("unzip: %v", err)
	}
	if !strings.Contains(out, "Encrypted: 1") {
		t.Errorf("expected one encrypted entry:\n%s", out)
	}
	if !strings.Contains(out, "[encrypted]") {
		t.Errorf("expected [encrypted] marker:\n%s", out)
	}
	if !strings.Contains(out, "notes.txt") {
		t.Errorf("expected plaintext entry listed too:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// ZIP extract
// ---------------------------------------------------------------------------

func TestUnzipExtractPlainZip(t *testing.T) {
	p := makeZip(t, map[string][]byte{
		"doc/report.txt": []byte("hello world"),
	}, nil)
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", p, dest})
	if err != nil {
		t.Fatalf("unzip -x: %v", err)
	}
	if !strings.Contains(out, "Extracted 1 files") {
		t.Errorf("expected extraction summary:\n%s", out)
	}
	got, err := os.ReadFile(filepath.Join(dest, "doc", "report.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("extracted content = %q, want %q", got, "hello world")
	}
}

func TestUnzipExtractAESZipCorrectPassword(t *testing.T) {
	p := makeZip(t, map[string][]byte{
		"secret.txt": []byte("classified"),
	}, map[string]string{"secret.txt": "hunter2"})
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", "-p", "hunter2", p, dest})
	if err != nil {
		t.Fatalf("unzip -x -p: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "secret.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v\nout:\n%s", err, out)
	}
	if string(got) != "classified" {
		t.Errorf("extracted content = %q, want %q", got, "classified")
	}
}

func TestUnzipExtractAESZipWrongPassword(t *testing.T) {
	p := makeZip(t, map[string][]byte{
		"secret.txt": []byte("classified"),
	}, map[string]string{"secret.txt": "hunter2"})
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", "-p", "wrong", p, dest})
	if err != nil {
		t.Fatalf("unzip -x -p: %v", err)
	}
	if !strings.Contains(out, "incorrect password") {
		t.Errorf("expected incorrect-password warning:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "secret.txt")); !os.IsNotExist(statErr) {
		t.Errorf("wrong password should not write the file (stat err = %v)", statErr)
	}
}

func TestUnzipExtractAESZipNoPassword(t *testing.T) {
	p := makeZip(t, map[string][]byte{
		"secret.txt": []byte("classified"),
	}, map[string]string{"secret.txt": "hunter2"})
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", p, dest})
	if err != nil {
		t.Fatalf("unzip -x: %v", err)
	}
	if !strings.Contains(out, "encrypted (provide a password") {
		t.Errorf("expected encrypted-skip warning:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "secret.txt")); !os.IsNotExist(statErr) {
		t.Errorf("no password should not write the file (stat err = %v)", statErr)
	}
}

func TestUnzipExtractZipSlip(t *testing.T) {
	p := makeZipSlip(t)
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", p, dest})
	if err != nil {
		t.Fatalf("unzip -x: %v", err)
	}
	if !strings.Contains(out, "unsafe archive path") {
		t.Errorf("expected unsafe-path warning:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); !os.IsNotExist(statErr) {
		t.Errorf("zip-slip escaped dest dir (stat err = %v)", statErr)
	}
}

// ---------------------------------------------------------------------------
// ZIP errors
// ---------------------------------------------------------------------------

func TestUnzipNotArchive(t *testing.T) {
	p := writeFixture(t, "plain.txt", []byte("not an archive"))
	_, err := Run("unzip", []string{p})
	if err == nil || !strings.Contains(err.Error(), "not a ZIP or 7-Zip") {
		t.Fatalf("expected archive-format error, got %v", err)
	}
}

func TestUnzipMissingFile(t *testing.T) {
	_, err := Run("unzip", []string{"/no/such/archive.zip"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUnzipUsage(t *testing.T) {
	_, err := Run("unzip", nil)
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 7-Zip (requires 7zz on PATH)
// ---------------------------------------------------------------------------

// make7z builds a 7z archive via the 7zz CLI. If password is non-empty the
// file contents are encrypted.
func make7z(t *testing.T, password string) string {
	t.Helper()
	if _, err := exec.LookPath("7zz"); err != nil {
		t.Skip("7zz not on PATH; skipping 7z tests")
	}
	dir := t.TempDir()
	content := filepath.Join(dir, "payload.txt")
	if err := os.WriteFile(content, []byte("seven secret"), 0644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "test.7z")
	args := []string{"a"}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, archive, content)
	cmd := exec.Command("7zz", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7zz create: %v\n%s", err, out)
	}
	return archive
}

func TestUnzip7zList(t *testing.T) {
	p := make7z(t, "")
	out, err := Run("unzip", []string{p})
	if err != nil {
		t.Fatalf("unzip 7z list: %v", err)
	}
	if !strings.Contains(out, "7-Zip Archive") {
		t.Errorf("expected 7-Zip header:\n%s", out)
	}
	if !strings.Contains(out, "payload.txt") {
		t.Errorf("expected payload.txt in listing:\n%s", out)
	}
}

func TestUnzip7zExtract(t *testing.T) {
	p := make7z(t, "")
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", p, dest})
	if err != nil {
		t.Fatalf("unzip 7z extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "payload.txt"))
	if err != nil {
		t.Fatalf("read extracted 7z file: %v\nout:\n%s", err, out)
	}
	if string(got) != "seven secret" {
		t.Errorf("extracted content = %q, want %q", got, "seven secret")
	}
}

func TestUnzip7zExtractWithPassword(t *testing.T) {
	p := make7z(t, "letmein")
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", "-p", "letmein", p, dest})
	if err != nil {
		t.Fatalf("unzip 7z extract -p: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "payload.txt"))
	if err != nil {
		t.Fatalf("read extracted 7z file: %v\nout:\n%s", err, out)
	}
	if string(got) != "seven secret" {
		t.Errorf("extracted content = %q, want %q", got, "seven secret")
	}
}

func TestUnzip7zExtractWrongPassword(t *testing.T) {
	p := make7z(t, "letmein")
	dest := filepath.Join(t.TempDir(), "out")
	out, err := Run("unzip", []string{"-x", "-p", "nope", p, dest})
	if err != nil {
		t.Fatalf("unzip 7z extract wrong pw: %v", err)
	}
	if !strings.Contains(out, "nothing extracted") && !strings.Contains(out, "failed payload.txt") {
		t.Errorf("expected failure warning for wrong password:\n%s", out)
	}
	if data, statErr := os.ReadFile(filepath.Join(dest, "payload.txt")); statErr == nil && !bytes.Equal(data, []byte("seven secret")) {
		t.Errorf("wrong password yielded corrupt/partial plaintext: %q", data)
	}
}
