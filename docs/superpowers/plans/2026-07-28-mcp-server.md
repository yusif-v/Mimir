# Mimir MCP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `mimir mcp` subcommand that serves Mimir's DFIR capabilities as a Model Context Protocol server over stdio and Streamable HTTP.

**Architecture:** A new `internal/mcpserver` package builds an `*mcp.Server` (official Go SDK) whose typed tool handlers reuse the existing `shell.App` subsystems (`Cases`, `Tools`, `Runner`, `Output`, `ioc.Extract`) directly — no shelling out, no re-implementing command logic. Tools are fully stateless: every case-scoped tool takes an explicit `case` argument and the handler opens it under a mutex. `cmd/mimir/main.go` gains an `mcp` branch that parses flags and dispatches to stdio or Streamable HTTP transport.

**Tech Stack:** Go 1.26, `github.com/modelcontextprotocol/go-sdk/mcp`, existing Mimir internal packages.

## Global Constraints

- Module path: `github.com/yusif-v/mimir`; Go version floor `go 1.26`.
- Target release: **v0.8.0** (`shell.Version` must read `0.8.0` by the end).
- **stdio invariant:** in stdio mode, stdout carries MCP frames only. Never `fmt.Print*` to stdout from any MCP code path. Human-facing/warn output goes to `os.Stderr`.
- Reuse domain methods; do NOT call the `cmd*` REPL handlers (they print to stdout). Read each named `cmd*` in `internal/shell/shell.go` to learn the sequence of domain calls, then replicate only the domain-call portion.
- Every tool returns BOTH a typed `Out` struct AND a human-readable text summary in `CallToolResult.Content`.
- Domain failures (case not found, Docker down, etc.) are returned as `*mcp.CallToolResult{IsError: true}` with a message — NOT as a Go `error` (a returned Go error becomes a protocol error / dropped call). Reserve the returned `error` for programmer bugs only.
- Follow existing test style: table tests, temp dirs via `t.TempDir()`, no external network.
- Commit after every task with a `feat:`/`test:`/`docs:` message. Do NOT add a `Co-Authored-By` trailer (repo rule).

## File Structure

```
cmd/mimir/main.go                    # MODIFY: add `mcp` subcommand branch + handleMCPCmd()
internal/mcpserver/server.go         # NEW: NewServer(app) registers all tools; holds *App + sync.Mutex
internal/mcpserver/transport.go      # NEW: Serve(ctx, srv, Options) — stdio vs Streamable HTTP + auth
internal/mcpserver/schema.go         # NEW: shared In/Out structs + text-summary helpers
internal/mcpserver/tools_case.go     # NEW: cases_list, case_create, case_status
internal/mcpserver/tools_timeline.go # NEW: timeline_read, timeline_correlate, note_add
internal/mcpserver/tools_evidence.go # NEW: evidence_list, evidence_add, evidence_tag, evidence_verify
internal/mcpserver/tools_ioc.go      # NEW: ioc_extract, ioc_diff, correlate
internal/mcpserver/tools_exec.go     # NEW: tools_list, tool_run, tool_build, tool_install
internal/mcpserver/tools_report.go   # NEW: report, export
internal/mcpserver/*_test.go         # NEW: per-file unit tests + server_test.go integration + transport_test.go
```

**Shared test helper** (define once in `server_test.go`, reused by sibling tests in the same package): `newTestServer(t)` returns `(*mcp.ClientSession, *shell.App, func())` — builds an `App` on a `t.TempDir()` CasesPath, constructs the server, connects an in-memory client, and returns a cleanup func.

---

### Task 1: Dependency + runnable stdio server skeleton with `cases_list`

Delivers an end-to-end runnable `mimir mcp` (stdio) exposing one read-only tool, proving the wiring.

**Files:**
- Modify: `go.mod` (add SDK), `cmd/mimir/main.go`
- Create: `internal/mcpserver/server.go`, `internal/mcpserver/transport.go`, `internal/mcpserver/schema.go`, `internal/mcpserver/tools_case.go`, `internal/mcpserver/server_test.go`

**Interfaces:**
- Produces:
  - `func NewServer(app *shell.App) *Server` where `type Server struct { app *shell.App; mu sync.Mutex; srv *mcp.Server }`, and `func (s *Server) MCP() *mcp.Server`.
  - `func Serve(ctx context.Context, s *Server, opts Options) error` and `type Options struct { HTTPAddr string; Token string }` (HTTPAddr empty ⇒ stdio).
  - `func (s *Server) withCase(name string, fn func(*cases.Case) (any, error)) (any, error)` — locks `s.mu`, calls `s.app.Cases.Open(name)`, invokes `fn` with the opened case, returns its result. (Used by later tasks.)
  - `func textResult(summary string, out any) *mcp.CallToolResult` — builds a result whose `Content` is one `*mcp.TextContent{Text: summary}`. (Used by every tool.)
  - `func errResult(format string, a ...any) *mcp.CallToolResult` — returns `&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(...)}}}`.

