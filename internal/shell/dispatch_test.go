package shell

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDispatchRemovedCommandsNameReplacement(t *testing.T) {
	app := newTestApp(t)
	tests := []struct {
		line string
		want string
	}{
		{"run hash sample.bin", "tool run"},
		{"tools", "tool list"},
		{"cases", "case list"},
		{"status", "case status"},
		{"evidence", "evidence list"},
		{"ioc", "ioc list"},
		{"timeline", "timeline list"},
		{"install yara", "tool install"},
		{"build volatility", "tool build"},
		{"use hash", "tool info"},
		{"analyze sample.bin", "tool analyze"},
		{"dashboard", "tool dashboard"},
		{"dash", "tool dashboard"},
		{"volatility windows.pslist", "memory"},
		{"mem windows.pslist", "memory"},
		{"regripper SYSTEM.hive", "registry"},
		{"sleuthkit fls", "disk"},
		{"case -n demo", "case create"},
		{"case -o demo", "case open"},
		{"case -c", "case close"},
	}
	for _, tc := range tests {
		err := app.dispatch(tc.line)
		if err == nil {
			t.Errorf("dispatch(%q): expected an unknown-command error", tc.line)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "unknown command") {
			t.Errorf("dispatch(%q) = %q, want unknown command", tc.line, msg)
		}
		if !strings.Contains(msg, tc.want) {
			t.Errorf("dispatch(%q) = %q, want replacement %q", tc.line, msg, tc.want)
		}
	}
}

func TestDispatchCanonicalCaseActions(t *testing.T) {
	app := newTestApp(t)

	if err := app.dispatch("case create demo"); err != nil {
		t.Fatalf("case create: %v", err)
	}
	all, err := app.Cases.List()
	if err != nil {
		t.Fatalf("list cases: %v", err)
	}
	found := false
	for _, c := range all {
		if c.Name == "demo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("case create did not create the case: %+v", all)
	}
	for _, line := range []string{
		"case open demo",
		"case status",
		"case list",
		"case templates",
		"case close",
	} {
		if err := app.dispatch(line); err != nil {
			t.Errorf("%s: %v", line, err)
		}
	}
}

func TestDispatchCanonicalToolActionsRoute(t *testing.T) {
	app := newTestApp(t)

	if err := app.dispatch("tool list"); err != nil {
		t.Fatalf("tool list: %v", err)
	}

	// These actions need an argument or an open case; they must reach their
	// handler rather than fall through to the removed-command map.
	for _, line := range []string{
		"tool run",
		"tool install",
		"tool build",
		"tool info",
		"tool analyze",
	} {
		err := app.dispatch(line)
		if err == nil {
			t.Errorf("%s: expected a usage error", line)
			continue
		}
		if strings.Contains(err.Error(), "unknown command") {
			t.Errorf("%s: routed to the removed-command map: %v", line, err)
		}
	}
}

func TestDispatchCanonicalForensicsActionsRoute(t *testing.T) {
	app := newTestApp(t)
	for _, line := range []string{"memory windows.pslist", "disk fls", "registry SYSTEM.hive"} {
		err := app.dispatch(line)
		if err == nil {
			t.Errorf("%s: expected a no-case error", line)
			continue
		}
		if strings.Contains(err.Error(), "unknown command") {
			t.Errorf("%s: routed to the removed-command map: %v", line, err)
		}
	}
}

func TestHelpGroupedByDomain(t *testing.T) {
	app := newTestApp(t)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := app.cmdHelp(nil)
	w.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatalf("cmdHelp: %v", runErr)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	help := buf.String()

	for _, want := range []string{
		"case create", "case open", "case close", "case status",
		"evidence list", "ioc extract", "ioc from-output",
		"timeline list", "tool run", "tool info", "tool dashboard",
		"memory", "disk", "registry", "Other",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q:\n%s", want, help)
		}
	}
	for _, removed := range []string{
		"case -n", "case -o", "case -c", "regripper", "volatility <plugin>",
	} {
		if strings.Contains(help, removed) {
			t.Errorf("help still advertises removed name %q:\n%s", removed, help)
		}
	}
}
