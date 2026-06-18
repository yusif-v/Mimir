# Mimir

DFIR shell and tool multiplexer. Written in Go.

## Quick Start

```bash
go build -o mimir ./cmd/mimir
./mimir
```

Open a case, run a tool against evidence, and review the timeline:

```
[user][mimir] |> case -n incident-42      # create a case
[user][mimir] |> case -o incident-42      # open it
[user][mimir][incident-42] |> run hash evidence/sample.bin
[user][mimir][incident-42] |> note suspicious packed binary
[user][mimir][incident-42] |> timeline
```

## REPL

The interactive shell has full line editing via readline:

- **Arrow keys** — ↑/↓ walk persistent command history (saved to `~/.mimir/.history`), ←/→ move the cursor
- **Tab** — context-aware completion: command names, tool names (for `run`/`use`), installable tools (for `install`), case names (for `case -o`/`-c`), and file paths
- **Ctrl+R** — reverse history search
- **Ctrl+C** — cancel the current line (does not exit); **Ctrl+D** — exit
- Any unrecognized command is passed through to the system shell

## Commands

| Command | Description |
|---------|-------------|
| `help` | List commands |
| `status` | Show the current case status |
| `case -n <name>` | Create a new case |
| `case -o <name>` | Open a case |
| `case -c` | Close the current case |
| `cases` | List all cases |
| `tools` | List installed, built-in, and installable tools with status |
| `run <tool> [args...]` | Run a tool; output is captured to the open case |
| `install <name>` | Install a tool template from the embedded catalog |
| `build <name>` | Build a Docker-backed tool's image |
| `use <tool>` | Show details for a tool |
| `note <text>` | Add a note to the open case |
| `timeline [-n N]` | Show the case timeline (tool runs + notes), optionally the last N events |
| `clear` | Clear the screen |
| `exit` / `quit` | Exit Mimir |

## Tools

Mimir runs three kinds of tools, all through `run <tool>`:

- **Built-in** — native Go, always available, no dependencies: `hash` (MD5/SHA1/SHA256), `strings`, `file`
- **Docker** — sandboxed tools from the embedded catalog (e.g. `volatility`, `yara`, `bulk_extractor`); `install <name>` adds the template, `build <name>` builds the image
- **Local** — tools found on `PATH` via a `mimir.toml` template

When a case is open, tool output is saved under the case's `output/` directory and recorded as a `tool_run` event on the timeline.

## Config

Mimir stores everything under `~/.mimir/`:

```
~/.mimir/config.yaml       # Config file
~/.mimir/investigations/   # Cases
~/.mimir/tools/            # Tool templates
~/.mimir/plugins/          # Plugins
~/.mimir/cache/            # Cache
~/.mimir/logs/             # Logs
~/.mimir/logs/mimir.log    # Main log file
~/.mimir/.history          # Command history
```

Generate a default config:

```bash
./mimir config
# or specify a path:
./mimir config ~/.mimir/config.yaml
```

### Config File (`~/.mimir/config.yaml`)

```yaml
log_level: INFO
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `MIMIR_HOME` | Override base directory (default: `~/.mimir`) |
| `MIMIR_CASES_PATH` | Override cases directory |
| `MIMIR_LOG_LEVEL` | Log level (DEBUG, INFO, WARN, ERROR) |

## License

MIT