- [ ] **Step 1: Add the SDK dependency**

Run: `go get github.com/modelcontextprotocol/go-sdk@latest && go mod tidy`
Expected: `go.mod` now requires `github.com/modelcontextprotocol/go-sdk`.

- [ ] **Step 2: Write shared helpers in `schema.go`**

```go
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// textResult wraps a human-readable summary (structured Out is returned
// separately by each typed handler).
func textResult(summary string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: summary}},
	}
}

// errResult reports a domain failure to the client as a tool error.
func errResult(format string, a ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, a...)}},
	}
}
```

- [ ] **Step 3: Write `server.go` with `NewServer`, `Server`, `withCase`, and tool registration hook**

```go
package mcpserver

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/shell"
)

// Server adapts Mimir's subsystems to the Model Context Protocol.
type Server struct {
	app *shell.App
	mu  sync.Mutex
	srv *mcp.Server
}

// NewServer builds an MCP server exposing Mimir's DFIR tools.
func NewServer(app *shell.App) *Server {
	s := &Server{
		app: app,
		srv: mcp.NewServer(&mcp.Implementation{Name: "mimir", Version: shell.Version}, nil),
	}
	s.registerCaseTools() // more register* calls added by later tasks
	return s
}

// MCP returns the underlying protocol server.
func (s *Server) MCP() *mcp.Server { return s.srv }

// withCase opens the named case under the server mutex and runs fn against it.
func (s *Server) withCase(name string, fn func(*cases.Case) (any, error)) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.app.Cases.Open(name)
	if err != nil {
		return nil, err
	}
	return fn(c)
}
```

- [ ] **Step 4: Write `tools_case.go` with `cases_list` only (for now)**

Read `internal/shell/shell.go` `cmdCases` to see how `App.Cases.List()` and status filtering work; replicate the data-gathering (not the printing).

```go
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type casesListIn struct {
	Status string `json:"status,omitempty" jsonschema:"filter by status: open or closed"`
}
type caseSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type casesListOut struct {
	Cases []caseSummary `json:"cases"`
}

func (s *Server) registerCaseTools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "cases_list",
		Description: "List all Mimir cases, optionally filtered by open/closed status.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.casesList)
}

func (s *Server) casesList(ctx context.Context, req *mcp.CallToolRequest, in casesListIn) (*mcp.CallToolResult, casesListOut, error) {
	s.mu.Lock()
	list, err := s.app.Cases.List()
	s.mu.Unlock()
	if err != nil {
		return errResult("list cases: %v", err), casesListOut{}, nil
	}
	out := casesListOut{}
	for _, c := range list {
		st := c.Status // confirm the field/getter name against internal/cases/model.go Case
		if in.Status != "" && !strings.EqualFold(st, in.Status) {
			continue
		}
		out.Cases = append(out.Cases, caseSummary{Name: c.Name, Status: st})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d case(s):\n", len(out.Cases))
	for _, c := range out.Cases {
		fmt.Fprintf(&b, "  %s [%s]\n", c.Name, c.Status)
	}
	return textResult(b.String()), out, nil
}
```

> Verify `Case.Name` / `Case.Status` field access against `internal/cases/model.go:17` (`type Case struct`). Adjust to the real field/getter names if they differ.

- [ ] **Step 5: Write `transport.go`**

```go
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures how the server is served.
type Options struct {
	HTTPAddr string // empty => stdio
	Token    string // bearer token for HTTP; see Task 7
}

// Serve runs the MCP server until the context is cancelled or the client
// disconnects. HTTP handling is added in Task 7.
func Serve(ctx context.Context, s *Server, opts Options) error {
	if opts.HTTPAddr == "" {
		return s.srv.Run(ctx, &mcp.StdioTransport{})
	}
	return serveHTTP(ctx, s, opts) // implemented in Task 7
}
```

For this task only, add a temporary `serveHTTP` stub in `transport.go` that returns `fmt.Errorf("http transport not yet implemented")` so the package compiles; Task 7 replaces it.

- [ ] **Step 6: Wire the `mcp` subcommand in `cmd/mimir/main.go`**

