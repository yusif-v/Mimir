package shell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/tools"
)

// captureJSONOutput runs fn with os.Stdout redirected and returns what was
// written. It is used to assert that --json commands emit one clean document.
func captureJSONOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String(), runErr
}

func decodeJSON(t *testing.T, out string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), v); err != nil {
		t.Fatalf("invalid JSON: %v\noutput:\n%s", err, out)
	}
}

func TestJSONCaseListAndStatus(t *testing.T) {
	app := newTestApp(t)
	if err := app.dispatch("case create alpha"); err != nil {
		t.Fatalf("case create: %v", err)
	}
	if err := app.dispatch("case open alpha"); err != nil {
		t.Fatalf("case open: %v", err)
	}

	out, err := captureJSONOutput(t, func() error {
		return app.dispatch("case list --json")
	})
	if err != nil {
		t.Fatalf("case list --json: %v", err)
	}
	var list []caseSummaryJSON
	decodeJSON(t, out, &list)
	if len(list) != 1 || list[0].Name != "alpha" || list[0].Status != "open" {
		t.Fatalf("unexpected case list JSON: %s", out)
	}
	if list[0].ToolsUsed == nil {
		t.Errorf("tools_used must be [] not null: %s", out)
	}

	out, err = captureJSONOutput(t, func() error {
		return app.dispatch("case status --json")
	})
	if err != nil {
		t.Fatalf("case status --json: %v", err)
	}
	var status caseStatusJSON
	decodeJSON(t, out, &status)
	if !status.Open || status.Name != "alpha" || status.Status != "open" {
		t.Fatalf("unexpected case status JSON: %s", out)
	}
}

func TestJSONCaseTemplates(t *testing.T) {
	app := newTestApp(t)
	out, err := captureJSONOutput(t, func() error {
		return app.dispatch("case templates --json")
	})
	if err != nil {
		t.Fatalf("case templates --json: %v", err)
	}
	var templates []caseTemplateJSON
	decodeJSON(t, out, &templates)
	if len(templates) == 0 {
		t.Fatalf("expected built-in templates, got %s", out)
	}
	if templates[0].Name == "" {
		t.Fatalf("template name missing: %s", out)
	}
}

