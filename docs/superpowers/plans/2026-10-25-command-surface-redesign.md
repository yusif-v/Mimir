# Command Surface Redesign — Implementation Plan

Design: `docs/superpowers/specs/2026-10-25-command-surface-design.md`.
Workstream: W6 in `PLAN.md`. Proposed 2026-10-25.

Eight tasks. T6.1 restructures the dispatch; T6.2–T6.7 add capability on top of it;
T6.8 is documentation. T6.1 must land first, and T6.2–T6.7 all edit
`internal/shell/` so they are sequential.

## Phase 1 — the dispatch

### T6.1 Canonical `domain action` names, old names as aliases

- **Do:** restructure the REPL dispatch to `<domain> <verb>`; register every old
  name as an alias that prints a one-line deprecation notice naming its
  replacement. Group `help` output by domain and add alias + subcommand
  completion.
- **Done when:** every canonical name works; every old name still works with a
  deprecation notice; `help` is grouped by domain and fits the terminal width.
- **Watch out for:** the dispatch is a hand-written `switch` on `args[0]`
  (`internal/shell/shell.go:275`), not a command framework. Adding domains means
  restructuring that switch — do it once, not per command. And the deprecation
  notice must go to stderr or it will corrupt `--json` output.
- **Learn:** why aliases beat renames — a breaking CLI change costs every script,
  doc and muscle memory at once, and buys nothing the alias does not.

### T6.2 `--json` on every command

- **Do:** add a shared `--json` flag path and emit structured output per command.
- **Done when:** all commands emit valid JSON; `jq` parses `tool list`,
  `case list`, `evidence list` and `ioc list`; human output is unchanged without
  the flag.
- **Watch out for:** the deprecation notices from T6.1. If they go to stdout they
  break JSON parsing — this is the first place that bites.
- **Learn:** a flag that changes the output format has to be handled before any
  other output is written.

## Phase 2 — capability

### T6.3 Empty states everywhere

- **Do:** give every list command a named empty-state message; fix `theme list`,
  which is currently rejected while `help` advertises it.
- **Done when:** every list command names what is empty; `theme list` either works
  or is removed from `help` and the docs.
- **Watch out for:** `theme list` is rejected because the parser only accepts
  `list` as a bare `theme` with no argument — check the argument handling before
  assuming it is a missing case.
- **Learn:** "no output" and "nothing here" are different answers, and only one of
  them is honest.

### T6.4 IOC lifecycle

- **Do:** add `ioc add`, `ioc search`, `ioc export --format csv|json|stix`.
- **Done when:** a manual indicator lands on the timeline; search matches value,
  type and source; export writes a file and records the export.
- **Watch out for:** STIX output needs a valid `@context` and id format — emit
  STIX 2.1 with a deterministic id derived from the indicator, not a random UUID,
  or re-exporting the same case produces different files.
- **Learn:** an IOC store you cannot add to or export from is a read-only log, not
  a threat-intel workflow.

### T6.5 Batch evidence and egress

- **Do:** `evidence add` accepts multiple paths; add `evidence pull`.
- **Done when:** a multi-file add reports per-file success and failure instead of
  aborting; `evidence pull` copies an artefact out and verifies the hash after.
- **Watch out for:** per-file reporting means partial success is a real state — the
  exit code and the summary line have to reflect it, or a failed file looks like a
  succeeded one.
- **Learn:** batch operations need per-item results, not a single success flag.

### T6.6 Replace `use` with `tool info`

- **Do:** add `tool info <name>`; keep `use` as an alias.
- **Done when:** `tool info` prints description, category, source, status, resolved
  command or image and run count; `use` points at it.
- **Watch out for:** `use` currently fails for builtins because it resolves only
  the installed registry — the same defect as the TUI. Fix the resolution, do not
  just alias it.
- **Learn:** a command that silently resolves a different set of things than the
  command it resembles is worse than one that fails loudly.

### T6.7 ATT&CK surface

- **Do:** add `attack <id-or-name>` and `attack list`, reading the capa capabilities
  already mapped to ATT&CK.
- **Done when:** a technique query prints the technique, its tactics and the
  matching capabilities in the current case; `attack list` prints what is mapped.
- **Watch out for:** capa capabilities are stored as IOCs of type `capability`
  (added in the static-analysis work) — find how they are recorded before assuming
  a shape, and handle a case where nothing was analysed.
- **Learn:** the value of an ATT&CK mapping is being able to ask the tool about it,
  not just having it in a file.

## Phase 3 — documentation

### T6.8 Docs

- **Do:** move the README quick-start and every `docs/*.md` command reference to
  canonical names with aliases noted; update the vault's [[Mimir Commands]] note.
- **Done when:** no documented example uses a name that does not work; the vault
  note reflects the new surface.
- **Watch out for:** the README's quick-start currently contains
  `run hash evidence/sample.bin`, which is broken (T0.4). Fixing T0.4 changes what
  the example should be — check it after T0.4 lands, not before.
- **Learn:** docs that name commands are a contract; renaming without updating them
  breaks the contract silently.

## Sequencing

```
T6.1 ─┬─► T6.2 ─► T6.3 ─┐
      ├─► T6.4 ──────────┤
      ├─► T6.5 ──────────┤
      ├─► T6.6 ──────────┤
      └─► T6.7 ──────────┴─► T6.8
```

T6.2–T6.7 are mutually independent but all edit `internal/shell/`, so run them
sequentially. T6.4 depends on T0.5 (`ioc --from-output` accepting a bare tool name)
for consistent path resolution — land T0.5 first or together.

## Acceptance for the workstream

- `go build ./...`, `go vet ./...`, `go test ./...` pass.
- Every old command name still works; no documented example breaks.
- `help` is grouped by domain and fits the terminal width.
- The REPL and MCP surfaces follow the same `domain_action` convention.

## Report back

The agent reports: files changed, commands run, the `help` output, and the output of
`--json` for at least one command piped through `jq`. A claim that "the aliases
work" without showing an old name still resolving is not evidence.