Add `case "mcp": handleMCPCmd(); return` to the `os.Args[1]` switch, and:

```go
func handleMCPCmd() {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	httpAddr := fs.String("http", "", "serve Streamable HTTP on this address instead of stdio")
	token := fs.String("token", os.Getenv("MIMIR_MCP_TOKEN"), "bearer token for HTTP transport")
	_ = fs.Parse(os.Args[2:])

	cfg := config.Load()
	app := shell.NewApp(cfg)
	if err := mcpserver.Serve(context.Background(), mcpserver.NewServer(app), mcpserver.Options{
		HTTPAddr: *httpAddr,
		Token:    *token,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "mcp: %v\n", err)
		os.Exit(1)
	}
}
```

Add imports: `context`, `flag`, and `github.com/yusif-v/mimir/internal/mcpserver`.

- [ ] **Step 7: Write the integration test + shared helper in `server_test.go`**

```go
package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/shell"
)

func newTestServer(t *testing.T) (*mcp.ClientSession, *shell.App) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.CasesPath = t.TempDir()
	app := shell.NewApp(cfg)
	srv := NewServer(app)

	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.MCP().Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs, app
}

func TestCasesListEmpty(t *testing.T) {
	cs, _ := newTestServer(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "cases_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
}
```

> Confirm `config.DefaultConfig()` exposes a settable `CasesPath` field (see `internal/config/config.go:22`). If the field name differs, use the real one.

- [ ] **Step 8: Run and iterate**

Run: `go build ./... && go test ./internal/mcpserver/...`
Expected: builds; `TestCasesListEmpty` passes. Fix any field-name mismatches surfaced by the compiler.

- [ ] **Step 9: Manual smoke (stdio)**

Run: `go build -o mimir ./cmd/mimir && printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' | ./mimir mcp`
Expected: a JSON-RPC response listing `cases_list` on stdout, no banner/prompt text.

- [ ] **Step 10: Commit**

```bash
git add go.mod go.sum cmd/mimir/main.go internal/mcpserver
git commit -m "feat(mcp): stdio server skeleton with cases_list tool"
```

---

### Task 2: Case create/status + timeline + notes

**Files:**
- Create: `internal/mcpserver/tools_timeline.go`, `internal/mcpserver/tools_timeline_test.go`
- Modify: `internal/mcpserver/tools_case.go` (add `case_create`, `case_status`), `internal/mcpserver/server.go` (call `registerTimelineTools()`)

**Interfaces:**
- Consumes: `Server.withCase`, `textResult`, `errResult` (Task 1).
- Produces: registers tools `case_create`, `case_status`, `timeline_read`, `timeline_correlate`, `note_add`.

- [ ] **Step 1: Write failing tests in `tools_timeline_test.go`**

```go
package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res
}

func TestCaseCreateThenNoteAppearsInTimeline(t *testing.T) {
	cs, _ := newTestServer(t)
	if r := call(t, cs, "case_create", map[string]any{"name": "inc-1"}); r.IsError {
		t.Fatalf("case_create error: %v", r.Content)
	}
	if r := call(t, cs, "note_add", map[string]any{"case": "inc-1", "text": "suspicious binary"}); r.IsError {
		t.Fatalf("note_add error: %v", r.Content)
	}
	r := call(t, cs, "timeline_read", map[string]any{"case": "inc-1", "type": "note"})
	if r.IsError {
		t.Fatalf("timeline_read error: %v", r.Content)
	}
	// StructuredContent carries the typed Out; assert at least one event.
	if r.StructuredContent == nil {
		t.Fatal("expected structured timeline output")
	}
}
```

