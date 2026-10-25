package shell

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yusif-v/mimir/internal/ai"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/catalog"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/theme"
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
		Cases:  cases.NewManager(base, bus, nil, nil),
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
		Cases:  cases.NewManager(base, bus, nil, nil),
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
	app := &App{Cases: cases.NewManager(t.TempDir(), events.NewBus(), nil, nil)}
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
	app := &App{Cases: cases.NewManager(t.TempDir(), events.NewBus(), nil, nil), Theme: theme.DefaultTheme()}
	line := app.contextLine()
	if !strings.Contains(strings.ToLower(line), "mimir") {
		t.Fatalf("expected 'mimir' segment, got %q", line)
	}
	if strings.Contains(line, "❯") {
		t.Fatalf("context line should not contain the input marker: %q", line)
	}
}

func TestContextLineWithOpenCase(t *testing.T) {
	t.Setenv("MIMIR_ASCII", "1")
	base := t.TempDir()
	app := &App{Cases: cases.NewManager(base, events.NewBus(), nil, nil), Theme: theme.DefaultTheme()}
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
	app := &App{Cases: cases.NewManager(t.TempDir(), events.NewBus(), nil, nil), Theme: theme.DefaultTheme()}
	if got := app.promptMarker(); !strings.Contains(got, ">") {
		t.Fatalf("ASCII marker expected, got %q", got)
	}
}

