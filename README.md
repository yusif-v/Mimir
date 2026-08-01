# Mimir

A DFIR investigation shell for incident response, written in Go.

Mimir is a REPL that puts a full investigation toolkit at your fingertips. Open a
case, add evidence, run analysis tools (native Go builtins, Docker-backed
forensics images, or arbitrary local binaries), watch IOCs get extracted and
correlated automatically, and produce an incident report — with everything
recorded on a searchable, exportable timeline.

## Quick Start

```bash
go build -o mimir ./cmd/mimir
./mimir
```

A five-minute investigation:

```
[user][mimir] ❯ case -n incident-42                    # create a case
[user][mimir] ❯ case -o incident-42                    # open it
[user][mimir][incident-42] ❯ evidence add sample.bin --tag malware,packed
[user][mimir][incident-42] ❯ run hash evidence/sample.bin
[user][mimir][incident-42] ❯ ioc --from-output hash
[user][mimir][incident-42] ❯ note suspicious packed binary
[user][mimir][incident-42] ❯ timeline
[user][mimir][incident-42] ❯ report
```

## Features

- **REPL with readline** — history, tab completion (commands, tools, cases,
  files), Ctrl+R search, and shell passthrough for anything else.
- **Cases** — create/open/close cases; every action is recorded on an immutable
  timeline with notes.
- **Evidence & IOCs** — hash-tracked evidence with tags, automatic IOC
  extraction from tool output, and cross-domain correlation.
- **Three tool kinds** — native Go builtins, Docker-backed catalog tools, and
  local binaries, all run through `run <tool>`.
- **First-class analysis** — `volatility` (memory), `sleuthkit` (disk), and
  `regripper` (Windows registry) as dedicated commands.
- **Sandbox** — disposable Docker containers for detonating and analyzing files.
- **AI assistant** — optional LLM integration for case analysis, next-step
  suggestions, and report narratives (OpenRouter, Anthropic, Ollama, or any
  OpenAI-compatible endpoint).
- **MCP server** — expose the investigation to Claude Desktop / Claude Code.
- **Portable cases** — export to `tar.gz` (with provenance manifest) or JSON,
  import anywhere.

## Command Reference

| Command | Description |
|---|---|
| `help` | List commands |
| `status` | Show the current case status |
| `case` | `-n <name> [-t <template>]` create, `-o <name>` open, `-c` close, `templates` |
| `cases` | List cases (`--status open\|closed`) |
| `tools` | List built-in, installed, and installable tools |
| `run <tool> [args...]` | Run a tool; output captured to the open case |
| `install <name>` / `build <name>` | Install a catalog tool / build its Docker image |
| `note <text>` | Add a note to the current case |
| `evidence` / `ev` | `add`, `tag`, `verify`, `classify` |
| `timeline` | Case timeline (`--type`, `--grep`, `-n`, `export`, `correlate`, `build`) |
| `ioc` | Extract from a file or tool output, `diff` sets, list tracked |
| `correlate [--dot]` | Cross-domain (memory/disk/network) IOC graph |
| `search` / `grep` | Search cases, notes, and tool output |
| `export` / `import` | Bundle / restore case archives (`tar.gz` or JSON) |
| `ai` | `ask`, `analyze`, `suggest`, `explain`, `summarize`, `config`, `use` |
| `plugin` | `list`, `run <name>`, `info <name>` |
| `theme` | `list`, `set <name>`, `save [path]` |
| `sandbox` | `create`, `run`, `shell`, `logs`, `report`, `cp`, `destroy`, `list` |
| `sleuthkit <tool>` | Disk forensics: fls, fsstat, icat, mmls, istat, ... |
| `regripper <hive>` | Windows registry hive analysis (`-p` plugin, `--json`) |
| `volatility <plugin>` / `mem` | Memory forensics (Volatility 3) |
| `report` | Generate an incident report (`md` or `pdf`) |
| `dashboard` / `dash` | Interactive TUI tool explorer |
| `clear` / `exit` / `quit` | Utility |

## Documentation

Full documentation lives in [`docs/`](docs/):

- [Overview & configuration](docs/overview.md)
- [Cases & notes](docs/cases.md)
- [Evidence, IOCs & timeline](docs/evidence.md)
- [Tools: builtins & catalog](docs/tools.md)
- [Memory, disk & registry analysis](docs/analysis.md)
- [Sandbox](docs/sandbox.md)
- [AI assistant](docs/ai.md)
- [Reporting, export & themes](docs/reporting.md)
- [MCP server](docs/mcp.md)
- [Plugins](docs/plugins.md)