> `CallToolResult.StructuredContent` holds the JSON of the typed `Out`. If the SDK version names it differently, inspect `res.Content` text instead and assert the summary mentions the note.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run TestCaseCreateThenNoteAppearsInTimeline -v`
Expected: FAIL (`case_create`/`note_add`/`timeline_read` not registered).

- [ ] **Step 3: Add `case_create` and `case_status` to `tools_case.go`**

Read `cmdCase` (create/status paths) and `cmdStatus` in `internal/shell/shell.go` for the domain calls (`Cases.Create`, `Cases.CreateWithTemplate`, and how status is derived).

```go
type caseCreateIn struct {
	Name     string `json:"name" jsonschema:"case name"`
	Template string `json:"template,omitempty" jsonschema:"optional case template name"`
}
type caseCreateOut struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) caseCreate(ctx context.Context, req *mcp.CallToolRequest, in caseCreateIn) (*mcp.CallToolResult, caseCreateOut, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		c   *cases.Case
		err error
	)
	if in.Template != "" {
		c, err = s.app.Cases.CreateWithTemplate(in.Name, in.Template)
	} else {
		c, err = s.app.Cases.Create(in.Name)
	}
	if err != nil {
		return errResult("create case: %v", err), caseCreateOut{}, nil
	}
	out := caseCreateOut{Name: c.Name, Path: c.Path} // confirm Path field
	return textResult(fmt.Sprintf("created case %q", c.Name)), out, nil
}
```

Add `case_status` similarly (typed `caseStatusIn{Case string}` → open via `withCase`, report name/status/counts of evidence, notes, timeline events, IOCs from the `*cases.Case` getters `Evidence()`, `Timeline()`, `IOCs()`). Register both in `registerCaseTools`, each with `ReadOnlyHint` for `case_status` and `DestructiveHint: false` (default) for `case_create`.

Add `import "github.com/yusif-v/mimir/internal/cases"` to `tools_case.go`.

- [ ] **Step 4: Write `tools_timeline.go`**

Read `cmdTimeline` (filter parsing: `--type`, `--grep`, `-n`) and `cmdNote` for domain calls (`Case.AppendEvent` / `Case.AddNote`, `Case.Timeline()`).

```go
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
)

type timelineReadIn struct {
	Case  string `json:"case" jsonschema:"case name"`
	Type  string `json:"type,omitempty" jsonschema:"filter by event type"`
	Grep  string `json:"grep,omitempty" jsonschema:"filter by substring match"`
	Limit int    `json:"limit,omitempty" jsonschema:"max events (0 = all)"`
}
type timelineEvent struct {
	Time string `json:"time"`
	Type string `json:"type"`
	Text string `json:"text"`
}
type timelineReadOut struct {
	Events []timelineEvent `json:"events"`
}

func (s *Server) registerTimelineTools() {
	mcp.AddTool(s.srv, &mcp.Tool{Name: "timeline_read",
		Description: "Read a case timeline, optionally filtered by type/substring/limit.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.timelineRead)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "timeline_correlate",
		Description: "Group a case's tool runs and summarize indicators by type (cross-tool view).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.timelineCorrelate)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "note_add",
		Description: "Append an investigator note to a case timeline."}, s.noteAdd)
}

func (s *Server) timelineRead(ctx context.Context, req *mcp.CallToolRequest, in timelineReadIn) (*mcp.CallToolResult, timelineReadOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		var evs []timelineEvent
		for _, e := range c.Timeline() { // confirm TimelineEvent field names at model.go:114
			if in.Type != "" && !strings.EqualFold(e.Type, in.Type) {
				continue
			}
			if in.Grep != "" && !strings.Contains(e.Description, in.Grep) {
				continue
			}
			evs = append(evs, timelineEvent{Time: e.Timestamp.String(), Type: e.Type, Text: e.Description})
		}
		if in.Limit > 0 && len(evs) > in.Limit {
			evs = evs[len(evs)-in.Limit:]
		}
		return evs, nil
	})
	if err != nil {
		return errResult("timeline_read: %v", err), timelineReadOut{}, nil
	}
	evs := v.([]timelineEvent)
	return textResult(fmt.Sprintf("%d event(s)", len(evs))), timelineReadOut{Events: evs}, nil
}
```

Implement `noteAdd` (typed `{Case, Text}`; inside `withCase` call `c.AddNote(in.Text, "mcp")` — confirm signature at `model.go:99`) and `timelineCorrelate` (port the grouping logic from `cmdTimeline`'s `correlate` path). Confirm `TimelineEvent` field names (`Type`, `Description`, `Timestamp`) against `internal/cases/model.go:114`.

- [ ] **Step 5: Register timeline tools in `NewServer`**

Add `s.registerTimelineTools()` after `s.registerCaseTools()` in `server.go`.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/mcpserver/ -run 'TestCaseCreate|TestCasesList' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/mcpserver
git commit -m "feat(mcp): case create/status, timeline read/correlate, note_add"
```

---

### Task 3: Evidence tools

**Files:**
- Create: `internal/mcpserver/tools_evidence.go`, `internal/mcpserver/tools_evidence_test.go`
- Modify: `internal/mcpserver/server.go` (call `registerEvidenceTools()`)

**Interfaces:**
- Consumes: `withCase`, `textResult`, `errResult`.
- Produces: registers `evidence_list`, `evidence_add`, `evidence_tag`, `evidence_verify`.

- [ ] **Step 1: Write failing test in `tools_evidence_test.go`**