func TestJSONEvidenceAddAndList(t *testing.T) {
	app, _ := newTestAppWithCase(t)
	src := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(src, []byte("json evidence payload"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := captureJSONOutput(t, func() error {
		return app.dispatch(fmt.Sprintf("evidence add %s --tag malware --json", src))
	})
	if err != nil {
		t.Fatalf("evidence add --json: %v", err)
	}
	var added evidenceJSON
	decodeJSON(t, out, &added)
	if added.Name != "sample.bin" || added.SHA256 == "" || added.Size == 0 {
		t.Fatalf("unexpected evidence add JSON: %s", out)
	}
	if len(added.Tags) != 1 || added.Tags[0] != "malware" {
		t.Fatalf("tags not encoded in JSON: %s", out)
	}

	out, err = captureJSONOutput(t, func() error {
		return app.dispatch("evidence list --json")
	})
	if err != nil {
		t.Fatalf("evidence list --json: %v", err)
	}
	var listed []evidenceJSON
	decodeJSON(t, out, &listed)
	if len(listed) != 1 || listed[0].Name != "sample.bin" || listed[0].SHA256 != added.SHA256 {
		t.Fatalf("unexpected evidence list JSON: %s", out)
	}
}

func TestJSONIOCAndTimelineLists(t *testing.T) {
	app, c := newTestAppWithCase(t)
	if err := c.AppendIOC(cases.IOCRecord{Type: "ip", Value: "9.9.9.9", Source: "test", Time: "2026-10-26T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := c.AppendEvent(cases.TimelineEvent{Type: "note", Timestamp: "2026-10-26T00:00:00Z", Payload: map[string]any{"content": "json"}}); err != nil {
		t.Fatal(err)
	}

	out, err := captureJSONOutput(t, func() error {
		return app.dispatch("ioc list --json")
	})
	if err != nil {
		t.Fatalf("ioc list --json: %v", err)
	}
	var iocs []iocJSON
	decodeJSON(t, out, &iocs)
	if len(iocs) != 1 || iocs[0].Value != "9.9.9.9" || iocs[0].Type != "ip" {
		t.Fatalf("unexpected ioc list JSON: %s", out)
	}

	out, err = captureJSONOutput(t, func() error {
		return app.dispatch("timeline list --type note --json")
	})
	if err != nil {
		t.Fatalf("timeline list --json: %v", err)
	}
	var events []cases.TimelineEvent
	decodeJSON(t, out, &events)
	if len(events) != 1 || events[0].Type != "note" {
		t.Fatalf("unexpected timeline list JSON: %s", out)
	}
}

func TestJSONToolListAndInfo(t *testing.T) {
	app := newTestApp(t)
	app.Tools.Register(&tools.Definition{
		Name:        "fake",
		Description: "fake tool",
		Category:    "test",
		LocalCmd:    "echo",
		Tags:        []string{"test"},
	})

	out, err := captureJSONOutput(t, func() error {
		return app.dispatch("tool list --json")
	})
	if err != nil {
		t.Fatalf("tool list --json: %v", err)
	}
	var list toolListJSON
	decodeJSON(t, out, &list)
	if len(list.Installed) != 1 || list.Installed[0].Name != "fake" {
		t.Fatalf("unexpected installed tools JSON: %s", out)
	}
	if len(list.Builtin) == 0 {
		t.Fatalf("expected built-in tools in JSON: %s", out)
	}
	if list.Installed[0].Status == "" {
		t.Fatalf("tool status missing: %s", out)
	}

	out, err = captureJSONOutput(t, func() error {
		return app.dispatch("tool info fake --json")
	})
	if err != nil {
		t.Fatalf("tool info --json: %v", err)
	}
	var info toolInfoJSON
	decodeJSON(t, out, &info)
	if info.Name != "fake" || info.Description != "fake tool" || info.Mode != "local" {
		t.Fatalf("unexpected tool info JSON: %s", out)
	}

	out, err = captureJSONOutput(t, func() error {
		return app.dispatch("tool info hash --json")
	})
	if err != nil {
		t.Fatalf("tool info builtin --json: %v", err)
	}
	var builtinInfo toolInfoJSON
	decodeJSON(t, out, &builtinInfo)
	if builtinInfo.Name != "hash" || builtinInfo.Source != "builtin" || builtinInfo.Mode != "builtin" {
		t.Fatalf("unexpected builtin tool info JSON: %s", out)
	}
}

func TestJSONThemeAndSandboxLists(t *testing.T) {
	app := newTestApp(t)

	out, err := captureJSONOutput(t, func() error {
		return app.dispatch("theme list --json")
	})
	if err != nil {
		t.Fatalf("theme list --json: %v", err)
	}
	var themes []themeJSON
	decodeJSON(t, out, &themes)
	if len(themes) == 0 {
		t.Fatalf("expected built-in themes, got %s", out)
	}

	out, err = captureJSONOutput(t, func() error {
		return app.dispatch("sandbox list --json")
	})
	if err != nil {
		t.Fatalf("sandbox list --json: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("expected an empty JSON array for no sandboxes, got %s", out)
	}
}

func TestJSONOnNoDataCommandErrorsClearly(t *testing.T) {
	app := newTestApp(t)
	err := app.dispatch("case close --json")
	if err == nil {
		t.Fatal("expected case close --json to error")
	}
	if !strings.Contains(err.Error(), "--json is not supported for case close") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJSONOutputIsSingleDocument(t *testing.T) {
	app, _ := newTestAppWithCase(t)
	src := filepath.Join(t.TempDir(), "one.bin")
	if err := os.WriteFile(src, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := captureJSONOutput(t, func() error {
		return app.dispatch(fmt.Sprintf("evidence add %s --json", src))
	})
	if err != nil {
		t.Fatalf("evidence add --json: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("first document invalid: %v\n%s", err, out)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("expected exactly one JSON document, got trailing data: %v\n%s", err, out)
	}
}
