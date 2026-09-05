// Package filetype classifies a file by its magic bytes only — no libmagic
// dependency, no reliance on file extensions. It exists to drive automatic
// static-tool selection in the `analyze` command.
package filetype

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
)

// Kind is a coarse file-type classification.
type Kind string

const (
	PE      Kind = "pe"
	ELF     Kind = "elf"
	OLE     Kind = "ole"   // legacy Office: doc/xls/ppt (CDF container)
	OOXML   Kind = "ooxml" // modern Office: docx/xlsx/pptx (zip container)
	Unknown Kind = "unknown"
)

// sniffLen is the number of bytes read from disk before classifying — large
// enough to cover every magic-byte check below, small enough to never read a
// multi-GB sample fully into memory just to classify it.
const sniffLen = 8192

var (
	pePrefix  = []byte("MZ")
	elfPrefix = []byte{0x7f, 'E', 'L', 'F'}
	olePrefix = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	zipPrefix = []byte("PK\x03\x04")
)

// Detect reads the leading bytes of the file at path and classifies it.
func Detect(path string) (Kind, error) {
	f, err := os.Open(path)
	if err != nil {
		return Unknown, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	buf := make([]byte, sniffLen)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return Unknown, fmt.Errorf("read %s: %w", path, err)
	}
	return DetectBytes(buf[:n]), nil
}

// DetectBytes classifies an in-memory buffer. Only a small prefix is
// examined for the simple magic-byte checks; the OOXML check additionally
// needs to walk the zip's central directory, so it re-reads data as a
// bytes.Reader zip archive.
func DetectBytes(data []byte) Kind {
	switch {
	case bytes.HasPrefix(data, pePrefix):
		return PE
	case bytes.HasPrefix(data, elfPrefix):
		return ELF
	case bytes.HasPrefix(data, olePrefix):
		return OLE
	case bytes.HasPrefix(data, zipPrefix):
		if isOOXML(data) {
			return OOXML
		}
		return Unknown
	default:
		return Unknown
	}
}

// isOOXML reports whether a zip-shaped buffer looks like an Office Open XML
// document: it must actually parse as a zip (a bare 4-byte prefix match
// isn't enough) and contain the telltale OOXML entries.
func isOOXML(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		switch {
		case f.Name == "[Content_Types].xml":
			return true
		case len(f.Name) > 5 && (f.Name[:5] == "word/" || f.Name[:3] == "xl/" || f.Name[:4] == "ppt/"):
			return true
		}
	}
	return false
}
