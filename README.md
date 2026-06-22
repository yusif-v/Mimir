# Mimir

DFIR shell and tool multiplexer. Written in Go.

## Quick Start

```bash
go build -o mimir ./cmd/mimir
./mimir
```

Open a case, run a tool against evidence, and review the timeline:

```
[user][mimir] ❯ case -n incident-42      # create a case
[user][mimir] ❯ case -o incident-42      # open it
[user][mimir][incident-42] ❯ run hash evidence/sample.bin
[user][mimir][incident-42] ❯ evidence add evidence/sample.bin --tag malware,packed
[user][mimir][incident-42] ❯ note suspicious packed binary
[user][mimir][incident-42] ❯ timeline
```

## REPL

The interactive shell has full line editing via readline:

- **Arrow keys** — ↑/↓ walk persistent command history (saved to `~/.mimir/.history`), ←/→ move the cursor
- **Tab** — context-aware completion: command names, tool names (for `run`/`use`), installable tools (for `install`), case names (for `case -o`/`-c`), and file paths
- **Ctrl+R** — reverse history search
- **Ctrl+C** — cancel the current line (does not exit); **Ctrl+D** — exit
- Any unrecognized command is passed through to the system shell

### Prompt

The prompt shows a context line (`user | mimir | case`) above the `❯` input line (Starship-style). Set `MIMIR_ASCII=1` or `NO_COLOR=1` to switch to a plain ASCII prompt with no icons or color.

## Commands

| Command | Description |
|---------|-------------|
| `help` | List commands |
| `status` | Show the current case status |
| `case -n <name> [-t <template>]` | Create a new case (optionally from a template) |
| `case -o <name>` | Open a case |
| `case -c` | Close the current case |
| `case templates` | List available case templates |
| `cases [--status open\|closed]` | List all cases (filter by status) |
| `tools` | List installed, built-in, and installable tools with status |
| `run <tool> [args...]` | Run a tool; output is captured to the open case |
| `install <name>` | Install a tool template from the embedded catalog |
| `build <name>` | Build a Docker-backed tool's image |
| `use <tool>` | Show details for a tool |
| `note <text>` | Add a note to the open case |
| `timeline [-n N] [--type <event-type>] [--grep <pattern>] [export [path] [--format csv|json]]` | Show the case timeline; filter by event type or text; export to CSV or JSON |
| `evidence add <path> [--tag a,b]` | Add a file to evidence (copies + SHA-256) |
| `evidence tag <name> <tag>...` | Tag an evidence item |
| `evidence verify [<name>]` | Verify evidence SHA-256 hashes |
| `evidence` / `ev` | List evidence for the current case |
| `ioc <file>` | Extract IOCs (IPs, domains, hashes, URLs) from a file |
| `ioc --from-output <name>` | Extract IOCs from a captured tool-output file |
| `ioc` | List tracked IOCs for the current case |
| `search <query> [--in-output]` | Search across all cases (names, notes, paths); `--in-output` searches within captured tool output |
| `grep <query>` | Search within captured tool output (alias for `search --in-output`) |
| `export [path] [--no-output] [--json]` | Export the current case to a tar.gz archive |
| `import <archive>` | Import a case archive (auto-renames on conflict) |
| `plugin list` | List discovered plugins |
| `plugin run <name>` | Run a plugin |
| `plugin info <name>` | Show plugin details |
| `clear` | Clear the screen |
| `exit` / `quit` | Exit Mimir |

## Tools

Mimir runs three kinds of tools, all through `run <tool>`:

- **Built-in** — native Go, always available, no dependencies: `hash` (MD5/SHA1/SHA256), `strings`, `file`, `hexdump`, `entropy`, `decode`, `peinfo`, `elfinfo`, `rtfscan`, `lnkparse`, `mimetype`
- **Docker** — sandboxed tools from the embedded catalog (e.g. `volatility`, `yara`, `bulk_extractor`); `install <name>` adds the template, `build <name>` builds the image
- **Local** — tools found on `PATH` via a `mimir.toml` template

When a case is open, tool output is saved under the case's `output/` directory and recorded as a `tool_run` event on the timeline.

### Built-in tool quick reference

| Tool | Usage |
|------|-------|
| `hash` | `run hash <file>` — MD5 / SHA1 / SHA256 |
| `strings` | `run strings <file>` — printable strings |
| `file` | `run file <file>` — magic-byte identification |
| `hexdump` | `run hexdump <file>` — canonical hex + ASCII dump |
| `entropy` | `run entropy <file>` — Shannon entropy (flags packed/encrypted regions) |
| `decode` | `run decode [--base64\|--hex\|--url] <input>` — decode encoded data (auto-detect if no flag) |
| `peinfo` | `run peinfo <file>` — PE headers, imports, sections, compile timestamp |
| `elfinfo` | `run elfinfo <file>` — ELF headers, sections, symbols |
| `rtfscan` | `run rtfscan <file>` — detect RTF exploit objects (objdata, objclass, DDE, hex blobs) |
| `lnkparse` | `run lnkparse <file>` — parse Windows LNK shortcut files |
| `mimetype` | `run mimetype <file>` — deep MIME type detection with format-specific inspection |

## Evidence & IOCs

**Evidence** tracks files tied to a case. Each `evidence add` copies the file under `evidence/`, records SHA-256, and appends to `evidence.jsonl`. Use `evidence verify` to re-check hashes at any time.

**IOC extraction** scans a file or prior tool output for indicators (IP addresses, domains, file hashes, URLs) and appends them to `ioc.jsonl`. All tracked IOCs survive session restarts.

```
[user][mimir][incident-42] ❯ evidence add malware.exe --tag pe,packed
[user][mimir][incident-42] ❯ run entropy malware.exe
[user][mimir][incident-42] ❯ ioc malware.exe
[user][mimir][incident-42] ❯ ioc
```

## Sharing (export / import)

`export` bundles the case directory into a tar.gz archive with a `manifest.json` (case metadata + file hashes). Use `--no-output` to omit captured tool output, `--json` to print the manifest to stdout instead of saving it.

`import` extracts an archive into `~/.mimir/investigations/`. If a case with the same name exists it is auto-renamed (`<name>-1`, `<name>-2`, …). File hashes in the manifest are verified on import.

```
[user][mimir][incident-42] ❯ export ~/Desktop/incident-42.tar.gz
[user][mimir] ❯ import ~/Desktop/incident-42.tar.gz
```

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
| `MIMIR_ASCII` | Set to any non-empty value to use a plain ASCII prompt (no icons) |
| `NO_COLOR` | Disable all ANSI color output (implies ASCII prompt) |

## License

MIT