```go
func TestEvidenceAddThenList(t *testing.T) {
	cs, app := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-e"})
	f := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(f, []byte("MZmalware"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := call(t, cs, "evidence_add", map[string]any{"case": "inc-e", "path": f, "tags": []any{"malware"}}); r.IsError {
		t.Fatalf("evidence_add: %v", r.Content)
	}
	r := call(t, cs, "evidence_list", map[string]any{"case": "inc-e"})
	if r.IsError {
		t.Fatalf("evidence_list: %v", r.Content)
	}
	_ = app
}
```

Add imports `os`, `path/filepath`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run TestEvidenceAddThenList -v`
Expected: FAIL (tools not registered).

- [ ] **Step 3: Write `tools_evidence.go`**

Read `cmdEvidence` in `internal/shell/shell.go:1847` for the add/tag/verify/list domain calls (copy-into-evidence + SHA-256 + `Case.AppendEvidence`, tag mutation, hash verification). Replicate the domain portion; each handler wraps `withCase`.

```go
type evidenceAddIn struct {
	Case string   `json:"case" jsonschema:"case name"`
	Path string   `json:"path" jsonschema:"path to the file to add as evidence"`
	Tags []string `json:"tags,omitempty" jsonschema:"optional tags"`
}
type evidenceItem struct {
	Name   string   `json:"name"`
	SHA256 string   `json:"sha256"`
	Tags   []string `json:"tags"`
}
type evidenceAddOut struct{ Item evidenceItem `json:"item"` }
```

Register all four in `registerEvidenceTools`: `evidence_list` and `evidence_verify` get `ReadOnlyHint: true`; `evidence_add`/`evidence_tag` are mutating (default annotations). Confirm `EvidenceRecord`/`Evidence` field names against `internal/cases/model.go:181`.

- [ ] **Step 4: Register in `NewServer`**

Add `s.registerEvidenceTools()` in `server.go`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/mcpserver/ -run TestEvidence -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcpserver
git commit -m "feat(mcp): evidence add/list/tag/verify tools"
```

---

### Task 4: IOC tools (extract, diff, correlate)

**Files:**
- Create: `internal/mcpserver/tools_ioc.go`, `internal/mcpserver/tools_ioc_test.go`
- Modify: `internal/mcpserver/server.go` (call `registerIOCTools()`)

**Interfaces:**
- Consumes: `withCase`, `textResult`, `errResult`, `ioc.Extract`.
- Produces: registers `ioc_extract`, `ioc_diff`, `correlate`.

- [ ] **Step 1: Write failing test in `tools_ioc_test.go`**

```go
func TestIOCExtractFromFile(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-ioc"})
	f := filepath.Join(t.TempDir(), "log.txt")
	os.WriteFile(f, []byte("contact 8.8.8.8 and evil.example.com"), 0o644)
	r := call(t, cs, "ioc_extract", map[string]any{"case": "inc-ioc", "path": f})
	if r.IsError {
		t.Fatalf("ioc_extract: %v", r.Content)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run TestIOCExtractFromFile -v`
Expected: FAIL.

- [ ] **Step 3: Write `tools_ioc.go`**

Read `cmdIOC` (`internal/shell/shell.go:888`, both `<file>` and `--from-output` paths), `cmdIOCDiff:955`, and `cmdCorrelate:1004`. `ioc.Extract(data []byte) []ioc.Indicator` is the core (confirm `Indicator` fields at `internal/ioc/ioc.go`). For `--from-output`, resolve the captured output file path the same way `cmdIOC` does.

```go
type iocExtractIn struct {
	Case       string `json:"case" jsonschema:"case name"`
	Path       string `json:"path,omitempty" jsonschema:"file to extract IOCs from"`
	FromOutput string `json:"from_output,omitempty" jsonschema:"captured tool-output name to extract from"`
}
type indicator struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
type iocExtractOut struct{ Indicators []indicator `json:"indicators"` }
```

`correlate` takes `{Case string, Dot bool}`; when `Dot` is true, return the Graphviz digraph string in the summary. Register with `ReadOnlyHint: true` for all three.

- [ ] **Step 4: Register in `NewServer`**

