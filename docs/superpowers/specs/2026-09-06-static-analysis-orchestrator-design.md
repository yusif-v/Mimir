# Static Analysis Orchestrator — Design

Status: approved by user, pre-implementation
Part of: Mimir Malware Analysis Workbench (sub-project 1 of 5)

## Context

The Malware Analysis Workbench idea (orchestrate static/dynamic analysis,
extract IOCs, generate YARA/Sigma rules, write forensic reports) is too large
for one spec. It decomposes into:

1. **Static Analysis Orchestrator** (this spec)
2. Dynamic Analysis Integration (sandboxed execution, e.g. via `cuckoo`/the
   existing `sandbox` command)
3. YARA Rule Generator
4. Sigma Rule Generator
5. Workbench Report (consolidated static+dynamic+IOC+rules report)

This spec covers only #1: a single command that runs the applicable static
tools against a sample and folds everything into the open case, with no
sandboxing or code execution of the sample itself.

Mimir already has the building blocks: the `yara`, `capa`, and `oletools`
catalog tools; the `ioc.Extract` regex-based IOC extractor; and the
`captureInto` pipeline (timeline event + IOC extraction + tool-aware hint
parsing) already used by the `disk`, `volatility`, and `regripper` command
paths. This orchestrator is new glue code, not new infrastructure.

## Goals

- `analyze <path>` runs the right static tools automatically based on file
  type, with no per-run tool selection required from the analyst.
- Every tool run is recorded exactly like a manual `run`/`disk`/`volatility`
  invocation today: timeline event, IOC extraction, tool-aware hints.
- capa's ATT&CK-mapped capability output is captured as structured data, not
  just raw text, giving ATT&CK technique tagging "for free" for PE/ELF
  samples.
- Missing tools degrade gracefully (warn + skip), never abort the whole run.

## Non-goals

- No dynamic/behavioral analysis (no sample execution) — that's sub-project 2.
- No YARA/Sigma rule generation — sub-projects 3/4.
- No new report format — sub-project 5.
- No new external dependencies (no libmagic/`file` shellout).

## Components

### 1. `internal/filetype` (new package)

Pure Go, magic-byte sniffing only, no filesystem/tool dependency beyond
reading the first N bytes of the sample.

```go
package filetype

type Kind string

const (
    PE      Kind = "pe"
    ELF     Kind = "elf"
    OLE     Kind = "ole"    // legacy Office: doc/xls/ppt (CDF container)
    OOXML   Kind = "ooxml"  // modern Office: docx/xlsx/pptx (zip container)
    Unknown Kind = "unknown"
)

// Detect reads the header of the file at path and classifies it.
func Detect(path string) (Kind, error)

// DetectBytes classifies an in-memory buffer (used by tests and by Detect).
func DetectBytes(data []byte) Kind
```

