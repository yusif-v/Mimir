# Static Analysis Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `analyze <path> [--rules <path>]` command to Mimir that auto-detects a sample's file type, runs the applicable static analysis tools (yara/capa/oletools) against it, and folds every finding into the open case exactly like existing manual tool runs do.

**Architecture:** A new `internal/filetype` package does pure magic-byte sniffing (no external dependency). A tool-selection table in `internal/shell` maps detected kind → tool names. `cmdAnalyze` registers the sample as evidence (reusing `evidenceAdd`), runs each applicable/installed tool via the existing `a.Tools.Get` + `a.Runner.Run` path, and records each run through `captureInto` (timeline + generic IOC extraction + tool-specific hints) — the same pipeline `disk`/`volatility`/`regripper` already use. capa gets one new hint parser (`capaHints`) that turns its JSON output into structured `capability` IOCs carrying ATT&CK technique IDs.

**Tech Stack:** Go (existing Mimir stack: `internal/tools`, `internal/cases`, `internal/ui`, `internal/builtins`), Go's standard `testing` package, no new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-06-static-analysis-orchestrator-design.md`

## Global Constraints

- No new external dependencies (no libmagic/`file` shellout) — file-type detection is a pure Go magic-byte sniffer.
- Every static tool run must go through `captureInto`, not `recordRun` — IOC extraction and hints are required, not optional.
- Missing/uninstalled tools produce a warning and are skipped; they never abort the whole `analyze` run.
- Only two conditions abort `analyze` entirely: no open case, and an unreadable sample path.
- `analyze` runs no code from the sample itself (no execution/sandboxing) — static analysis only.

---

### Task 1: `internal/filetype` package

**Files:**
- Create: `internal/filetype/filetype.go`
- Test: `internal/filetype/filetype_test.go`

**Interfaces:**
- Produces: `type Kind string` with constants `PE`, `ELF`, `OLE`, `OOXML`, `Unknown`; `func Detect(path string) (Kind, error)`; `func DetectBytes(data []byte) Kind`. Task 3 imports this package and calls `filetype.Detect(path)`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/filetype/... -v`
Expected: FAIL — build fails, package `filetype` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

```go
// internal/filetype/filetype.go

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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/filetype/... -v`
Expected: PASS (all 8 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/filetype/filetype.go internal/filetype/filetype_test.go
git commit -m "feat: add filetype package for magic-byte sample classification"
```

---

### Task 2: capa ATT&CK capability hints

**Files:**
- Create: `internal/shell/capa.go`
- Test: `internal/shell/capa_test.go`
- Modify: `internal/shell/shell.go:2244` (`captureInto`) — add a capa branch

**Interfaces:**
- Consumes: nothing new (pure string→struct parsing).
- Produces: `type capaHint struct{ typ, val string }`; `func capaHints(out string) []capaHint`. Task 3's `cmdAnalyze` doesn't call this directly — it flows automatically through `captureInto` whenever `tool == "capa"`, same as `volatilityHints`/`regripperHints` do for their tools today.

capa's `-j` JSON output shape (subset actually parsed):

```json
{
  "rules": {
    "rule name here": {
      "meta": {
        "attack": [
          {"tactic": "Defense Evasion", "technique": "Process Injection", "subtechnique": "", "id": "T1055"}
        ]
      }
    },
    "rule with no ATT&CK mapping": {
      "meta": {"attack": []}
    }
  }
}
```

- [ ] **Step 1: Write the failing test**

```go
// internal/shell/capa_test.go
package shell

import "testing"

const capaFixtureJSON = `{
  "rules": {
    "inject thread into remote process": {
      "meta": {
        "attack": [
          {"tactic": "Defense Evasion", "technique": "Process Injection", "subtechnique": "", "id": "T1055"}
        ]
      }
    },
    "encode data using base64": {
      "meta": {
        "attack": [
          {"tactic": "Defense Evasion", "technique": "Obfuscated Files or Information", "subtechnique": "Standard Encoding", "id": "T1027.010"}
        ]
      }
    },
    "read file on Windows": {
      "meta": {"attack": []}
    }
  }
}`

func TestCapaHints(t *testing.T) {
	hints := capaHints(capaFixtureJSON)
	if len(hints) != 2 {
		t.Fatalf("expected 2 capability hints (rule with no ATT&CK mapping skipped), got %d: %+v", len(hints), hints)
	}
	want := map[string]bool{
		"T1055 Process Injection":                          false,
		"T1027.010 Obfuscated Files or Information: Standard Encoding": false,
	}
	for _, h := range hints {
		if h.typ != "capability" {
			t.Errorf("expected typ=capability, got %q", h.typ)
		}
		if _, ok := want[h.val]; !ok {
			t.Errorf("unexpected hint value: %q", h.val)
		}
		want[h.val] = true
	}
	for val, seen := range want {
		if !seen {
			t.Errorf("missing expected hint: %q", val)
		}
	}
}

func TestCapaHintsEmptyOrInvalid(t *testing.T) {
	if hints := capaHints(""); len(hints) != 0 {
		t.Errorf("empty input: expected no hints, got %+v", hints)
	}
	if hints := capaHints("not json"); len(hints) != 0 {
		t.Errorf("invalid json: expected no hints, got %+v", hints)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shell/... -run TestCapaHints -v`
