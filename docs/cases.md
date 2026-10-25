# Cases & Notes

Every investigation lives in a **case** — a directory under
`~/.mimir/investigations/` that collects evidence, tool output, notes, IOCs, and
an immutable timeline.

## Case lifecycle

| Command | Description |
|---|---|
| `case create <name>` | Create a new case |
| `case create <name> -t <template>` | Create a case from a named template |
| `case open <name>` | Open a case (start appending to its timeline) |
| `case close` | Close the current case |
| `case templates` | List available case templates |
| `case list` | List all cases |
| `case list --status open` / `--status closed` | Filter by status |
| `case status` | Show the current case (name, path, tools used, note count) |

A case must be open before evidence, notes, tools, or IOCs are recorded — the
shell shows the current case in the prompt, e.g.
`[user][mimir][incident-42] ❯`.

## Templates

Templates seed a case with directory structure, initial notes, default tags, and
recommended tools. Two ship built in:

- **`default`** — `evidence/`, `output/`, `notes/`
- **`incident-response`** — adds `evidence/{malware,memory,disk,network}` and
  `output/{volatility,yara,capa}` directories, an initial note, and recommends
  `volatility`, `yara`, `capa`, `peinfo`.

The template schema (YAML) supports:

```yaml
name: incident-response
description: Standard incident response case
directories:        # directories created under the case path
  - evidence/malware
default_tags:       # default evidence tags
  - ir
initial_notes:      # notes added as authored by "system"
  - "Case created via incident response template"
recommended_tools:  # surfaced in `ai suggest` and `tool dashboard`
  - volatility
```

## Case structure on disk

```
investigations/<name>/
├── case.json          # metadata + the full timeline (atomic writes)
├── evidence/          # added evidence files
├── evidence.jsonl     # evidence index (JSON lines)
├── ioc.jsonl          # extracted indicators (JSON lines)
├── notes/             # note files
└── output/            # tool output, narrative, reports, exports
```

`case.json` is rewritten atomically (temp file + rename) on every change, so a
crash never leaves a half-written case.

## Notes

`note <text>` appends a timestamped, authored note to the current case. Notes
appear on the timeline, in case exports, and in generated reports.

## Portability

Cases can be bundled and restored with `export` / `import`; see
[Reporting, export & themes](reporting.md#export--import).