Detection rules:
- `MZ` (0x4D 0x5A) at offset 0 → `PE`
- `\x7fELF` at offset 0 → `ELF`
- `D0 CF 11 E0 A1 B1 1A E1` at offset 0 → `OLE`
- `PK\x03\x04` at offset 0, and the zip central directory or first entries
  contain `[Content_Types].xml` or a `word/`/`xl/`/`ppt/` path → `OOXML`
  (a plain zip that isn't Office falls through to `Unknown`)
- anything else → `Unknown`

`DetectBytes` only needs to look at a small prefix (a few KB) for the OOXML
check, so `Detect` reads the file in one bounded read rather than loading
huge samples fully into memory.

### 2. Tool-selection table

Lives alongside the new `cmdAnalyze` in `internal/shell`:

```go
var staticToolsFor = map[filetype.Kind][]string{
    filetype.PE:      {"yara", "capa"},
    filetype.ELF:     {"yara", "capa"},
    filetype.OLE:     {"yara", "oletools"},
    filetype.OOXML:   {"yara", "oletools"},
    filetype.Unknown: {"yara"},
}
```

Resolution order per tool name: check `builtins.Has(name)`, then
`a.Tools.Get(name)` (installed catalog tool). Not found → print
`warning: tool "<name>" not installed — run: install <name>` to stderr and
skip it. This mirrors the existing "warning, don't abort" pattern used
throughout `shell.go` (e.g. `recordRun`'s save-case warning).

### 3. `cmdAnalyze` orchestration flow

```
analyze <path>
```

1. Require an open case (`a.Cases.Current()`), else error.
2. Read/hash the sample, `AppendEvidence` (op "add") exactly as `evidence add`
   does today — the sample becomes case evidence before analysis runs.
3. `filetype.Detect(path)` → look up the tool list.
4. For each tool in the list that resolves to an installed tool/builtin, build
   its invocation per the fixed per-tool arg rules below, run it (same
   invocation path as `runToolByName`, but `analyze` calls `captureInto`
   directly instead of `recordRun`, since `captureInto` is the richer,
   IOC-aware pipeline), print its raw stdout as it completes (matches `run`'s
   current UX), then `captureInto(c, toolName, command, sampleName, output)`.
5. After all tools run, print the summary table (see Output below).

**Per-tool invocation** (fixes the underspecified "run it against the
sample" — each of these three tools needs different args, not just a bare
path):

- **capa**: `capa -j /evidence/<sample>`. JSON output only; no extra
  arguments needed.
- **oletools**: `olevba /evidence/<sample>`. oletools bundles several
  scripts (olevba, oleid, mraptor); `olevba` is the one that surfaces
  macro/VBA content and is the most useful single default for triage, so
  it's the only oletools sub-script `analyze` runs in this sub-project.
  (Running the others too is a natural future addition, not needed for v0.1.)
- **yara**: needs a rules file, which Mimir has no bundled default for.
  `analyze` gains a `--rules <path>` flag; if omitted, it looks for
  `<case>/rules.yar` and uses that if present. If neither is given nor
  found, yara is skipped with `warning: no yara rules available (pass
  --rules or place rules.yar in the case dir) — skipping yara`, and its
  summary row shows `skipped`.

Tool failures (nonzero exit, missing binary at exec time) are caught and
reported as a warning per tool; `analyze` continues to the next tool and
still produces a summary at the end. The command as a whole only returns an
error for the fatal, pre-loop conditions (no open case, sample unreadable).

### 4. capa capability hints (extends `captureInto`)

New file `internal/shell/capa.go`, same shape as the existing
`volatilityHints`/`regripperHints`:

```go
type capaHint struct{ typ, val string }

// capaHints parses capa's `-j` JSON output into one hint per matched rule.
func capaHints(out string) []capaHint
```

Each matched capa rule becomes `capaHint{typ: "capability", val: "<ATT&CK ID> <rule/technique name>"}`
(e.g. `"T1055 Process Injection"`); rules with no ATT&CK mapping are skipped
(capa maps most but not all rules to ATT&CK).

`captureInto` gets one more branch, following the existing pattern for
volatility/network/disk/regripper:

```go
if tool == "capa" {
    for _, h := range capaHints(out) {
        c.AppendIOC(cases.IOCRecord{Type: h.typ, Value: h.val, Source: tool + ":" + command + dumpSuffix(dump), Time: now})
    }
}
```

This requires `analyze` to invoke capa with `-j` (JSON output) specifically
when it's the orchestrator calling it, so the raw stdout captured and
recorded is JSON (capa's normal human-readable text goes to a case output
file as usual via existing output-recording, and is not the primary display
format for `analyze`'s capa step — a short "N capabilities matched" line
is printed instead of the raw JSON blob, to keep terminal output readable).

### 5. Output

Per-tool: printed live as each tool finishes (yara's raw match output;
oletools' raw findings; capa's summarized capability count instead of raw
JSON, per above).

Final summary table:

```
TOOL       STATUS   IOCS FOUND   CAPABILITIES FOUND
yara       ok       3            -
capa       ok       0            12
```

`STATUS` is `ok`, `failed`, or `skipped` (skipped = not installed).

## Data flow

```
analyze <path>
  → filetype.Detect(path)
  → staticToolsFor[kind]
  → for each tool:
        run tool against path
        captureInto(c, tool, cmd, sampleName, stdout)
          → timeline event (tool_run)
          → ioc.Extract(stdout) → generic IOCs
          → tool-specific hints (capaHints for capa; existing hints
            untouched for other tools — yara/oletools get no special
            hint parser in this sub-project, generic IOC extraction only)
  → print summary table
```

## Error handling

- No open case → error, command aborts before doing anything.
- Sample path unreadable → error, command aborts.
- Tool not installed → warning, skip, continue.
- Tool exits nonzero / binary missing at exec → warning (includes stderr),
  continue; summary row shows `failed`.
- `AppendEvidence`/`AppendIOC`/`AppendEvent` failures → warning (matches
  existing `c.Save()`-failure warning style elsewhere in `shell.go`), never
  fatal — an analyst should not lose the rest of the analysis run because one
  disk write hiccuped.

## Testing

- `internal/filetype`: table-driven tests over small byte-fixture headers
  for PE/ELF/OLE/OOXML/Unknown, plus the "plain zip that isn't Office"
  negative case.
- `internal/shell/capa.go`: `capaHints` tested against a small embedded capa
  `-j` JSON fixture (2-3 rules, one without an ATT&CK mapping to confirm it's
  skipped).
- `internal/shell`: a `cmdAnalyze`/tool-selection test using the existing
  dry-run/builtin test harness patterns already in `shell_internal_test.go`
  (e.g. mirroring the `captureInto` tests already present there), covering:
  tool selection per detected type, missing-tool warning-and-continue
  behavior, and evidence auto-registration.

## Command reference addition

`docs/analysis.md` gets a new `## Static Analysis (analyze)` section
documenting `analyze <path> [--rules <path>]`, matching the existing style of
the Volatility/Disk sections in that file. `internal/shell/completer.go` gets
`analyze` added to `commandNames` with file-path completion (falls through
to the existing `default: s.filePath(word)` branch, no special-casing
needed). Help text (`cmdHelp`) gets one new line.
