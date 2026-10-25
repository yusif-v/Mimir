# Mimir — Overview & Configuration

Mimir is a DFIR (Digital Forensics and Incident Response) shell written in Go.
It provides an interactive REPL for running investigations: cases, evidence,
IOCs, timeline, analysis tools, sandbox, reporting, and AI assistance — all in
one place.

## Building

```bash
go build -o mimir ./cmd/mimir
```

Requires Go 1.26+. Docker is optional but recommended: several analysis tools
run in containers.

## Command-line entry points

| Invocation | Description |
|---|---|
| `mimir` | Start the interactive REPL |
| `mimir config [path]` | Write a default `config.yaml` (default: `~/.mimir/config.yaml`) and exit |
| `mimir mcp` | Run the MCP server over stdio (see [MCP](mcp.md)) |
| `mimir mcp --http <addr>` | Run the MCP server over HTTP |
| `mimir version` / `--version` / `-v` | Print the version and exit |

## Directory layout

Mimir follows an FHS-like layout under `~/.mimir/`:

```
~/.mimir/
├── config.yaml          # configuration (like /etc)
├── investigations/      # case files (like /var/lib)
│   └── <case-name>/
│       ├── case.json     # case metadata + full timeline
│       ├── evidence/     # added evidence (hash-tracked)
│       ├── evidence.jsonl # evidence index (one JSON record per line)
│       ├── ioc.jsonl      # extracted indicators
│       ├── notes/         # case notes
│       └── output/        # tool output, reports, exports
├── tools/               # installed catalog tools (like /opt)
├── plugins/             # installed plugins (like /opt)
├── cache/               # transient data
├── logs/                # log file (mimir.log)
└── .history             # REPL history
```

The base directory can be overridden with `MIMIR_HOME` (or `MIMIR_CASES_PATH`
for just the case root).

## Configuration (`config.yaml`)

`mimir config` writes the defaults; edit `~/.mimir/config.yaml` directly.

| Key | Default | Description |
|---|---|---|
| `log_level` | `info` | Logging verbosity |
| `ai.provider` | `openrouter` | AI provider: `openrouter`, `anthropic`, `ollama`, or a custom name |
| `ai.base_url` | `https://openrouter.ai/api/v1` | API endpoint base URL |
| `ai.api_key` | *(empty)* | API key (or use `MIMIR_AI_API_KEY`) |
| `ai.model` | *(empty)* | Model identifier (see `ai use`) |
| `ai.max_tokens` | `4096` | Max output tokens |
| `ai.temperature` | `0.1` | Sampling temperature |
| `ai.context_window` | `50000` | Token window used for context budgeting |
| `ai.timeout` | `60` | HTTP timeout (seconds) |
| `ai.auto_analyze_evidence` | *(unset)* | When `true`, adding evidence triggers automatic AI analysis |

Secrets (the API key) are redacted by `ai config` and never logged.

## Environment variables

| Variable | Purpose |
|---|---|
| `MIMIR_HOME` | Base directory (default `~/.mimir`) |
| `MIMIR_CASES_PATH` | Override case storage root |
| `MIMIR_LOG_LEVEL` | Override `log_level` |
| `MIMIR_ASCII=1` | Force the plain ASCII prompt (no unicode glyphs) |
| `NO_COLOR=1` | Disable ANSI color output |
| `MIMIR_MCP_TOKEN` | Bearer token for the MCP server |
| `MIMIR_AI_PROVIDER` | Override AI provider |
| `MIMIR_AI_BASE_URL` | Override AI base URL |
| `MIMIR_AI_API_KEY` | Override AI API key |
| `MIMIR_AI_MODEL` | Override AI model |
| `MIMIR_AI_MAX_TOKENS` | Override max tokens |
| `MIMIR_AI_TEMPERATURE` | Override temperature |
| `MIMIR_AI_CONTEXT_WINDOW` | Override context window |
| `MIMIR_AI_TIMEOUT` | Override timeout (seconds) |

## The REPL

- **Prompt**: shows the current case, e.g. `[user][mimir][incident-42] ❯`.
  The status line is rendered separately to avoid readline corruption.
- **Completion**: Tab completes command domains, tool names (`tool run`,
  `tool install`, `tool build`, `tool info`), case names, and file paths.
- **History**: arrow keys / Ctrl+R search; persisted to `~/.mimir/.history`.
- **Ctrl+C** cancels the current input; **Ctrl+D** exits.
- **Shell passthrough**: any unknown command is executed by the system shell,
  so you can mix `ls`, `cat`, `grep`, etc. into a session.
- **Themes**: see [Reporting, export & themes](reporting.md#themes).
