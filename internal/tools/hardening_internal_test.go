package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCappedBufferUnderLimit(t *testing.T) {
	c := &cappedBuffer{limit: 8}
	n, err := c.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 5 {
		t.Errorf("wrote %d bytes, want 5", n)
	}
	if c.truncated {
		t.Error("expected truncated=false for under-limit write")
	}
	if c.buf.String() != "hello" {
		t.Errorf("buf = %q, want %q", c.buf.String(), "hello")
	}
}

func TestCappedBufferExactlyLimit(t *testing.T) {
	c := &cappedBuffer{limit: 5}
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if c.truncated {
		t.Error("expected truncated=false for exact-limit write")
	}
	if c.buf.String() != "hello" {
		t.Errorf("buf = %q, want %q", c.buf.String(), "hello")
	}
}

func TestCappedBufferOverLimit(t *testing.T) {
	c := &cappedBuffer{limit: 5}
	n, err := c.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 5 {
		t.Errorf("reported %d bytes written, want 5 (the amount actually stored)", n)
	}
	if !c.truncated {
		t.Error("expected truncated=true for over-limit write")
	}
	if c.buf.String() != "hello" {
		t.Errorf("buf = %q, want %q (capped at limit)", c.buf.String(), "hello")
	}
}

func TestCappedBufferMultipleWritesCapped(t *testing.T) {
	c := &cappedBuffer{limit: 5}
	c.Write([]byte("abc"))
	c.Write([]byte("def"))
	if c.buf.String() != "abcde" {
		t.Errorf("buf = %q, want %q", c.buf.String(), "abcde")
	}
	if !c.truncated {
		t.Error("expected truncated=true across writes")
	}
}

// wrapError wraps an error so exitCode's errors.As traversal is exercised.
type wrapError struct{ err error }

func (w *wrapError) Error() string { return w.err.Error() }
func (w *wrapError) Unwrap() error { return w.err }

// exitErr runs `sh -c "exit N"` and returns its *exec.ExitError, so the test
// exercises a real process-state-derived exit code without internal hacks.
func exitErr(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit "+strconv.Itoa(code))
	_ = cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no process state for exit %d", code)
	}
	return &exec.ExitError{ProcessState: cmd.ProcessState}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"generic error", errors.New("boom"), 1},
		{"exit code 3", exitErr(t, 3), 3},
		{"exit code 7", exitErr(t, 7), 7},
		{"wrapped exit code 5", &wrapError{err: exitErr(t, 5)}, 5},
	}
	for _, tc := range cases {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("%s: exitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestDownloadFileMatchingChecksum(t *testing.T) {
	body := []byte("mimir-forensic-binary-v1")
	wantSum := sha256.Sum256(body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "tool")
	if err := downloadFile(srv.URL, dest, hex.EncodeToString(wantSum[:])); err != nil {
		t.Fatalf("downloadFile with matching checksum failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("downloaded bytes = %q, want %q", got, body)
	}
}

func TestDownloadFileChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("corrupted-bytes"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "tool")
	err := downloadFile(srv.URL, dest, hex.EncodeToString(make([]byte, 32)))
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !errors.Is(err, errChecksumMismatch) {
		t.Errorf("expected errChecksumMismatch, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("partial file %s should have been removed, stat err = %v", dest, statErr)
	}
}

func TestDownloadFileNon2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "tool")
	if err := downloadFile(srv.URL, dest, ""); err == nil {
		t.Fatal("expected error for 404 status, got nil")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("no file should exist after failed download, stat err = %v", statErr)
	}
}
