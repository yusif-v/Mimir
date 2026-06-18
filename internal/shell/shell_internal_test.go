package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
)

func TestBannerUsesCurrentVersion(t *testing.T) {
	b := banner()
	if !strings.Contains(b, Version) {
		t.Errorf("banner missing version %q: %s", Version, b)
	}
	if strings.Contains(b, "0.1.0") {
		t.Errorf("banner still shows stale version: %s", b)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		ToolsPath: filepath.Join(dir, "tools"),
		CasesPath: filepath.Join(dir, "cases"),
	}
	return NewApp(cfg)
}

func TestCmdInstallUnknownTool(t *testing.T) {
	app := newTestApp(t)
	err := app.cmdInstall([]string{"definitely-not-a-tool"})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}
}

func TestCmdInstallRequiresArg(t *testing.T) {
	app := newTestApp(t)
	if err := app.cmdInstall(nil); err == nil {
		t.Fatal("expected usage error with no args")
	}
}

func TestCmdBuildNotInstalled(t *testing.T) {
	app := newTestApp(t)
	err := app.cmdBuild([]string{"volatility"})
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected not-installed error, got %v", err)
	}
}

func TestCmdRunRecordsTimeline(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{
		Config: &config.Config{CasesPath: base},
		Events: bus,
		Cases:  cases.NewManager(base, bus),
		Tools:  tools.NewRegistry(bus),
		Runner: tools.NewRunner(bus),
		Output: tools.NewOutputCapture(bus),
	}

	if _, err := app.Cases.Create("c1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, err := app.Cases.Open("c1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Evidence file to hash.
	ev := filepath.Join(c.Path, "evidence", "f.txt")
	if err := os.WriteFile(ev, []byte("abc"), 0644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	if err := app.cmdRun([]string{"hash", ev}); err != nil {
		t.Fatalf("cmdRun: %v", err)
	}

	// An output file was written.
	outs, _ := os.ReadDir(filepath.Join(c.Path, "output"))
	if len(outs) == 0 {
		t.Fatal("expected an output file")
	}

	// A tool_run event is on the timeline.
	var found bool
	for _, e := range c.Timeline() {
		if e.Type == "tool_run" && e.Payload["tool"] == "hash" {
			found = true
			if e.Payload["success"] != true {
				t.Errorf("expected success=true, got %v", e.Payload["success"])
			}
			if _, ok := e.Payload["return_code"]; !ok {
				t.Errorf("expected return_code in payload")
			}
			if _, ok := e.Payload["duration_ms"]; !ok {
				t.Errorf("expected duration_ms in payload")
			}
			if _, ok := e.Payload["output_file"]; !ok {
				t.Errorf("expected output_file in payload")
			}
			if _, ok := e.Payload["args"]; !ok {
				t.Errorf("expected args in payload")
			}
		}
	}
	if !found {
		t.Fatalf("no tool_run event found; timeline=%+v", c.Timeline())
	}
}

func TestCmdRunRecordsFailedRun(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{
		Config: &config.Config{CasesPath: base},
		Events: bus,
		Cases:  cases.NewManager(base, bus),
		Tools:  tools.NewRegistry(bus),
		Runner: tools.NewRunner(bus),
		Output: tools.NewOutputCapture(bus),
	}

	if _, err := app.Cases.Create("c2"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := app.Cases.Open("c2"); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Run hash on a path that does not exist — builtin will fail with a non-nil Error.
	err := app.cmdRun([]string{"hash", "/no/such/file/xyz"})
	if err == nil {
		t.Fatal("expected cmdRun to return a non-nil error for failed run")
	}

	c := app.Cases.Current()
	var found bool
	for _, e := range c.Timeline() {
		if e.Type == "tool_run" && e.Payload["tool"] == "hash" {
			found = true
			if e.Payload["success"] != false {
				t.Errorf("expected success=false, got %v", e.Payload["success"])
			}
			stderr, ok := e.Payload["stderr"]
			if !ok {
				t.Errorf("expected stderr key in failed tool_run payload; payload=%+v", e.Payload)
			} else if s, _ := stderr.(string); s == "" {
				t.Errorf("expected non-empty stderr in payload")
			}
		}
	}
	if !found {
		t.Fatalf("no tool_run event found for failed run; timeline=%+v", c.Timeline())
	}
}

func TestCmdTimelineNoCase(t *testing.T) {
	app := &App{Cases: cases.NewManager(t.TempDir(), events.NewBus())}
	if err := app.cmdTimeline(nil); err == nil {
		t.Fatal("expected error when no case is open")
	}
}

// TestCmdBuildDockerDown verifies that building an installed docker tool while
// the Docker daemon is unreachable yields the friendly docker-down error rather
// than a raw `docker build` failure. Skips when Docker is actually available.
func TestCmdBuildDockerDown(t *testing.T) {
	app := newTestApp(t)
	if app.Runner.DockerReachable() {
		t.Skip("docker daemon is reachable; this test exercises the docker-down path")
	}
	// Install the volatility template so it is registered as a docker tool.
	dest := filepath.Join(app.Config.ToolsPath, "volatility")
	if err := catalog.Install("volatility", dest); err != nil {
		t.Fatalf("seed install failed: %v", err)
	}
	if err := app.Tools.DiscoverFromPath(app.Config.ToolsPath); err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	err := app.cmdBuild([]string{"volatility"})
	if err == nil || !strings.Contains(err.Error(), "docker is not available") {
		t.Fatalf("expected docker-not-available error, got %v", err)
	}
}

func TestCmdShellQuietsExitError(t *testing.T) {
	app := &App{}
	// `false` exits 1; its nonzero exit must NOT surface as a Mimir error.
	if err := app.cmdShell("false"); err != nil {
		t.Errorf("expected nil for nonzero subprocess exit, got %v", err)
	}
	// A command-not-found inside sh exits 127; sh prints its own message.
	if err := app.cmdShell("this_command_definitely_does_not_exist_xyz"); err != nil {
		t.Errorf("expected nil for in-shell command-not-found, got %v", err)
	}
}

func TestCmdShellSucceeds(t *testing.T) {
	app := &App{}
	if err := app.cmdShell("true"); err != nil {
		t.Errorf("expected nil for successful command, got %v", err)
	}
}

func TestContextLineNoCase(t *testing.T) {
	t.Setenv("MIMIR_ASCII", "1")
	app := &App{Cases: cases.NewManager(t.TempDir(), events.NewBus())}
	line := app.contextLine()
	if !strings.Contains(line, "mimir") {
		t.Fatalf("expected 'mimir' segment, got %q", line)
	}
	if strings.Contains(line, "❯") {
		t.Fatalf("context line should not contain the input marker: %q", line)
	}
}

func TestContextLineWithOpenCase(t *testing.T) {
	t.Setenv("MIMIR_ASCII", "1")
	base := t.TempDir()
	app := &App{Cases: cases.NewManager(base, events.NewBus())}
	if _, err := app.Cases.Create("incident-42"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Cases.Open("incident-42"); err != nil {
		t.Fatal(err)
	}
	line := app.contextLine()
	if !strings.Contains(line, "incident-42") {
		t.Fatalf("expected case name in context line, got %q", line)
	}
}

func TestBuildPromptMarker(t *testing.T) {
	t.Setenv("MIMIR_ASCII", "1")
	app := &App{}
	if got := app.buildPrompt(); !strings.Contains(got, "|>") {
		t.Fatalf("ASCII marker expected, got %q", got)
	}
}

func TestCmdEvidenceAddAndList(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{
		Config: &config.Config{CasesPath: base},
		Events: bus,
		Cases:  cases.NewManager(base, bus),
	}
	if _, err := app.Cases.Create("c1"); err != nil {
		t.Fatal(err)
	}
	c, err := app.Cases.Open("c1")
	if err != nil {
		t.Fatal(err)
	}
	// External source file
	src := filepath.Join(base, "ext.bin")
	if err := os.WriteFile(src, []byte("abc"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdEvidence([]string{"add", src, "--tag", "malware"}); err != nil {
		t.Fatalf("evidence add: %v", err)
	}
	// File copied into evidence/
	if _, err := os.Stat(filepath.Join(c.Path, "evidence", "ext.bin")); err != nil {
		t.Fatalf("evidence file not copied: %v", err)
	}
	ev := c.Evidence()
	if len(ev) != 1 || ev[0].SHA256 == "" || !contains(ev[0].Tags, "malware") {
		t.Fatalf("evidence record wrong: %+v", ev)
	}
	// Timeline event recorded
	found := false
	for _, e := range c.Timeline() {
		if e.Type == "evidence_added" {
			found = true
		}
	}
	if !found {
		t.Fatal("no evidence_added timeline event")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func newTestAppWithCase(t *testing.T) (*App, *cases.Case) {
	t.Helper()
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{
		Config: &config.Config{CasesPath: base},
		Events: bus,
		Cases:  cases.NewManager(base, bus),
	}
	if _, err := app.Cases.Create("tc"); err != nil {
		t.Fatalf("create case: %v", err)
	}
	c, err := app.Cases.Open("tc")
	if err != nil {
		t.Fatalf("open case: %v", err)
	}
	return app, c
}

func TestCmdEvidenceOverwriteRefused(t *testing.T) {
	app, c := newTestAppWithCase(t)

	// Add first file "a.bin" containing "abc" from an external dir.
	dir1 := t.TempDir()
	src1 := filepath.Join(dir1, "a.bin")
	if err := os.WriteFile(src1, []byte("abc"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdEvidence([]string{"add", src1}); err != nil {
		t.Fatalf("first add failed: %v", err)
	}

	// Record original hash.
	ev := c.Evidence()
	if len(ev) == 0 {
		t.Fatal("no evidence after first add")
	}
	originalHash := ev[0].SHA256

	// Attempt to add a DIFFERENT file also named "a.bin" from a second dir.
	dir2 := t.TempDir()
	src2 := filepath.Join(dir2, "a.bin")
	if err := os.WriteFile(src2, []byte("xyz"), 0644); err != nil {
		t.Fatal(err)
	}
	err := app.cmdEvidence([]string{"add", src2})
	if err == nil {
		t.Fatal("expected overwrite-refusal error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Original hash must be unchanged.
	latest := c.Evidence()
	if latest[0].SHA256 != originalHash {
		t.Fatalf("hash changed after refused overwrite: got %q want %q", latest[0].SHA256, originalHash)
	}
}

func TestCmdEvidenceInPlace(t *testing.T) {
	app, c := newTestAppWithCase(t)

	// Write a file directly into the case's evidence/ directory.
	evidenceDir := filepath.Join(c.Path, "evidence")
	if err := os.MkdirAll(evidenceDir, 0755); err != nil {
		t.Fatal(err)
	}
	inplace := filepath.Join(evidenceDir, "inplace.bin")
	if err := os.WriteFile(inplace, []byte("inplace-content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Add the file using its absolute path (same as dest → in-place).
	if err := app.cmdEvidence([]string{"add", inplace}); err != nil {
		t.Fatalf("in-place add failed: %v", err)
	}

	ev := c.Evidence()
	if len(ev) == 0 {
		t.Fatal("no evidence recorded for in-place file")
	}
	if ev[0].SHA256 == "" {
		t.Fatal("expected non-empty SHA256 for in-place evidence")
	}
	if ev[0].Name != "inplace.bin" {
		t.Fatalf("unexpected evidence name: %q", ev[0].Name)
	}
}

func TestCmdEvidenceVerifyMissingName(t *testing.T) {
	app, _ := newTestAppWithCase(t)

	err := app.cmdEvidence([]string{"verify", "ghost"})
	if err == nil {
		t.Fatal("expected error when verifying non-existent evidence name, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error should mention the missing name, got: %v", err)
	}
}

func TestCmdIOCExtractAndTrack(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{Config: &config.Config{CasesPath: base}, Events: bus, Cases: cases.NewManager(base, bus)}
	if _, err := app.Cases.Create("c1"); err != nil {
		t.Fatal(err)
	}
	c, err := app.Cases.Open("c1")
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(c.Path, "evidence", "n.txt")
	os.MkdirAll(filepath.Dir(f), 0755)
	os.WriteFile(f, []byte("beacon to 9.9.9.9 and evil.example.com"), 0644)

	if err := app.cmdIOC([]string{f}); err != nil {
		t.Fatalf("ioc: %v", err)
	}
	iocs := c.IOCs()
	if len(iocs) < 2 {
		t.Fatalf("want >=2 IOCs tracked, got %d (%+v)", len(iocs), iocs)
	}
	found := false
	for _, e := range c.Timeline() {
		if e.Type == "ioc_extracted" {
			found = true
		}
	}
	if !found {
		t.Fatal("no ioc_extracted timeline event")
	}
}

func TestFilterTimeline(t *testing.T) {
	evs := []cases.TimelineEvent{
		{Type: "tool_run", Payload: map[string]any{"tool": "hash"}},
		{Type: "note", Payload: map[string]any{"content": "suspicious binary"}},
		{Type: "tool_run", Payload: map[string]any{"tool": "strings"}},
	}
	// type filter
	got := filterTimeline(evs, []string{"note"}, "")
	if len(got) != 1 || got[0].Type != "note" {
		t.Fatalf("type filter: got %+v", got)
	}
	// grep filter
	got = filterTimeline(evs, nil, "strings")
	if len(got) != 1 || got[0].Payload["tool"] != "strings" {
		t.Fatalf("grep filter: got %+v", got)
	}
	// no filters → all
	if len(filterTimeline(evs, nil, "")) != 3 {
		t.Fatal("no filter should return all")
	}
}
