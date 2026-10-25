# Command Surface Redesign — Design

Status: proposed 2026-10-25. Code is written by orchestrated agents; this document
defines the design and acceptance criteria only.

## The problem

The REPL exposes 30 commands in one flat namespace with no grouping, mixed
conventions and several broken or misleading entries. The MCP server exposes 21
tools under a **consistent `domain_action` convention** (`tool_run`, `evidence_add`,
`case_status`, `ioc_extract`, `timeline_read`). The two surfaces disagree, and the
REPL is the one that is inconsistent.

Concrete defects, all verified against the code:

1. **Mixed flag and subcommand conventions in one domain.** `case -n`, `case -o`,
   `case -c` are single-letter flags, while `case list` and `case templates` are
   subcommands. Nothing tells you which form a given action takes.

2. **`run` is overloaded.** `run <tool> <path>` runs an analysis tool;
   `sandbox run <file>` detonates a file. Same verb, unrelated meanings, and the
   first one does not resolve case-relative paths.

3. **`use` is broken and redundant.** `use hash` returns `tool not found` even though
   `tools` lists `hash` as `ready`. Its purpose overlaps `tool run` without adding
   anything.

4. **`search` and `grep` are confusingly near-duplicates.** `grep` is an alias for
   `search --in-output`. A security analyst types `grep` expecting grep semantics.

5. **Empty states are inconsistent.** `sandbox list` with no sandboxes prints
   nothing at all; `plugin list` prints `No plugins discovered.`; `theme list` is
   rejected outright despite `help` advertising it.

6. **`ai config` cannot inspect configuration before it exists** — it errors with
   `AI not configured`, so you cannot see what you are about to set.

7. **No machine-readable output.** Only `export --json` and `cases --json` exist.
   A tool an analyst wants to pipe into `jq` cannot.

8. **No batch operations.** `evidence add` takes one path. Triage means adding
   twenty files.

9. **The IOC lifecycle is half-built.** You can extract and diff IOCs, but not add
   one manually, not search them, and not export them to a shareable format.

10. **No ATT&CK surface.** capa output is mapped to ATT&CK techniques internally,
    but there is no way to query them.

## Design principles

**P1 — Canonical form is `<domain> <verb>`.** Actions are subcommands, never
single-letter flags. Options are flags. This matches the MCP surface exactly, so
the REPL and MCP stop disagreeing and there is one convention to learn.

**P2 — Domains are the analyst's nouns.** `case`, `evidence`, `ioc`, `timeline`,
`tool`, `sandbox`, `report`, `search`, `plugin`, `theme`, `ai`. One domain, one
concern.

**P3 — Every command has a short alias.** Discovered via `help`, not by reading
source. Aliases never collide with a canonical name.

**P4 — `--json` on every command.** Human tables by default, machine-readable on
request. A tool that cannot emit JSON cannot be scripted.

**P5 — Every empty state prints a message.** "Nothing here" must be
distinguishable from "the command did nothing".

**P6 — Breaking rename: old names are removed, not aliased.** Decided 2026-10-25.
The new names are canonical and the old ones are gone. The cost is accepted
explicitly: the README, the MCP tool list, the vault notes and the user's muscle
memory all reference `run`, `tools`, `cases`, `status`, `evidence` and `volatility`.

Because the old names disappear, `help` must make the new surface discoverable —
that is the entire mitigation, since there is no alias to fall back on. An analyst
who types `run` gets `unknown command: run` and has to read `help`. That is the
trade: a clean surface with no legacy cruft, paid for with a relearn.

The `Was` column in each table below is therefore a **migration map**, not a list
of aliases. T6.8 is not optional — a rename that leaves docs pointing at removed
names is worse than no rename.

## Canonical surface

### case

| Command | Was | Notes |
|---|---|---|
| `case create <name> [-t <tmpl>]` | `case -n` | |
| `case open <name>` | `case -o` | |
| `case close` | `case -c` | |
| `case list [--status open\|closed]` | `cases` | `--json` |
| `case status` | `status` | `--json` |
| `case templates` | `case templates` | unchanged |
| `case delete <name>` | — | new; prompts first |
| `case switch <name>` | — | new; opens without closing the current case |

### evidence

| Command | Was | Notes |
|---|---|---|
| `evidence add <path>... [--tag a,b]` | `evidence add <path>` | multi-file |
| `evidence list` | `evidence` | `--json` |
| `evidence show <name>` | — | new |
| `evidence tag <name> <tag>` | `evidence tag` | |
| `evidence verify [name]` | `evidence verify` | |
| `evidence classify [name]` | `evidence classify` | |
| `evidence hash <name>` | — | new |
| `evidence pull <name> [dest]` | — | new; copies out of the case |
| `evidence delete <name>` | — | new; prompts first |

### ioc

| Command | Was | Notes |
|---|---|---|
| `ioc list` | `ioc` | `--json` |
| `ioc add <indicator> [--type t]` | — | new; manual entry |
| `ioc extract <file>` | `ioc <file>` | |
| `ioc from-output <tool>` | `ioc --from-output` | accepts the bare tool name (T0.5) |
| `ioc diff <a> <b>` | `ioc diff` | resolves against the case `output/` |
| `ioc search <pattern>` | — | new |
| `ioc export [--format csv\|json\|stix]` | — | new |
| `ioc delete <indicator>` | — | new |

### timeline

| Command | Was | Notes |
|---|---|---|
| `timeline list [-n N] [--type t] [--grep p]` | `timeline` | `--json` |
| `timeline show <id>` | — | new |
| `timeline export [path]` | `timeline export` | |
| `timeline build` | `timeline build` | |
| `timeline correlate` | `timeline correlate` | |
| `timeline stats` | — | new; event counts by type |