Expected: FAIL — `capaHints` undefined.

- [ ] **Step 3: Write the implementation**

```go
// internal/shell/capa.go

package shell

import "encoding/json"

// capaHint is an ATT&CK-mapped capability extracted from capa's JSON output.
type capaHint struct{ typ, val string }

type capaAttack struct {
	Technique    string `json:"technique"`
	Subtechnique string `json:"subtechnique"`
	ID           string `json:"id"`
}

type capaRuleMeta struct {
	Attack []capaAttack `json:"attack"`
}

type capaRule struct {
	Meta capaRuleMeta `json:"meta"`
}

type capaOutput struct {
	Rules map[string]capaRule `json:"rules"`
}

// capaHints parses capa's `-j` JSON output into one hint per ATT&CK-mapped
// rule match. Rules with no ATT&CK mapping (an empty "attack" list — capa
// does not map every rule to a technique) are skipped. Malformed/empty input
// yields no hints rather than an error, matching the other *Hints parsers'
// best-effort style (regripperHints, volatilityHints).
func capaHints(out string) []capaHint {
	var parsed capaOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return nil
	}
	var hints []capaHint
	for _, rule := range parsed.Rules {
		for _, a := range rule.Meta.Attack {
			val := a.ID + " " + a.Technique
			if a.Subtechnique != "" {
				val += ": " + a.Subtechnique
			}
			hints = append(hints, capaHint{typ: "capability", val: val})
		}
	}
	return hints
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/shell/... -run TestCapaHints -v`
Expected: PASS

- [ ] **Step 5: Wire capa into `captureInto`**

In `internal/shell/shell.go`, find the existing volatility-hints branch inside `captureInto` (around line 2280, right after the generic `ioc.Extract` loop):

```go
	// Volatility plugin-aware hints (commands / NTLM hashes).
	if strings.Contains(strings.ToLower(command), "vol ") || strings.ToLower(tool) == "volatility" {
```

