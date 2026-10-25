# Evidence, IOCs & Timeline

Cases record three interlocking artifacts: **evidence**, **indicators of
compromise (IOCs)**, and a **timeline**. Everything happens relative to the open
case.

## Evidence

`evidence` manages the evidence index for the current case.

| Command | Description |
|---|---|
| `evidence list` | List evidence (name, SHA-256 prefix, size, tags) |
| `evidence add <path> [--tag a,b]` | Add a file to the case and index it |
| `evidence tag <name> <tag>...` | Add tags to evidence |
| `evidence verify [name]` | Verify stored evidence against recorded hashes |
| `evidence classify [name]` | Classify evidence |

### `evidence add`

- Copies the source file into `evidence/` (unless it is already inside the case)
  and appends a record to `evidence.jsonl` with a SHA-256 hash and size.
- **Duplicate protection**: refuses to add the same content twice — either an
  exact duplicate by hash (`evidence already exists: <name> (added <time>)`) or
  an existing file whose hash differs (`refusing to overwrite`).
- Prints a **suggested tool** based on the file type (e.g. `peinfo` for PE
  files, `volatility` for memory dumps).
- When `ai.auto_analyze_evidence` is enabled in config, kicks off a background
  AI summary recorded as an `ai_response` timeline event.
- Appends an `evidence_added` timeline event.

### Tags

Tags are free-form strings (`--tag malware,packed`). The `memory-dump` tag is
special: it is how `memory` auto-selects which evidence file to analyze.

## IOCs

`ioc` extracts indicators from files or tool output and tracks them in
`ioc.jsonl`.

| Command | Description |
|---|---|
| `ioc list` | List tracked IOCs (type / value / source) |
| `ioc extract <file>` | Extract indicators from a file and track them |
| `ioc from-output <tool>` | Extract from a captured tool output file |
| `ioc diff <fileA> <fileB>` | Compare the indicator sets of two sources |

### Extraction types

The extractor recognizes common indicator types such as IP addresses, domains,
URLs, email addresses, and file hashes.

- `ioc from-output <tool>` reads the most recent captured output for that
  tool in `<case>/output/` — e.g. `ioc from-output volatility`. This is exactly
  what tool runs do automatically.
- `ioc diff <a> <b>` prints `added (only in B)`, `removed (only in A)`, and
  `shared` counts with per-indicator detail. Useful for comparing two memory
  snapshots or two tool runs.

Each extraction appends an `ioc_extracted` timeline event carrying the source
and per-type counts.

## Correlate

`correlate [--dot]` builds a cross-domain graph of tracked IOCs. Each IOC is
assigned a domain based on the tool that produced it:

| Source contains | Domain |
|---|---|
| `volatility` | memory |
| `sleuthkit`, `icat`, `fls` | disk |
| `zeek`, `tshark` | network |
| anything else | other |

Indicators that appear in **more than one domain** are highlighted — a URL found
by both `volatility` and `zeek` suggests a stronger connection than one found by
a single tool. `--dot` emits a Graphviz DOT description.

With no IOCs yet, it suggests extracting some first
(`ioc list`, `memory <plugin>`, `sandbox run -- <tool>`).

## Timeline

Every action in a case is appended as a timestamped event. `timeline list`
displays them, newest-last, with filters.

| Command | Description |
|---|---|
| `timeline list` | Show the timeline |
| `timeline list -n <N>` | Show only the last N events |
| `timeline list --type <t1,t2>` | Filter to event types |
| `timeline list --grep <pattern>` | Show events matching a pattern |
| `timeline export [path] [--format csv\|json]` | Write the timeline to a file |
| `timeline build` | Write a prose narrative to `output/narrative.md` |
| `timeline correlate` | Cross-tool view: tool-run counts + indicators by type |

### Event types

| Type | Meaning |
|---|---|
| `case_opened` / `case_closed` | Case lifecycle |
| `note` | A note was added |
| `evidence_added` / `evidence_tagged` | Evidence changes |
| `tool_run` | A tool executed (payload: tool, args, return code, duration, output file) |
| `ioc_extracted` | IOCs extracted (payload: source, counts, total) |
| `ai_query` / `ai_response` | AI interactions |
| `report_generated` | A report was produced |

### Timeline correlate

Groups tool runs by tool (with run counts) and tallies indicators by type — a
quick summary of how the investigation was built. Equivalent to G5.

## Search

- `search <query>` — search across all cases (names, notes, evidence);
  `--in-output` restricts to captured tool output (`file:line: content`).
- `grep <pattern> [--in-output]` — shorthand for `search` with `--in-output`.
