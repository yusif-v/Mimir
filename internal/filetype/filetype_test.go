// internal/filetype/filetype_test.go
package filetype

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectBytesPE(t *testing.T) {
	data := append([]byte("MZ"), make([]byte, 62)...)
	if got := DetectBytes(data); got != PE {
		t.Errorf("PE header: got %s, want %s", got, PE)
	}
}

func TestDetectBytesELF(t *testing.T) {
	data := append([]byte{0x7f, 'E', 'L', 'F'}, make([]byte, 12)...)
	if got := DetectBytes(data); got != ELF {
		t.Errorf("ELF header: got %s, want %s", got, ELF)
	}
}

func TestDetectBytesOLE(t *testing.T) {
	data := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 8)...)
	if got := DetectBytes(data); got != OLE {
		t.Errorf("OLE header: got %s, want %s", got, OLE)
	}
}

func TestDetectBytesUnknown(t *testing.T) {
	if got := DetectBytes([]byte("just some plain text file\n")); got != Unknown {
		t.Errorf("plain text: got %s, want %s", got, Unknown)
	}
}

func TestDetectBytesPlainZipIsUnknown(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if got := DetectBytes(buf.Bytes()); got != Unknown {
		t.Errorf("plain zip (not Office): got %s, want %s", got, Unknown)
	}
}

func TestDetectBytesOOXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("<Types/>")); err != nil {
		t.Fatal(err)
	}
	f2, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Write([]byte("<doc/>")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if got := DetectBytes(buf.Bytes()); got != OOXML {
		t.Errorf("docx-shaped zip: got %s, want %s", got, OOXML)
	}
}

func TestDetectReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.bin")
	data := append([]byte("MZ"), make([]byte, 62)...)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(path)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != PE {
		t.Errorf("Detect(sample.bin): got %s, want %s", got, PE)
	}
}

func TestDetectMissingFile(t *testing.T) {
	if _, err := Detect("/nonexistent/does-not-exist"); err == nil {
		t.Error("expected error for missing file")
	}
}