Add `s.registerIOCTools()`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/mcpserver/ -run 'TestIOC' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcpserver
git commit -m "feat(mcp): ioc extract/diff and correlate tools"
```

---

### Task 5: Execution tools (tools_list, tool_run, tool_build, tool_install)

**Files:**
- Create: `internal/mcpserver/tools_exec.go`, `internal/mcpserver/tools_exec_test.go`
- Modify: `internal/mcpserver/server.go` (call `registerExecTools()`)

**Interfaces:**
- Consumes: `withCase`, `textResult`, `errResult`, `s.app.Tools`, `s.app.Runner`, `s.app.Output`.
- Produces: registers `tools_list`, `tool_run`, `tool_build`, `tool_install`.

- [ ] **Step 1: Write failing test in `tools_exec_test.go`**

`tools_list` needs no Docker. Assert it lists at least the built-in tools.

```go
func TestToolsListIncludesBuiltins(t *testing.T) {
	cs, _ := newTestServer(t)
	r := call(t, cs, "tools_list", map[string]any{})
	if r.IsError {
		t.Fatalf("tools_list: %v", r.Content)
	}
	if r.StructuredContent == nil {
		t.Fatal("expected structured tool list")
	}
}
```

Also assert exec-tool annotations are dangerous, by listing tools and checking hints:

```go
func TestToolRunIsAnnotatedDangerous(t *testing.T) {
	cs, _ := newTestServer(t)
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tl := range tools.Tools {
		if tl.Name == "tool_run" {
			found = true
			if tl.Annotations == nil || !tl.Annotations.DestructiveHint {
				t.Error("tool_run must carry DestructiveHint")
			}
		}
	}
	if !found {
		t.Error("tool_run not registered")
	}
}
```

> Confirm the `ListTools` return shape (`*mcp.ListToolsResult` with `.Tools []*mcp.Tool`) against the SDK version; adjust field access if needed.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run 'TestTools|TestToolRun' -v`
Expected: FAIL.

- [ ] **Step 3: Write `tools_exec.go`**

Read `cmdTools:509` (list + `ResolveStatuses`), `cmdRun:577` (`Registry.Get`, `Runner.Run`/`RunBuiltin`, `Output.Record` against the open case path), `cmdBuild:1824`, `cmdInstall:1781`.

```go
type toolRunIn struct {
	Case string   `json:"case" jsonschema:"case name (output is captured here)"`
	Tool string   `json:"tool" jsonschema:"tool name"`
	Args []string `json:"args,omitempty" jsonschema:"tool arguments"`
}
type toolRunOut struct {
	Tool     string `json:"tool"`
	ExitOK   bool   `json:"exit_ok"`
	Output   string `json:"output"`
	Captured string `json:"captured_path,omitempty"`
}

func (s *Server) registerExecTools() {
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tools_list",
		Description: "List installed, built-in, and installable tools with status.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.toolsList)
	danger := &mcp.ToolAnnotations{DestructiveHint: true, OpenWorldHint: true}
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tool_run",
		Description: "Run a DFIR tool against evidence; output is captured to the case. Executes Docker/host processes.",
		Annotations: danger}, s.toolRun)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tool_build",
		Description: "Build a Docker-backed tool image.", Annotations: danger}, s.toolBuild)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "tool_install",
		Description: "Install a tool template from the embedded catalog.", Annotations: danger}, s.toolInstall)
}
```

For `tool_run`, resolve the case path via `withCase` (use `c.Path`), then `Registry.Get`/`Runner.Run` or `RunBuiltin`, then `Output.Record(tool, output, casePath)`. Map a non-success `*tools.Result` to `ExitOK:false` but still return a normal (non-`IsError`) result — a tool that ran and exited non-zero is a valid outcome the client should see; reserve `errResult` for "tool not found" / "docker unreachable".

- [ ] **Step 4: Register in `NewServer`**

