# MCP Server

Mimir exposes its investigation tools over the **Model Context Protocol** so an
LLM client (Claude Desktop, Claude Code, or any MCP client) can drive a case.

## Running

```
mimir mcp                              # stdio transport (for MCP clients)
mimir mcp --http 127.0.0.1:7777        # Streamable HTTP
mimir mcp --http :7777 --token <t>     # HTTP with bearer auth
```

- `--token` overrides the `MIMIR_MCP_TOKEN` environment variable.
- For HTTP on a **non-loopback** interface, set a token. Loopback addresses
  work without one.
- The client authenticates with `Authorization: Bearer <token>`.

## Tool surface

The server registers the same subsystems the REPL exposes. Every tool takes an
explicit case name (the server is stateless between calls).

| Tool | Description |
|---|---|
| `cases_list` | List cases (optional status filter) |
| `case_create` | Create a case (name, optional template) |
| `case_status` | Case summary (evidence, note, event, IOC counts) |
| `case_close` | Close a case |
| `timeline_read` | Read a case timeline |
| `timeline_correlate` | Cross-tool correlation |
| `note_add` | Add a note |
| `evidence_list` / `evidence_add` / `evidence_tag` / `evidence_verify` | Evidence management |
| `ioc_extract` / `ioc_diff` | Indicator extraction & diffing |
| `correlate` | Cross-domain IOC correlation |
| `tools_list` | List available tools |
| `tool_run` | Run a tool on a case (captures output + IOCs) |
| `tool_build` / `tool_install` | Build / install catalog tools |
| `report` / `export` | Produce reports and archives |
| `search` | Search notes and evidence across cases |

Tools that mutate state (e.g. `tool_build`, `tool_install`) carry destructive
annotations so clients surface confirmation; read-only tools are marked
`ReadOnlyHint`.

## Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "mimir": {
      "command": "/path/to/mimir",
      "args": ["mcp"]
    }
  }
}
```

## Implementation

The server uses the `modelcontextprotocol/go-sdk`. Server identity: name
`mimir`, version = `shell.Version`. Case access is serialized behind a mutex, so
concurrent tool calls never interleave writes to a case.
