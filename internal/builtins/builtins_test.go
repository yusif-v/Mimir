package builtins

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

func TestHashKnownVector(t *testing.T) {
	p := writeFixture(t, "f.txt", []byte("abc"))
	out, err := Run("hash", []string{p})
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	// Known digests of "abc".
	wantSHA256 := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	wantMD5 := "900150983cd24fb0d6963f7d28e17f72"
	if !strings.Contains(out, wantSHA256) {
		t.Errorf("sha256 missing: %s", out)
	}
	if !strings.Contains(out, wantMD5) {
		t.Errorf("md5 missing: %s", out)
	}
}

func TestStringsExtractsRuns(t *testing.T) {
	data := []byte{0x00, 'h', 'e', 'l', 'l', 'o', 0x01, 'h', 'i', 0x00}
	p := writeFixture(t, "b.bin", data)
	out, err := Run("strings", []string{p}) // default min length 4
	if err != nil {
		t.Fatalf("strings: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in output: %q", out)
	}
	if strings.Contains(out, "hi") {
		t.Errorf("'hi' is below min length 4, should be excluded: %q", out)
	}
}

func TestStringsMinLengthFlag(t *testing.T) {
	data := []byte{0x00, 'h', 'i', 0x00}
	p := writeFixture(t, "b.bin", data)
	out, err := Run("strings", []string{"-n", "2", p})
	if err != nil {
		t.Fatalf("strings: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("expected 'hi' with -n 2: %q", out)
	}
}

func TestFileIdentifiesPNG(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	p := writeFixture(t, "img", png)
	out, err := Run("file", []string{p})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "png") {
		t.Errorf("expected PNG identification: %q", out)
	}
}

func TestRunUnknownTool(t *testing.T) {
	if _, err := Run("nope", nil); err == nil {
		t.Fatal("expected error for unknown builtin")
	}
}

func TestMissingFileErrors(t *testing.T) {
	if _, err := Run("hash", []string{"/no/such/file"}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestHexdump(t *testing.T) {
	p := writeFixture(t, "h.bin", []byte("AB"))
	out, err := Run("hexdump", []string{p})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "41 42") || !strings.Contains(out, "AB") {
		t.Fatalf("unexpected hexdump:\n%s", out)
	}
}

func TestEntropy(t *testing.T) {
	const0 := writeFixture(t, "c.bin", bytes.Repeat([]byte{0x00}, 1024))
	out, err := Run("entropy", []string{const0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "0.00") {
		t.Fatalf("constant bytes should be ~0 entropy, got:\n%s", out)
	}
}

func TestDecodeBase64(t *testing.T) {
	out, err := Run("decode", []string{"--base64", "aGVsbG8="})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("want 'hello', got %q", out)
	}
}

func TestDecodeAutoHex(t *testing.T) {
	out, err := Run("decode", []string{"68656c6c6f"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("want 'hello' from auto hex, got %q", out)
	}
}

func TestDecodeAutoPrefersHex(t *testing.T) {
	// "68656c6c6f6f" is valid hex ("helloo") AND a valid base64 length (12).
	// Auto-detect must pick hex first.
	out, err := Run("decode", []string{"68656c6c6f6f"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "(hex)") || !strings.Contains(out, "helloo") {
		t.Fatalf("want hex-decoded 'helloo', got %q", out)
	}
}