Add a new branch immediately before it (order doesn't matter, tool names are mutually exclusive):

```go
	// capa ATT&CK-mapped capability hints.
	if tool == "capa" {
		for _, h := range capaHints(out) {
			if err := c.AppendIOC(cases.IOCRecord{
				Type:   h.typ,
				Value:  h.val,
				Source: tool + ":" + command + dumpSuffix(dump),
				Time:   now,
			}); err != nil {
				return fmt.Errorf("append capa hint: %w", err)
			}
		}
	}

```

- [ ] **Step 6: Write a test proving `captureInto` records capa hints**

Add to `internal/shell/shell_internal_test.go`, next to the existing `TestCaptureIntoVolatilityHints`:

```go
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
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/shell/... -run 'TestCapa' -v`
Expected: PASS (`TestCapaHints`, `TestCapaHintsEmptyOrInvalid`, `TestCaptureIntoCapaHints`)

- [ ] **Step 8: Run the full shell package test suite to confirm no regressions**

Run: `go test ./internal/shell/...`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/shell/capa.go internal/shell/capa_test.go internal/shell/shell.go internal/shell/shell_internal_test.go
git commit -m "feat: parse capa ATT&CK capability hints into case IOCs"
```

---

### Task 3: `analyze` command

**Files:**
- Create: `internal/shell/analyze.go`
- Test: `internal/shell/analyze_test.go`
- Modify: `internal/shell/shell.go` — dispatch switch (`~line 291`, alongside `case "run":`), `cmdHelp` (`~line 366`, alongside the `report`/`volatility` lines)
- Modify: `internal/shell/completer.go` — `commandNames` (`line 39-43`)
- Modify: `docs/analysis.md` — new `## Static Analysis (analyze)` section

**Interfaces:**
- Consumes: `filetype.Detect` / `filetype.Kind` (Task 1); `capaHints` (Task 2, via `captureInto`); `captureInto` (existing, now capa-aware per Task 2); `a.evidenceAdd(c, args []string) error` (existing, `internal/shell/shell.go:1932`); `hashFile(path string) (string, error)` (existing, `internal/shell/shell.go:2170`); `c.FindEvidenceByHash(sha256 string) *cases.Evidence` (existing); `a.Tools.Get(name string) (*tools.Definition, bool)`; `builtins.Has(name string) bool` / `a.Runner.RunBuiltin(name string, args []string) *tools.Result`; `a.Runner.Run(tool *tools.Definition, args []string, casePath string) *tools.Result`; `ui.Table{Headers []string, Align []ui.Align}` / `tbl.Render(w io.Writer, width int, plain bool)`.
- Produces: `func (a *App) cmdAnalyze(args []string) error` — the only symbol later tasks (docs, dispatch, completer) need to reference.

- [ ] **Step 1: Write the failing tests**

```go
// internal/shell/analyze_test.go
package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdAnalyzeRequiresOpenCase(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.exe")
	if err := os.WriteFile(sample, []byte("MZ"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdAnalyze([]string{sample}); err == nil {
		t.Error("cmdAnalyze with no open case: expected error")
	}
}

func TestCmdAnalyzeRequiresPath(t *testing.T) {
	app := newTestApp(t)
	if err := app.cmdAnalyze(nil); err == nil {
		t.Error("cmdAnalyze with no args: expected usage error")
	}
}

func TestCmdAnalyzeMissingSample(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.Cases.Create("t"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Cases.Open("t"); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdAnalyze([]string{"/nonexistent/nope.exe"}); err == nil {
		t.Error("cmdAnalyze with missing sample: expected error")
	}
}

// TestCmdAnalyzeSkipsUninstalledTools: none of yara/capa/oletools are
// installed in a fresh test app, so every applicable tool for a PE sample
// should be skipped with a warning, evidence should still be registered,
// and the command should still succeed overall (skip, don't abort).
func TestCmdAnalyzeSkipsUninstalledTools(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.Cases.Create("t"); err != nil {
		t.Fatal(err)
	}
	c, err := app.Cases.Open("t")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.exe")
	data := append([]byte("MZ"), make([]byte, 62)...)
	if err := os.WriteFile(sample, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := app.cmdAnalyze([]string{sample}); err != nil {
		t.Fatalf("cmdAnalyze: %v", err)
	}

	found := false
	for _, e := range c.Evidence() {
		if e.Name == "sample.exe" {
			found = true
		}
	}
	if !found {
		t.Error("expected sample.exe to be registered as evidence")
	}
}

func TestStaticToolsForKind(t *testing.T) {
	tests := []struct {
		kind string
		want []string
	}{
		{"pe", []string{"yara", "capa"}},
		{"elf", []string{"yara", "capa"}},
		{"ole", []string{"yara", "oletools"}},
		{"ooxml", []string{"yara", "oletools"}},
		{"unknown", []string{"yara"}},
	}
	for _, tc := range tests {
		got := staticToolNames(tc.kind)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("staticToolNames(%s): got %v, want %v", tc.kind, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/shell/... -run 'TestCmdAnalyze|TestStaticToolsForKind' -v`
Expected: FAIL — `cmdAnalyze`/`staticToolNames` undefined.

- [ ] **Step 3: Write the implementation**

```go
// internal/shell/analyze.go

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/filetype"
	"github.com/yusif-v/mimir/internal/ui"
)

// staticToolsFor maps a detected sample kind to the static tools that apply
// to it. yara runs against everything (a generic pattern match is always
// useful); capa only understands PE/ELF binaries; oletools only understands
// OLE/OOXML Office documents.
var staticToolsFor = map[filetype.Kind][]string{
	filetype.PE:      {"yara", "capa"},
	filetype.ELF:     {"yara", "capa"},
	filetype.OLE:     {"yara", "oletools"},
	filetype.OOXML:   {"yara", "oletools"},
	filetype.Unknown: {"yara"},
}

// staticToolNames returns the tool list for a filetype.Kind given as a raw
// string, so tests (and any future CLI surface) don't need to import
// internal/filetype just to look up the table.
func staticToolNames(kind string) []string {
	return staticToolsFor[filetype.Kind(kind)]
}

// analyzeToolResult is one row of the summary table cmdAnalyze prints at the
// end of a run.
type analyzeToolResult struct {
	tool         string
	status       string // ok | failed | skipped
	iocCount     int
	capabilities int
}

func (a *App) cmdAnalyze(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}

	var rulesPath, sample string
	for i := 0; i < len(args); i++ {
		if args[i] == "--rules" && i+1 < len(args) {
			rulesPath = args[i+1]
			i++
			continue
		}
		sample = args[i]
	}
	if sample == "" {
		return fmt.Errorf("usage: analyze <path> [--rules <path>]")
	}
	info, err := os.Stat(sample)
	if err != nil {
		return fmt.Errorf("stat %s: %w", sample, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", sample)
	}

	if rulesPath == "" {
		candidate := filepath.Join(c.Path, "rules.yar")
		if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
			rulesPath = candidate
		}
	}

	// Register the sample as case evidence before analysis. If this exact
	// content is already registered (possibly under a different name than
	// this sample's basename — evidenceAdd dedupes by hash, not name), reuse
	// its existing name rather than re-adding, so containerPath below always
	// points at the file that's actually sitting in <case>/evidence/. A
	// hash mismatch under the same name is a genuine conflict and must stay
	// fatal — do not swallow it, or analyze would silently run tools against
	// the wrong file.
	sum, err := hashFile(sample)
	if err != nil {
		return fmt.Errorf("hash sample: %w", err)
	}
	name := filepath.Base(sample)
	if existing := c.FindEvidenceByHash(sum); existing != nil {
		name = existing.Name
	} else if err := a.evidenceAdd(c, []string{sample}); err != nil {
		return fmt.Errorf("register evidence: %w", err)
	}
	containerPath := "/evidence/" + name

	kind, err := filetype.Detect(sample)
	if err != nil {
		return fmt.Errorf("detect file type: %w", err)
	}
	fmt.Printf("%s: detected %s%s%s\n", name, colorCyan, kind, colorReset)

	var results []analyzeToolResult
	for _, toolName := range staticToolsFor[kind] {
		if toolName == "yara" && rulesPath == "" {
			fmt.Fprintf(os.Stderr, "%swarning:%s no yara rules available (pass --rules or place rules.yar in the case dir) — skipping yara\n", colorYellow, colorReset)
			results = append(results, analyzeToolResult{tool: "yara", status: "skipped"})
			continue
		}

		var toolArgs []string
		switch toolName {
		case "yara":
			toolArgs = []string{rulesPath, containerPath}
		case "capa":
			toolArgs = []string{"-j", containerPath}
		case "oletools":
			toolArgs = []string{"olevba", containerPath}
		}

		out, runErr := a.runAnalyzeTool(toolName, toolArgs)
		if runErr != nil {
			if strings.Contains(runErr.Error(), "not installed") {
				fmt.Fprintf(os.Stderr, "%swarning:%s tool %q not installed — run: install %s\n", colorYellow, colorReset, toolName, toolName)
				results = append(results, analyzeToolResult{tool: toolName, status: "skipped"})
				continue
			}
			fmt.Fprintf(os.Stderr, "%swarning:%s %s failed: %v\n", colorYellow, colorReset, toolName, runErr)
			results = append(results, analyzeToolResult{tool: toolName, status: "failed"})
			continue
		}

		if toolName == "capa" {
			hints := capaHints(out)
			fmt.Printf("%s: %d capabilit%s matched\n", toolName, len(hints), pluralIES(len(hints)))
		} else {
			fmt.Print(out)
		}

		beforeIOCs := len(c.IOCs())
		command := toolName + " " + strings.Join(toolArgs, " ")
		if err := captureInto(c, toolName, command, name, out); err != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s record %s output: %v\n", colorYellow, colorReset, toolName, err)
		}
		added := len(c.IOCs()) - beforeIOCs

		res := analyzeToolResult{tool: toolName, status: "ok", iocCount: added}
		if toolName == "capa" {
			res.capabilities = added
			res.iocCount = 0
		}
		results = append(results, res)
	}

	printAnalyzeSummary(results)
	return nil
}

// runAnalyzeTool resolves toolName (builtin or installed catalog tool) and
// runs it, returning its stdout. A tool that isn't installed/registered
// returns an error containing "not installed" so callers can distinguish a
// missing tool (skip) from an execution failure (warn).
func (a *App) runAnalyzeTool(toolName string, args []string) (string, error) {
	if builtins.Has(toolName) {
		result := a.Runner.RunBuiltin(toolName, args)
		if !result.Success() {
			if result.Error != nil {
				return result.Stdout, result.Error
			}
			return result.Stdout, fmt.Errorf("exit code %d: %s", result.ReturnCode, result.Stderr)
		}
		return result.Stdout, nil
	}

	tool, ok := a.Tools.Get(toolName)
	if !ok {
		return "", fmt.Errorf("tool %q not installed", toolName)
	}
	casePath := ""
	if c := a.Cases.Current(); c != nil {
		casePath = c.Path
	}
	result := a.Runner.Run(tool, args, casePath)
	if !result.Success() {
		if result.Error != nil {
			return result.Stdout, result.Error
		}
		return result.Stdout, fmt.Errorf("exit code %d: %s", result.ReturnCode, result.Stderr)
	}
	return result.Stdout, nil
}

func printAnalyzeSummary(results []analyzeToolResult) {
	tbl := ui.Table{
		Headers: []string{"TOOL", "STATUS", "IOCS FOUND", "CAPABILITIES FOUND"},
		Align:   []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignRight, ui.AlignRight},
	}
	for _, r := range results {
		iocs, caps := "-", "-"
		if r.status == "ok" {
			iocs = fmt.Sprintf("%d", r.iocCount)
			if r.tool == "capa" {
				caps = fmt.Sprintf("%d", r.capabilities)
				iocs = "-"
			}
		}
		tbl.Rows = append(tbl.Rows, []string{r.tool, r.status, iocs, caps})
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
}

func pluralIES(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/shell/... -run 'TestCmdAnalyze|TestStaticToolsForKind' -v`
Expected: PASS. Note `TestCmdAnalyzeSkipsUninstalledTools` exercises the real "not installed" path since a fresh `newTestApp` registry has no tools installed — no Docker/mocking needed.

- [ ] **Step 5: Wire `analyze` into the REPL dispatcher**

In `internal/shell/shell.go`, add to the `dispatch` switch (next to `case "run":`):

```go
	case "analyze":
		return a.cmdAnalyze(args)
```

- [ ] **Step 6: Add `analyze` to tab completion**

In `internal/shell/completer.go`, add `"analyze"` to `commandNames` (line 39-43):

```go
var commandNames = []string{
	"help", "exit", "quit", "status", "case", "cases", "tools",
	"run", "install", "build", "use", "note", "evidence", "ev", "clear", "timeline", "ioc", "correlate", "search", "disk",
	"export", "import", "analyze",
}
```

No change needed in the `complete()` switch — `analyze` isn't special-cased, so word position 1+ falls through to the existing `default: s.filePath(word)` branch, giving free path completion for the sample argument.

- [ ] **Step 7: Add a help line**

In `internal/shell/shell.go`, `cmdHelp`, add (next to the `report`/`volatility` lines):

```go
	fmt.Printf("  %sanalyze%s    static malware analysis: analyze <path> [--rules <path>] (auto yara/capa/oletools)\n", colorGreen, colorReset)
```

- [ ] **Step 8: Run the full shell package test suite**

Run: `go test ./internal/shell/...`
Expected: PASS

- [ ] **Step 9: Document the command**

Append to `docs/analysis.md`, following the existing style of its Volatility/Disk sections:

```markdown
## Static Analysis (analyze)

```
analyze <path> [--rules <path>]
```

Auto-detects the sample's file type by magic bytes (PE, ELF, legacy OLE
Office docs, OOXML Office docs) and runs the applicable static tools:

- **PE/ELF**: `yara`, `capa`
- **OLE/OOXML** (Office documents): `yara`, `oletools` (`olevba`)
- **unknown**: `yara` only

The sample is registered as case evidence first. Each tool's output is
recorded to the timeline and scanned for IOCs exactly like a manual `run`.
capa's ATT&CK-mapped capabilities are recorded as `capability` IOCs (e.g.
`T1055 Process Injection`) instead of raw text.

`yara` needs a rules file: pass `--rules <path>`, or place `rules.yar` in the
case directory. Without either, yara is skipped with a warning.

A tool that isn't installed is skipped with a warning (`install <name>` to
add it) — `analyze` always finishes and reports what it could gather.

Example:

```
evidence add ram.bin --tag memory-dump
install capa
build capa
analyze suspicious.exe
```
```

- [ ] **Step 10: Run the full test suite one more time and build**

Run: `go build ./... && go test ./...`
Expected: PASS, no build errors.

- [ ] **Step 11: Commit**

```bash
git add internal/shell/analyze.go internal/shell/analyze_test.go internal/shell/shell.go internal/shell/completer.go docs/analysis.md
git commit -m "feat: add analyze command for static malware analysis orchestration"
```

---

## Post-implementation

- Run `graphify update .` (per project CLAUDE.md) to refresh the knowledge graph now that `internal/filetype` and `internal/shell/analyze.go`/`capa.go` exist.
- Update `CHANGELOG.md` under an `## [Unreleased]` or next-version heading per user's changelog-on-finish convention — not part of this plan's tasks since it's a repo-wide habit, not a task deliverable, but do it before calling the feature done.
