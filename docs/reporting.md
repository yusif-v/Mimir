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

A full-screen interactive TUI (bubbletea, alternate screen) with a single,
information-dense tool explorer. It merges all three tool sources into one
list — native builtins, installed tools, and installable catalog entries — so
the dashboard is populated even on a fresh install. Each row is labelled with
its source (`[builtin]`, `[installed]`, `[available]`), plus a status glyph,
name, category, description and status/run count.

- **Width-bounded rows**: long descriptions wrap onto continuation lines
  indented to the description column, and no line exceeds the terminal width.
- **Theme-aware colours**: the explorer reads colours from the active theme, so
  `theme set` changes the dashboard's appearance.
- **Search**: `/` opens an input that filters by name, description, category and
  tags, case-insensitively; `Esc` clears it and restores the full list.
- **Inline detail**: `Tab` (or `d`) expands the selected row in place to show the
  full description, tags, source, status, run count and category; `Tab` again
  collapses it.
- **Measured viewport**: the scroll window is computed from rendered row heights
  rather than a fixed row count, so the header stays pinned and the last visible
  row is never clipped.
- **Live statuses**: statuses refresh on start and with `r`; while a refresh is
  in flight the footer reads `refreshing…` and stale status glyphs are replaced
  by a spinner.

Keys: `j`/`k` or `↑`/`↓` move; `1`/`2`/`3` filter all / ready / needs
attention; `c` cycles category; `/` searches; `Esc` clears the search (or quits
when it is already empty); `Tab`/`d` expands; `r` refreshes; `Enter` runs the
selected tool; `q` or `Ctrl+C` quits back to the REPL.

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
