# Reporting, Export & Themes

## Reports

```
report [path] [--format md|pdf] [-o <path>] [--no-ai]
```

Generates an incident report from the current case (case summary, notes,
evidence, timeline, tool runs, IOCs).

- **Default output**: `<case>/output/report.md`.
- **`--format pdf`**: converts via the `pandoc/core` Docker image. If PDF is
  unavailable (no Docker, pandoc failure), Mimir degrades to Markdown and warns.
- **`--no-ai`**: skip the AI-generated summary. Otherwise the report embeds the
  AI narrative (the same one `ai summarize` produces); AI failures produce a
  warning and the report is still written.
- Every report appends a `report_generated` timeline event (path + format).

Example:

```
report                       # case/output/report.md
report --format pdf          # case/output/report.pdf (degrades to .md)
report -o ~/incident-42.md --no-ai
```

## Export & Import

Portable, self-contained case archives.

| Command | Description |
|---|---|
| `export [path] [--json] [--no-output] [--provenance]` | Bundle the current case |
| `import <archive.tar.gz> [--as <name>]` | Restore a case archive |

### `export`

- **Default**: writes `<name>.tar.gz` containing the case directory.
- **`--json`**: writes `<name>.json` (JSON export) instead of a tarball.
- **`--no-output`**: exclude the `output/` directory.
- **`--provenance`**: include a full provenance manifest (file hashes, tool
  runs, indicators) alongside the archive.

Example:

```
export ~/cases/incident-42.tar.gz --provenance
export --json
export --no-output
```

### `import`

Restores an exported archive into `~/.mimir/investigations/`. `--as <name>`
renames the case on import.

```
import ~/cases/incident-42.tar.gz
import ~/cases/incident-42.tar.gz --as incident-43
```

## Dashboard

```
dashboard    # or: dash
```

A full-screen interactive TUI (bubbletea, alternate screen) that explores the
installed tool set: live build/install status and per-tool run counts. Press
Enter on a tool to run it immediately. `q`/`Esc` exits back to the REPL.

## Themes

Mimir's prompt and colors are driven by a `Theme`.

| Command | Description |
|---|---|
| `theme` | List available themes |
| `theme set <name>` | Switch theme for the session |
| `theme save [path]` | Persist the current theme (default: `config.yaml`) |

Built-in themes: `default`, `kali`, `minimal`, `no-color`. Prompt style is
`inline` or `twoline`; the input marker is configurable (`❯` by default, `>` for
`minimal`). For a fully plain prompt: `theme set no-color` plus `MIMIR_ASCII=1`.

Colors may be ANSI color names (`green`, `bright_blue`) or raw escape codes
(`38;5;214m`).