Add `s.registerExecTools()`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/mcpserver/ -run 'TestTools|TestToolRun' -v`
Expected: PASS (Docker-dependent `tool_run` execution is NOT exercised here — only listing + annotations).

- [ ] **Step 6: Commit**

```bash
git add internal/mcpserver
git commit -m "feat(mcp): tools_list + gated-by-annotation exec tools"
```

---

### Task 6: Report + export tools

**Files:**
- Create: `internal/mcpserver/tools_report.go`, `internal/mcpserver/tools_report_test.go`
- Modify: `internal/mcpserver/server.go` (call `registerReportTools()`)

**Interfaces:**
- Consumes: `withCase`, `textResult`, `errResult`, `cases.BuildReport`, `Case.ExportTimeline`.
- Produces: registers `report`, `export`.

- [ ] **Step 1: Write failing test in `tools_report_test.go`**

```go
func TestReportForCase(t *testing.T) {
	cs, _ := newTestServer(t)
	call(t, cs, "case_create", map[string]any{"name": "inc-r"})
	call(t, cs, "note_add", map[string]any{"case": "inc-r", "text": "finding"})
	r := call(t, cs, "report", map[string]any{"case": "inc-r"})
	if r.IsError {
		t.Fatalf("report: %v", r.Content)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run TestReportForCase -v`
Expected: FAIL.

- [ ] **Step 3: Write `tools_report.go`**

Read `cmdReport:1634` and `cmdExport:1219`. `report` wraps `cases.BuildReport(c, summary)` (confirm signature at `internal/cases/report.go:27`) and returns the markdown in the summary + a `{Markdown string}` Out. `export` wraps `c.ExportTimeline(format, path)` (`internal/cases/export.go:15`); accept `{Case, Format ("csv"|"json"), Path, Provenance bool}`. For provenance, port the manifest logic from `cmdExport`'s `--provenance` branch. Register both `ReadOnlyHint: true` (they read case state; `export` writes only to a client-specified path — annotate `ReadOnlyHint: false` for `export` since it writes a file, `true` for `report`).

- [ ] **Step 4: Register in `NewServer`**

Add `s.registerReportTools()`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/mcpserver/ -run TestReportForCase -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcpserver
git commit -m "feat(mcp): report and export tools"
```

---

### Task 7: Streamable HTTP transport + auth hardening

**Files:**
- Modify: `internal/mcpserver/transport.go` (replace `serveHTTP` stub)
- Create: `internal/mcpserver/transport_test.go`

**Interfaces:**
- Consumes: `Options{HTTPAddr, Token}`, `s.srv`.
- Produces: `func serveHTTP(ctx context.Context, s *Server, opts Options) error`; `func requireLoopbackOrToken(addr, token string) error`.

- [ ] **Step 1: Write failing tests in `transport_test.go`**

```go
func TestRequireLoopbackOrToken(t *testing.T) {
	cases := []struct {
		addr, token string
		wantErr     bool
	}{
		{"127.0.0.1:7777", "", false},
		{"localhost:7777", "", false},
		{":7777", "", false},
		{"0.0.0.0:7777", "", true},
		{"0.0.0.0:7777", "secret", false},
		{"192.168.1.5:7777", "", true},
	}
	for _, c := range cases {
		err := requireLoopbackOrToken(c.addr, c.token)
		if (err != nil) != c.wantErr {
			t.Errorf("addr=%s token=%q: err=%v want %v", c.addr, c.token, err, c.wantErr)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mcpserver/ -run TestRequireLoopbackOrToken -v`
Expected: FAIL (`requireLoopbackOrToken` undefined).

- [ ] **Step 3: Implement `serveHTTP` + helpers**

```go
func requireLoopbackOrToken(addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --http address %q: %w", addr, err)
	}
	if host == "" || host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	if token == "" {
		return fmt.Errorf("binding non-loopback address %q requires a token (--token or MIMIR_MCP_TOKEN)", addr)
	}
	return nil
}

func serveHTTP(ctx context.Context, s *Server, opts Options) error {
	if err := requireLoopbackOrToken(opts.HTTPAddr, opts.Token); err != nil {
		return err
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.srv }, nil)
	var handler http.Handler = h
	if opts.Token != "" {
		handler = bearerAuth(h, opts.Token)
	}
	srv := &http.Server{Addr: opts.HTTPAddr, Handler: handler}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	fmt.Fprintf(os.Stderr, "mimir mcp: serving Streamable HTTP on %s\n", opts.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func bearerAuth(next http.Handler, token string) http.Handler {
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

Add imports: `net`, `net/http`, `os`, `crypto/subtle`, `fmt`, and the SDK `mcp`.

- [ ] **Step 4: Add a bearer-auth HTTP test**

```go
func TestBearerAuthRejectsMissingToken(t *testing.T) {
	h := bearerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec2.Code)
	}
}
```

Add imports `net/http/httptest`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/mcpserver/...`
Expected: PASS (all).

- [ ] **Step 6: Manual smoke (HTTP)**

Run: `./mimir mcp --http 127.0.0.1:7777` in one terminal; in another: `curl -s -XPOST 127.0.0.1:7777 -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'`
Expected: JSON-RPC tool list. Then confirm `./mimir mcp --http 0.0.0.0:7777` exits with the token error.

- [ ] **Step 7: Commit**

```bash
git add internal/mcpserver
git commit -m "feat(mcp): Streamable HTTP transport with loopback/token hardening"
```

---

### Task 8: Docs, help, version bump to 0.8.0

**Files:**
- Modify: `README.md`, `CHANGELOG.md`, `internal/shell/*` (`Version` const + `help`/command table), `cmd/mimir/main.go` (ensure `mcp` shows in usage if one exists)

