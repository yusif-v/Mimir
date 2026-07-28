# Design: `mimir mcp` — DFIR MCP Server

**Date:** 2026-07-28
**Status:** Approved (design), pending spec review
**Author:** lizard

## Summary

Expose Mimir's DFIR capabilities (cases, evidence, timeline, IOCs, tool
execution) as a [Model Context Protocol](https://modelcontextprotocol.io)
server, so any MCP client — Claude Desktop, Claude Code, or a networked
client — can drive an investigation against Mimir's `~/.mimir` case store.

Mimir remains an interactive REPL; the MCP server is a second, headless
front-end onto the *same* subsystems. A new `mimir mcp` subcommand starts it.

## Goals

- Serve Mimir's DFIR operations as MCP tools over **stdio** (local desktop
  clients) and **Streamable HTTP** (networked clients).
- Reuse the existing domain code (`cases.Manager`, `tools.Registry`/`Runner`,
  `tools.OutputCapture`, `ioc.Extract`) directly — no shelling out, no
  re-implementation of command logic.
- Full tool surface, including Docker-backed tool execution, with MCP
  annotations that let well-behaved clients confirm dangerous actions.

## Non-goals

- Mimir as an MCP *client* (consuming external MCP servers). Separate future
  work.
- Multi-tenant auth / RBAC beyond a single optional bearer token for HTTP.
- Replacing or deprecating the REPL.

## Decisions (locked)

| Decision | Choice |
|----------|--------|
| MCP role | Mimir is the **server** |
| Transport | **stdio + Streamable HTTP** (`--http :port`) |
| Tool surface | **Full**, including `tool_run`/`build`/`install` |
| Protocol impl | Official **`github.com/modelcontextprotocol/go-sdk`** |
| Execution gating | **On by default** (no `--allow-exec` flag); safety via MCP annotations + HTTP network hardening |
| State model | **Fully stateless** — every case-scoped tool takes an explicit `case` argument |

## Architecture

### Entry point

`cmd/mimir/main.go` already branches on `os.Args[1]` for `config` and
`version`. Add a `mcp` case that parses MCP-specific flags and starts the
server.

```
mimir mcp                      # stdio transport
mimir mcp --http :7777         # Streamable HTTP, binds 127.0.0.1:7777
mimir mcp --http 0.0.0.0:7777 --token <t>   # non-loopback requires a token
```

Flags:
- `--http <addr>` — if set, serve Streamable HTTP on `<addr>` instead of stdio.
- `--token <t>` (or `MIMIR_MCP_TOKEN` env) — bearer token required on HTTP
  requests. Optional for loopback; **required** to bind a non-loopback address.

### Headless subsystem reuse

The server builds Mimir's subsystems via the existing `shell.NewApp(cfg)`,
which wires `Cases`, `Tools`, `Runner`, `Output`, plugins, and AI **without**
initializing readline (readline is created in `App.Run()`, which the MCP path
never calls). The MCP layer reaches into `App`'s exported fields.

> **stdio invariant:** In stdio mode, stdout carries MCP frames only. Any
> human-facing output must go to stderr. `NewApp` already writes its warnings
> to `os.Stderr`; the implementation will audit the headless path and redirect
> any stray stdout writes (banner, prompts) — none should occur because
> `Run()` is not called, but this is verified by the integration test.

### Package layout

```
internal/mcpserver/
  server.go        # NewServer(app) *mcp.Server — registers all tools
  transport.go     # Serve(ctx, srv, opts) — stdio vs Streamable HTTP dispatch
  tools_case.go    # cases_list, case_create, case_status
  tools_evidence.go# evidence_add, evidence_list, evidence_tag, evidence_verify
  tools_timeline.go# timeline_read, timeline_correlate, note_add
  tools_ioc.go     # ioc_extract, ioc_diff, correlate
  tools_exec.go    # tools_list, tool_run, tool_build, tool_install
  tools_report.go  # report, export
  schema.go        # shared typed In/Out structs
  *_test.go
```

`cmd/mimir/main.go` gains a small `handleMCPCmd()` (parallel to
`handleConfigCmd()`) that parses flags, loads config, builds the app, and calls
`mcpserver.Serve`.

## State model

Fully stateless from the client's perspective. There is **no** `case_open` /
`case_close` MCP tool and no implicit "current case". Every case-scoped tool
accepts a `case` string argument. Each handler:

1. Acquires the server's `sync.Mutex` (the underlying `cases.Manager` mutates a
   shared "current case" field internally, so calls are serialized).
2. Calls `Cases.Open(case)` to load it.
3. Delegates to the domain method.
4. Returns the structured result.

This keeps HTTP multi-client access correct without exposing the REPL's mutable
session to clients. The mutex makes throughput single-flight; acceptable for a
forensics tool (operations are coarse-grained and often long-running anyway).

## Tool surface