### tool

| Command | Was | Notes |
|---|---|---|
| `tool list` | `tools` | `--json` |
| `tool run <name> <path>` | `run` | case-relative paths resolve (T0.4) |
| `tool install <name>` | `install` | |
| `tool build <name>` | `build` | |
| `tool info <name>` | `use` | replaces the broken `use` |
| `tool analyze <path> [--rules r]` | `analyze` | |
| `tool doctor` | `doctor` (W2) | |
| `tool dashboard` | `dashboard` / `dash` | launches the W5 TUI |

### forensics

| Command | Was | Notes |
|---|---|---|
| `memory <plugin> [-f <dump>]` | `volatility`, `mem` | |
| `disk <tool> [-i <image>]` | `disk`, `sleuthkit` | |
| `registry <hive> [-p plugin]` | `regripper` | |

### unchanged

`report`, `export`, `import`, `note`, `correlate`, `search`, `grep`, `plugin`,
`theme`, `sandbox`, `ai`, `clear`, `exit`, `help`.

`grep` stays as an alias for `search --in-output`, but `help` documents it as an
alias rather than a peer command, so the duplication stops looking like two
features.

## Consistency rules

1. Actions are subcommands; options are flags. No domain mixes both.
2. `--json` on every command, including the ones that currently lack it.
3. Every empty state prints a message naming what is empty.
4. Every command has a short alias, listed by `help`.
5. Errors name the command and the argument that failed, not just the syscall.
6. Destructive actions (`case delete`, `evidence delete`, `ioc delete`) prompt
   first and name what will be lost.
7. Paths given to `tool run`, `ioc extract` and `evidence add` resolve against the
   open case where a case-relative path is unambiguous, and accept absolute paths
   always.

## Capability gaps this closes

- **Batch triage** — `evidence add a.bin b.bin c.bin --tag malware`.
- **IOC lifecycle** — add, search, export to CSV/JSON/STIX. Threat-intel sharing is
  the point of an IOC store; extraction alone is not.
- **Scriptability** — `--json` everywhere, so the shell can be driven by other
  tools.
- **Evidence egress** — `evidence pull` copies an artefact out of the case without
  leaving the shell.
- **ATT&CK** — `attack <technique-id-or-name>` queries the technique catalogue and
  the capabilities capa already extracted, so an analyst can move from a finding to
  the technique without leaving the tool.

## Out of scope

- Pipes between commands (`evidence | ioc`). Real composition needs a lexer and a
  type system; that is a larger change than this redesign.
- Renaming the MCP tools. They are already consistent; the REPL converges on them.
- Changing case, evidence or timeline storage formats.
- A plugin/extension mechanism for commands.

## Workstream W6 — tasks

- **T6.1** Canonical `domain action` dispatch, replacing the old names.

  **AC:** every canonical name in the table above works; **no old name works** —
  `run`, `tools`, `cases`, `status`, `evidence` and `volatility` each return
  `unknown command` naming the replacement.
  **AC:** `help` lists canonical names grouped by domain.
  **AC:** tab completion completes canonical names and their subcommands.

- **T6.2** `--json` on every command.

  **AC:** each of the 30+ commands emits valid JSON with `--json`; `jq` parses the
  output of `tool list`, `case list`, `evidence list` and `ioc list`.
  **AC:** without `--json` the human table is unchanged.

- **T6.3** Empty states everywhere.

  **AC:** every list command prints a named empty-state message when it has nothing
  to show, including `sandbox list`.
  **AC:** `theme list` is accepted (currently rejected) or the help text and docs
  stop advertising it.

- **T6.4** `ioc` lifecycle: `add`, `search`, `export`.

  **AC:** `ioc add` records a manual indicator with a type and lands on the
  timeline.
  **AC:** `ioc search` matches value, type and source.
  **AC:** `ioc export --format csv|json|stix` writes a file and records the export
  on the timeline.

- **T6.5** Batch evidence and egress.

  **AC:** `evidence add` accepts multiple paths, reporting per-file success and
  failure rather than aborting on the first bad path.
  **AC:** `evidence pull <name> [dest]` copies an artefact out of the case and
  verifies the hash after the copy.

- **T6.6** Replace `use` with `tool info`.

  **AC:** `tool info <name>` prints the tool's description, category, source,
  status, resolved command or image, and run count.
  **AC:** `use <name>` still works as an alias and points at `tool info`.

- **T6.7** ATT&CK surface.

  **AC:** `attack <id-or-name>` prints the technique, its tactics, and any capa
  capability in the current case mapped to it.
  **AC:** `attack list` prints the mapped techniques for the current case.

- **T6.8** Documentation.

  **AC:** README quick-start uses canonical names.
  **AC:** every `docs/*.md` command reference uses canonical names with aliases
  noted.
  **AC:** the vault's [[Mimir Commands]] note is updated to the new surface.

## Sequencing

```
T6.1 (dispatch) ─► T6.2 (--json) ─► T6.3 (empty states)
                 └► T6.4 (ioc) ─┐
                 └► T6.5 (evidence) ─┤
                 └► T6.6 (tool info) ─┤
                 └► T6.7 (attack) ────┘
                                      └─► T6.8 (docs)
```

T6.1 must land first — everything else adds commands to the new dispatch. T6.2
through T6.7 are independent of each other but all edit `internal/shell/`, so they
run sequentially. T6.8 is last.

## Acceptance for the workstream

- `go build ./...`, `go vet ./...`, `go test ./...` pass.
- Every old command name still works; no documented example breaks.
- `help` output is grouped by domain and fits the terminal width.
- The REPL command surface and the MCP tool surface follow the same
  `domain_action` convention.
