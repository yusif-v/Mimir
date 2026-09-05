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

func TestDetectOOXMLLargeFile(t *testing.T) {
	// Create an OOXML-shaped zip larger than sniffLen (8KB) to test
	// that Detect reads the entire file, not just the first 8KB sniff buffer.
	// Zip's central directory lives at EOF, so truncation breaks parsing.
	dir := t.TempDir()
	path := filepath.Join(dir, "large.docx")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Add [Content_Types].xml (OOXML marker)
	f, err := zw.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("<Types/>")); err != nil {
		t.Fatal(err)
	}

	// Add word/document.xml (OOXML marker)
	f2, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Write([]byte("<doc/>")); err != nil {
		t.Fatal(err)
	}

	// Add a large dummy entry to push the file well past 8KB.
	// sniffLen=8192, so we want the archive to be noticeably larger.
	f3, err := zw.Create("word/dummy.bin")
	if err != nil {
		t.Fatal(err)
	}
	dummy := make([]byte, 20000) // 20KB of padding
	if _, err := f3.Write(dummy); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := Detect(path)
	if err != nil {
		t.Fatalf("Detect large OOXML: %v", err)
	}
	if got != OOXML {
		t.Errorf("Detect(large.docx): got %s, want %s", got, OOXML)
	}
}

func TestDetectEmptyFile(t *testing.T) {
	// An empty (0-byte) file should classify as Unknown, not error.
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(path)
	if err != nil {
		t.Errorf("Detect(empty file): got error %v, want (Unknown, nil)", err)
	}
	if got != Unknown {
		t.Errorf("Detect(empty file): got %s, want %s", got, Unknown)
	}
}