Each tool is a typed handler
(`func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)`)
registered with `mcp.AddTool`. `In`/`Out` structs use `json` + `jsonschema`
struct tags for auto-generated schemas. Every tool returns both a structured
`Out` and a text summary in `CallToolResult.Content` for clients that only
render text.

### Read-only (`ReadOnlyHint: true`)

| Tool | Args | Wraps |
|------|------|-------|
| `cases_list` | `status?` (open/closed) | `Cases.List` |
| `case_status` | `case` | `Cases.Open` + status assembly |
| `evidence_list` | `case` | case evidence model |
| `evidence_verify` | `case`, `name?` | evidence SHA-256 verify |
| `timeline_read` | `case`, `type?`, `grep?`, `limit?` | timeline query |
| `timeline_correlate` | `case` | `timeline correlate` logic |
| `ioc_extract` | `case`, `path?` \| `from_output?` | `ioc.Extract` |
| `ioc_diff` | `case`, `a`, `b` | `ioc diff` logic |
| `correlate` | `case`, `dot?` | cross-domain correlate |
| `tools_list` | `category?` | `Tools.List` + `ResolveStatuses` |
| `report` | `case` | `cases` report generation |
| `export` | `case`, `format?`, `provenance?` | `cases` export |

### Mutating (`DestructiveHint: false`, `IdempotentHint` where true)

| Tool | Args | Wraps |
|------|------|-------|
| `case_create` | `name`, `template?` | `Cases.Create` / `CreateWithTemplate` |
| `evidence_add` | `case`, `path`, `tags?[]` | evidence add (copy + SHA-256) |
| `evidence_tag` | `case`, `name`, `tags[]` | evidence tag |
| `note_add` | `case`, `text` | note add |

### Execution (`DestructiveHint: true`, `OpenWorldHint: true`)

Available by default. Annotations signal danger so clients confirm.

| Tool | Args | Wraps |
|------|------|-------|
| `tool_run` | `case`, `tool`, `args[]` | `Runner.Run` / `RunBuiltin` + `Output.Record` |
| `tool_build` | `tool` | Docker image build |
| `tool_install` | `name` | install from embedded catalog |

## Safety & security

- **Execution danger** is surfaced through MCP annotations, not gating: clients
  like Claude prompt the user before invoking a `DestructiveHint`/`OpenWorldHint`
  tool.
- **HTTP hardening:**
  - Default bind is `127.0.0.1`. Parsing `--http` splits host:port; a host that
    is empty or `localhost`/loopback needs no token.
  - Binding a **non-loopback** address without `--token`/`MIMIR_MCP_TOKEN` is a
    startup error.
  - When a token is set, every HTTP request must carry
    `Authorization: Bearer <token>`; mismatches get 401 before reaching the MCP
    handler (checked in an `http.Handler` wrapper around
    `mcp.NewStreamableHTTPHandler`).
- **Path handling:** `evidence_add`/`ioc_extract` `path` args are resolved and
  used exactly as the REPL does (no new path-escaping surface introduced; the
  server runs with the invoking user's privileges by design).

## SDK usage sketch

```go
srv := mcp.NewServer(&mcp.Implementation{
    Name: "mimir", Version: shell.Version,
}, nil)

mcp.AddTool(srv, &mcp.Tool{
    Name:        "timeline_read",
    Description: "Read a case timeline with optional type/text/limit filters.",
    Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
}, s.timelineRead)

// stdio:
srv.Run(ctx, &mcp.StdioTransport{})
// or HTTP:
h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
http.ListenAndServe(addr, authWrap(h, token))
```

## Error handling

- Domain errors (case not found, evidence missing, Docker unreachable) are
  returned as tool errors via `CallToolResult{IsError: true}` with a readable
  message in `Content`, **not** as Go transport errors — so the client sees a
  tool failure it can reason about, not a dropped connection.
- Transport/protocol errors propagate from the SDK.
- Startup errors (bad `--http` addr, missing token for non-loopback) print to
  stderr and exit non-zero.

## Testing

1. **Per-handler unit tests** — temp `CasesPath`, construct `App`, call handlers
   directly, assert `Out` and error paths. Mirrors existing
   `internal/cases` / `internal/tools` test style.
2. **Integration test** — SDK in-memory transport: connect a client, `tools/list`
   (assert the full expected tool set is present), then drive
   `case_create` → `evidence_add` → `note_add` → `timeline_read` end-to-end and
   assert results. Also asserts no stdout leakage in stdio setup.
3. **HTTP auth test** — non-loopback bind without token fails; request without
   bearer token gets 401; correct token succeeds.

## Docs & release

- `README.md`: new `mimir mcp` section with a copy-paste MCP client config block
  (stdio) and the `--http`/`--token` variant.
- `help` command / command table: add `mcp`.
- `CHANGELOG.md`: new version entry (e.g. `0.8.0`) describing the MCP server.
- `go.mod`: add `github.com/modelcontextprotocol/go-sdk`.

## Open questions

None. All design forks resolved during brainstorming.