func TestCmdEvidenceAddAndList(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{
		Config: &config.Config{CasesPath: base},
		Events: bus,
		Cases:  cases.NewManager(base, bus, nil, nil),
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
		Cases:  cases.NewManager(base, bus, nil, nil),
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
	app := &App{Config: &config.Config{CasesPath: base}, Events: bus, Cases: cases.NewManager(base, bus, nil, nil)}
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

	if err := app.cmdIOC([]string{"extract", f}); err != nil {
		t.Fatalf("ioc extract: %v", err)
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

func TestCaseMatches(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	mgr := cases.NewManager(base, bus, nil, nil)
	c, _ := mgr.Create("incident-42")
	c.AddNote("found a suspicious beacon", "analyst")
	_ = c.Save()

	ok, where := caseMatches(c, "beacon")
	if !ok {
		t.Fatal("expected note match")
	}
	if !strings.Contains(where, "notes") {
		t.Errorf("hint should mention notes, got %q", where)
	}
	if ok, _ := caseMatches(c, "nothinghere"); ok {
		t.Fatal("unexpected match")
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

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1929912432, "1.8 GB"},
		{18253611008, "17.0 GB"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestAIUseSwitchesModelAndValidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "qwen2.5:3b"},
					{"name": "sphinx:latest"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	aiShell, err := ai.NewShell(ai.AIConfig{Provider: "ollama", Model: "qwen2.5:3b", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}
	app := &App{AI: aiShell, Config: &config.Config{HomeDir: t.TempDir()}}

	// Unknown model should be rejected (validation against provider list).
	if err := app.aiUse([]string{"nonexistent:model"}); err == nil {
		t.Fatal("expected rejection of unavailable model")
	}
	// Known model switches.
	if err := app.aiUse([]string{"sphinx:latest"}); err != nil {
		t.Fatalf("aiUse: %v", err)
	}
	if app.AI.Model() != "sphinx:latest" {
		t.Errorf("model not switched: %q", app.AI.Model())
	}
	if app.Config.AI.Model != "sphinx:latest" {
		t.Errorf("config model not updated: %q", app.Config.AI.Model)
	}
}

func TestAIUseRequiresArg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()
	aiShell, _ := ai.NewShell(ai.AIConfig{Provider: "ollama", Model: "qwen2.5:3b", BaseURL: srv.URL})
	app := &App{AI: aiShell, Config: &config.Config{HomeDir: t.TempDir()}}
	if err := app.aiUse(nil); err == nil {
		t.Fatal("expected usage error with no model arg")
	}
}

func TestCmdReportNoAIWritesMarkdown(t *testing.T) {
	base := t.TempDir()
	bus := events.NewBus()
	app := &App{
		Config: &config.Config{CasesPath: base},
		Events: bus,
		Cases:  cases.NewManager(base, bus, nil, nil),
		Tools:  tools.NewRegistry(bus),
		Runner: tools.NewRunner(bus),
		Output: tools.NewOutputCapture(bus),
	}
	c, err := app.Cases.Create("c1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	c, err = app.Cases.Open("c1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := c.AppendEvidence(cases.EvidenceRecord{Op: "add", Name: "evil.exe", SHA256: "deadbeef", Size: 1024, Tags: []string{"malware"}}); err != nil {
		t.Fatalf("evidence: %v", err)
	}

	// No AI configured, --no-ai guarantees deterministic output.
	if err := app.cmdReport([]string{"--no-ai"}); err != nil {
		t.Fatalf("cmdReport: %v", err)
	}
	out := filepath.Join(c.Path, "output", "report.md")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	md := string(data)
	for _, want := range []string{"# Incident Report: c1", "evil.exe", "## Evidence"} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q\n%s", want, md)
		}
	}
	// A timeline event records the report generation.
	if !reportGenerated(c) {
		t.Error("expected report_generated timeline event")
	}
}

func reportGenerated(c *cases.Case) bool {
	for _, e := range c.Timeline() {
		if e.Type == "report_generated" {
			return true
		}
	}
	return false
}

func TestResolveDumpAutoSelectsMemoryTag(t *testing.T) {
	c := &cases.Case{Name: "t", Path: t.TempDir()}
	if err := c.Scaffold(); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	// Write two evidence files on disk; one tagged memory-dump.
	diskImg := filepath.Join(c.Path, "evidence", "disk.dd")
	memDump := filepath.Join(c.Path, "evidence", "Challenge.raw")
	if err := os.WriteFile(diskImg, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memDump, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format(time.RFC3339)
	if err := c.AppendEvidence(cases.EvidenceRecord{Op: "add", Name: "disk.dd", Time: now}); err != nil {
		t.Fatal(err)
	}
	if err := c.AppendEvidence(cases.EvidenceRecord{Op: "add", Name: "Challenge.raw", Tags: []string{"memory-dump"}, Time: now}); err != nil {
		t.Fatal(err)
	}

	// No -f given → should auto-pick the memory-dump tag.
	got, err := resolveDump(c, "")
	if err != nil {
		t.Fatalf("resolveDump: %v", err)
	}
	if got != memDump {
		t.Errorf("resolveDump auto-select: got %s, want %s", got, memDump)
	}

	// Explicit -f by name → resolves under evidence dir.
	got2, err := resolveDump(c, "disk.dd")
	if err != nil {
		t.Fatalf("resolveDump disk.dd: %v", err)
	}
	if got2 != diskImg {
		t.Errorf("resolveDump explicit: got %s, want %s", got2, diskImg)
	}

	// Unknown dump → error.
	if _, err := resolveDump(c, "nope.raw"); err == nil {
		t.Error("resolveDump unknown: expected error")
	}
}

func TestVolatilityRequiresOpenCase(t *testing.T) {
	app := newTestApp(t)
	// No case opened.
	if err := app.cmdVolatility([]string{"windows.pslist"}); err == nil {
		t.Error("cmdVolatility with no case: expected error")
	}
}

func TestVolatilityHints(t *testing.T) {
	// cmdscan-style output → command hints.
	cmdOut := "CommandProcess: cmd.exe\nC:\\Windows\\system32> whoami /all\nC:\\> ipconfig"
	hints := volatilityHints("windows.cmdscan", cmdOut)
	var cmds []string
	for _, h := range hints {
		if h.typ == "command" {
			cmds = append(cmds, h.val)
		}
	}
	if len(cmds) == 0 {
		t.Fatal("expected command hints from cmdscan output")
	}

	// hashdump-style output → ntlm hints.
	hashOut := "Administrator:500:abcdef00000000000000000000000000:101da33f44e92c27835e64322d72e8b7:::"
	hints2 := volatilityHints("windows.hashdump", hashOut)
	var found bool
	for _, h := range hints2 {
		if h.typ == "ntlm" && h.val == "101da33f44e92c27835e64322d72e8b7" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ntlm hint from hashdump output, got %+v", hints2)
	}

	// pslist (no hints).
	if len(volatilityHints("windows.pslist", "PID Name\n4 System")) != 0 {
		t.Error("pslist should produce no plugin hints")
	}
}

func TestCaptureIntoAppendsTimelineAndIOC(t *testing.T) {
	c := &cases.Case{Name: "cap", Path: t.TempDir()}
	if err := c.Scaffold(); err != nil {
		t.Fatal(err)
	}
	if err := c.AppendEvidence(cases.EvidenceRecord{Op: "add", Name: "disk.dd", Time: "2026-07-11T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	out := "PID Name\n4 System\nConnecting to 1.2.3.4\n"
	if err := captureInto(c, "sleuthkit", "icat -r img 0", "disk.dd", out); err != nil {
		t.Fatalf("captureInto: %v", err)
	}
	var gotRun, gotIP bool
	for _, e := range c.Timeline() {
		if e.Type == "tool_run" && e.Payload["command"] == "icat -r img 0" {
			gotRun = true
		}
	}
	for _, ic := range c.IOCs() {
		if ic.Type == "ipv4" && ic.Value == "1.2.3.4" {
			gotIP = true
		}
	}
	if !gotRun {
		t.Error("expected a tool_run timeline event from captureInto")
	}
	if !gotIP {
		t.Error("expected auto-IOC (1.2.3.4) from captureInto output")
	}
}

func TestCaptureIntoVolatilityHints(t *testing.T) {
	c := &cases.Case{Name: "cap", Path: t.TempDir()}
	if err := c.Scaffold(); err != nil {
		t.Fatal(err)
	}
	cmdOut := "C:\\Windows\\system32> whoami"
	if err := captureInto(c, "volatility", "vol windows.cmdscan", "Challenge.raw", cmdOut); err != nil {
		t.Fatalf("captureInto: %v", err)
	}
	var gotCmd bool
	for _, ic := range c.IOCs() {
		if ic.Type == "command" {
			gotCmd = true
		}
	}
	if !gotCmd {
		t.Error("expected volatility command hint from captureInto")
	}
}

func TestCaptureIntoCapaHints(t *testing.T) {
	c := &cases.Case{Name: "cap", Path: t.TempDir()}
	if err := c.Scaffold(); err != nil {
		t.Fatal(err)
	}
	if err := captureInto(c, "capa", "capa -j sample.exe", "sample.exe", capaFixtureJSON); err != nil {
		t.Fatalf("captureInto: %v", err)
	}
	var gotCapability bool
	for _, ic := range c.IOCs() {
		if ic.Type == "capability" && ic.Value == "T1055 Process Injection" {
			gotCapability = true
		}
	}
	if !gotCapability {
		t.Error("expected capa capability hint (T1055) from captureInto")
	}
}

func TestSuggestToolByExtension(t *testing.T) {
	cases := []struct {
		path string
		want string // substring expected in suggestions
	}{
		{"memory.raw", "volatility"},
		{"SYSTEM.hive", "regripper"},
		{"disk.dd", "sleuthkit"},
		{"capture.pcapng", "tshark"},
		{"security.evtx", "chainsaw"},
		{"malware.exe", "capa"},
		{"doc.docm", "oletools"},
		{"link.lnk", "lnkparse"},
	}
	for _, tc := range cases {
		got := suggestTool(tc.path)
		if len(got) == 0 {
			t.Errorf("%s: expected a suggestion, got none", tc.path)
			continue
		}
		var matched bool
		for _, g := range got {
			if strings.Contains(g, tc.want) {
				matched = true
			}
		}
		if !matched {
			t.Errorf("%s: want %q in %v", tc.path, tc.want, got)
		}
	}
}

func TestNetworkHintsFromTshark(t *testing.T) {
	out := "http.host example.com\ndns.qry.name evil.test\nhttp.request.uri /payload.bin\n1.2.3.4	5.6.7.8	443"
	hints := networkHints("tshark -T fields", out)
	var hasDomain, hasURL, hasIP bool
	for _, h := range hints {
		switch h.typ {
		case "domain":
			hasDomain = true
		case "url":
			hasURL = true
		case "ip":
			if h.val == "1.2.3.4" || h.val == "5.6.7.8" {
				hasIP = true
			}
		}
	}
	if !hasDomain || !hasURL || !hasIP {
		t.Errorf("networkHints missed expected indicators: %+v", hints)
	}
}

func TestIOCDiffAddedRemoved(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.txt")
	bPath := filepath.Join(dir, "b.txt")
	os.WriteFile(aPath, []byte("192.168.1.1\n1.2.3.4\n"), 0644)
	os.WriteFile(bPath, []byte("1.2.3.4\n9.9.9.9\n"), 0644)

	// Capture os.Stdout (cmdIOCDiff prints there, like the other commands).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := app.cmdIOCDiff([]string{aPath, bPath})
	w.Close()
	os.Stdout = old
	if runErr != nil {
		t.Fatalf("cmdIOCDiff: %v", runErr)
	}
	var buf bytes.Buffer
	io.Copy(&buf, r)
	s := buf.String()
	if !strings.Contains(s, "added (only in B):") {
		t.Errorf("expected added line, got:\n%s", s)
	}
	if !strings.Contains(s, "removed (only in A):") {
		t.Errorf("expected removed line, got:\n%s", s)
	}
	if !strings.Contains(s, "shared:") {
		t.Errorf("expected shared line, got:\n%s", s)
	}
	if !strings.Contains(s, "+ ipv4:9.9.9.9") {
		t.Errorf("expected + ipv4:9.9.9.9, got:\n%s", s)
	}
	if !strings.Contains(s, "- ipv4:192.168.1.1") {
		t.Errorf("expected - ipv4:192.168.1.1, got:\n%s", s)
	}
}

func TestTimelineCorrelateGroupsByTool(t *testing.T) {
	app, _ := newTestAppWithCase(t)
	c, err := app.Cases.Open("tc")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.AppendEvent(cases.TimelineEvent{Type: "tool_run", Timestamp: "2026-07-12T00:00:00Z", Payload: map[string]any{"tool": "volatility", "command": "vol windows.pslist"}})
	_ = c.AppendEvent(cases.TimelineEvent{Type: "tool_run", Timestamp: "2026-07-12T00:01:00Z", Payload: map[string]any{"tool": "sleuthkit", "command": "fls -r img"}})
	_ = c.AppendIOC(cases.IOCRecord{Type: "ipv4", Value: "1.2.3.4", Source: "volatility:vol windows.netscan"})

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = app.cmdTimeline([]string{"correlate"})
	w.Close()
	os.Stdout = old
	io.Copy(&buf, r)
	if err != nil {
		t.Fatalf("cmdTimeline correlate: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "Tool-run correlation") {
		t.Errorf("expected correlation header, got:\n%s", s)
	}
	if !strings.Contains(s, "volatility") || !strings.Contains(s, "sleuthkit") {
		t.Errorf("expected both tools listed, got:\n%s", s)
	}
	if !strings.Contains(s, "Indicators by type") {
		t.Errorf("expected indicator-by-type table, got:\n%s", s)
	}
}

func TestCorrelateCrossDomain(t *testing.T) {
	app, _ := newTestAppWithCase(t)
	c, err := app.Cases.Open("tc")
	if err != nil {
		t.Fatal(err)
	}
	// Same IP seen via volatility (memory) AND zeek (network) → cross-domain.
	_ = c.AppendIOC(cases.IOCRecord{Type: "ipv4", Value: "9.9.9.9", Source: "volatility:vol windows.netscan"})
	_ = c.AppendIOC(cases.IOCRecord{Type: "ipv4", Value: "9.9.9.9", Source: "zeek:conn.log"})

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = app.cmdCorrelate(nil)
	w.Close()
	os.Stdout = old
	io.Copy(&buf, r)
	if err != nil {
		t.Fatalf("cmdCorrelate: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "Cross-domain indicators") {
		t.Errorf("expected cross-domain section, got:\n%s", s)
	}
	if !strings.Contains(s, "9.9.9.9") {
		t.Errorf("expected the cross-domain IP, got:\n%s", s)
	}
}