**Interfaces:** none (docs + constant).

- [ ] **Step 1: Bump the version**

Find the `Version` const (`grep -rn 'Version =' internal/shell`) and set it to `"0.8.0"`.

- [ ] **Step 2: Add `mcp` to the `help` command / command table**

In `cmdHelp` (`internal/shell/shell.go:335`) add a row: `mcp` — "Run Mimir as an MCP server (stdio, or --http <addr>)". Note: `mcp` is a process-level subcommand parsed in `main.go`, not a REPL command, so word it as an external invocation (like `config`).

- [ ] **Step 3: Update `README.md`**

Add an `## MCP server` section after the Commands table:

````markdown
## MCP server

Run Mimir as a [Model Context Protocol](https://modelcontextprotocol.io) server so
Claude Desktop / Claude Code (or any MCP client) can drive an investigation.

```bash
mimir mcp                       # stdio (for local desktop clients)
mimir mcp --http 127.0.0.1:7777 # Streamable HTTP
```

Claude Desktop config:

```json
{
  "mcpServers": {
    "mimir": { "command": "mimir", "args": ["mcp"] }
  }
}
```

Networked use binds loopback by default; binding a non-loopback address
requires a token (`--token <t>` or `MIMIR_MCP_TOKEN`), sent by clients as
`Authorization: Bearer <t>`. Tools like `tool_run` execute Docker/host
processes and are marked destructive so clients prompt before running them.
````

- [ ] **Step 4: Update `CHANGELOG.md`**

Add under `## [Unreleased]` → move to a new `## [0.8.0] - 2026-07-28` section:

```markdown
## [0.8.0] - 2026-07-28

### Added
- **`mimir mcp`** — Mimir is now a Model Context Protocol server. Exposes DFIR
  operations as MCP tools over stdio and Streamable HTTP so Claude Desktop /
  Claude Code and other MCP clients can create cases, add and verify evidence,
  read and correlate the timeline, extract and diff IOCs, list and run tools,
  and generate reports/exports.
- Stateless tool model (explicit `case` argument per tool); execution tools
  (`tool_run`/`tool_build`/`tool_install`) carry destructive MCP annotations.
- Streamable HTTP transport binds loopback by default; non-loopback bind
  requires a bearer token.
```

- [ ] **Step 5: Full build + test + vet**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: all pass.

- [ ] **Step 6: Update graphify graph (repo rule)**

Run: `graphify update .`
Expected: graph refreshed for the new package (AST-only, no API cost).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "docs(mcp): README + CHANGELOG + help; bump version to 0.8.0"
```

---

## Self-Review

**Spec coverage:**
- Entry point / `mimir mcp` / flags → Task 1 (subcommand), Task 7 (`--http`/`--token`). ✓
- Headless subsystem reuse + stdio invariant → Task 1 (Step 6, Step 9 smoke). ✓
- Stateless `withCase` model → Task 1 (Step 3). ✓
- Read-only tools (cases_list, case_status, evidence_list/verify, timeline_read/correlate, ioc_extract/diff, correlate, tools_list, report, export) → Tasks 1–6. ✓
- Mutating tools (case_create, evidence_add/tag, note_add) → Tasks 2–3. ✓
- Execution tools + annotations → Task 5. ✓
- HTTP hardening (loopback default, token, 401) → Task 7. ✓
- Error-handling contract (`IsError` vs Go error) → Global Constraints + used throughout. ✓
- Testing (per-handler, in-memory integration, HTTP auth) → each task's tests + Task 1 integration + Task 7. ✓
- Docs/release (README, help, CHANGELOG, go.mod, version) → Task 8 + Task 1 Step 1. ✓

**Placeholder scan:** No TBD/TODO. Field-name verifications are called out explicitly with file:line anchors to check against, not left vague. ✓

**Type consistency:** `withCase`, `textResult`, `errResult` defined in Task 1 and consumed unchanged in Tasks 2–6. `Options{HTTPAddr, Token}` defined Task 1, implemented Task 7. Handler signature `func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)` uniform throughout. ✓

**Note on field-name verification:** Several handlers access `*cases.Case` fields/getters (`Name`, `Status`, `Path`, `Timeline()`, `Evidence()`, `IOCs()`, `AddNote`) and `TimelineEvent`/`Evidence`/`Indicator` fields. The exact names must be confirmed against `internal/cases/model.go` and `internal/ioc/ioc.go` during implementation — each such spot is flagged inline with a `>` note and file:line anchor. This is verification, not a placeholder: the domain calls are known, only the exact identifiers need confirming from source.
