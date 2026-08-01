package tools

import (
	"errors"
	"os/exec"
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
