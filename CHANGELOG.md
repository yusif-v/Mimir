# Changelog

All notable changes to Mimir will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **Dashboard rewritten around a merged tool list.** The explorer previously
  passed only `Registry.List("")` — the *installed* registry — so the native
  builtins and catalog entries were never offered and a fresh install rendered
  nothing but `No tools match the current filter`. It now merges all three tool
  sources into one list, each row labelled `[builtin]`, `[installed]` or
  `[available]`. Rows are width-bounded (long descriptions wrap to indented
  continuation lines, nothing exceeds the terminal width), colours come from the
  active theme so `theme set` reaches the dashboard, and `/` searches by name,
  description, category and tags. `Tab` expands the selected row in place with
  its full description, tags, source, status, run count and category. The
  viewport now measures rendered row heights instead of assuming a fixed number
  of chrome lines, keeping the header pinned and the last visible row unclipped,
  and an in-flight refresh shows `refreshing…` in the footer with spinner status
  glyphs instead of stale values.

### Fixed
- **`tools` output is now an aligned table.** `INSTALLED`, `AVAILABLE` and
  `BUILT-IN` were rendered as hand-rolled `Printf` lines with hardcoded widths, so
  `disk-forensics` (14 chars) overflowed the `%-12s` category field and the
  description column shifted right. They now build `ui.Table` output with a
  visible `DESCRIPTION` column — box borders on a TTY, plain space-aligned columns
  when piped.
- **`ui.Table` mis-measured ANSI-coloured cells.** `displayWidth` skipped only the
  `ESC` byte and then counted `[`, the digits and `m` as printable, so a colourised
  cell measured 7 columns too wide and broke its column alignment. It now consumes
  the whole ANSI CSI sequence. Measured before the fix:
  `displayWidth("\033[32myara\033[0m")` returned 11, want 4.

### Build
- **`go.sum` is now tracked.** It was listed in `.gitignore`, so a fresh clone
  could not build without `go mod tidy` and failed with `missing go.sum entry` for
  9 packages. `go.mod` is now reconciled with what the code imports —
  `github.com/alexmullins/zip`, `github.com/bodgit/sevenzip` and
  `github.com/docker/go-connections` promoted from `// indirect` to direct.

## [0.8.4] - 2026-09-14

### Added
- **`analyze` (static analysis orchestrator)** — `analyze <path> [--rules
  <path>]` auto-detects a sample's file type by magic bytes (PE, ELF, legacy
  OLE and OOXML Office docs) and runs the applicable static tools
  (`yara`/`capa` for PE/ELF, `yara`/`oletools` for Office docs) against it.
  The sample is registered as case evidence automatically, each tool's
  output is recorded to the timeline and scanned for IOCs via `captureInto`
  and saved to `<case>/output/`, and capa's ATT&CK-mapped capabilities are
  recorded as structured `capability` IOCs. Missing tools are skipped with a
  warning rather than aborting the run.
- **`disk` (first-class SleuthKit)** — `disk <tool> [-i <image>] [args...]`
  (alias: `sleuthkit`). Image resolution like the volatility command:
  `-i` absolute/relative/substring, auto-select evidence tagged
  `disk-image`, else most recent. Output is captured to the timeline via
  `captureInto`, scanned for IOCs, and parsed for tool-specific hints:
  `partition` indicators from `mmls`, `deleted_file` indicators from `fls`.

### Changed
- **RegRipper upgraded to a first-class command** — `regripper [hive]
  [-p plugin] [--json]` now resolves the hive automatically (positional /
  `registry-hive` tag / most recent), mounts only that hive read-only,
  and feeds output through `captureInto`: timeline event, generic IOCs, and
  registry hints (`usb_device` USBSTOR keys, `winpath` executable paths,
  `autostart` Run/RunOnce lines).

### Fixed
- **Auto-select fallback picked wrong evidence** — both dump and new-file
  resolution fell back to "last item of Evidence()", but Evidence() is
  name-sorted; they now compare `AddedAt` timestamps to pick the genuinely
  most recently added item.

## [0.8.3] - 2026-08-01

### Added
- **`dashboard` / `dash`** — interactive Bubble Tea TUI: a tool explorer with
  live status, filters, and run counts pulled from the case timeline; pressing
  Enter runs the selected tool in the current case. Replaces the standalone
  `cmd/tui-tools` scaffold.
- **`unzip` password/AES support** — list or extract password-protected ZIP
  (WinZip AES / method 99) and 7-Zip archives via `-p <password>`.
- **MCP tools** — `case_close` (tears down sandboxes, never deletes the case)
  and `search` (cross-case search over names, notes, timeline, IOCs, evidence,
  metadata, and optionally captured output under `output/`).
- **Tool templates** — optional `[docker.ports]` host/container port mappings
  and a `version` smoke-test arg; `checksum` for local downloads.
- **Case name validation** — `ValidateCaseName` rejects empty, absolute,
  separator-containing, and `..` names at the manager boundary.

### Fixed
- **Path traversal via case name** — `Create`/`Open`/`Load` now reject names
  that could escape the storage base directory.
- **Atomic `case.json` saves** — metadata is written to a temp file and renamed
  over `case.json`, so a crash mid-write can never leave a truncated case.
- **Global `os.Chdir` side effects** — case `Open`/`Close` no longer chdir into
  the case directory.
- **IOC false positives** — IPv4 extraction validates octets with
  `net.ParseIP`; domain/email extraction filters file-extension lookalikes
  (`.exe`, `.dll`, `.cmd`, …) that dominate forensic output.
- **`downloadFile`** — now enforces a 10-minute timeout and optional SHA-256
  checksum verification.
- **Sandbox `cp`** — stages files under `/work` (the exec-capable tmpfs)
  instead of noexec `/tmp`, so copied binaries can actually run in-container.
- **cuckoo template** — category corrected to `malware`.
- **REPL blank lines** — no more stray blank line on empty input or after
  command output.
- **README demo GIF** — regenerated from a real `asciinema` recording of the
  REPL (previous GIF had corrupted/misaligned text from a synthetic frame
  renderer).

### Security
- Tool execution hardening (sandbox cap-drop ALL, no-new-privileges,
  pids-limit; 1h local / 30m docker timeouts; 8 MiB output caps; docker
  memory/CPU limits; real exit codes).
- AI context is PII-safe for remote providers: evidence filenames are masked
  (`evidence_N`) unless the provider is local.

### Tests
- Case-name validation (absolute, `..`, separator, ok); atomic save; IPv4
  octet + file-extension IOC regressions; `case_close`; MCP `search`;
  `ioc_extract` persist flag; capped-buffer + exit-code tests; TUI explorer
  model tests.

## [0.8.2] - 2026-07-31

### Fixed
- **IOC extraction false positives** — IPv6 candidates are now validated with
  `net.ParseIP` instead of relying on word-boundary regex anchoring, so
  time-like strings (`06:19:54`) and MAC addresses (`00:1a:2b:3c:4d:5e`) in
  syslog/auth logs are no longer reported as IPv6 indicators. The Brutus
  auth.log now yields 6 real indicators instead of ~57.
- **Duplicate `case_opened` timeline events** — the MCP server resolved a case
  by name on every tool call via `Cases.Open`, which appended a `case_opened`
  event each time (plus status flips, chdir, and event emission). Added a
  side-effect-free `Cases.Load` and switched the MCP `withCase` helper to it,
  so read/write tool calls no longer mutate the timeline.
- **Empty "Tool Output (Findings)" section** — tool runs recorded via
  `recordRun`/MCP `tool_run` carry `tool`/`args`/`output_file` instead of the
  inline `command`/`output` of `captureInto`, so the report skipped them.
  Reports, narrative, archive, and export manifest now reconstruct the command
  and read the captured `output_file` (truncated to 2000 bytes).

### Tests
- IPv6 false-positive regression tests (timestamps, MACs, single-colon,
  real IPv6 still extracted); MCP server does not reopen the case per call;
  report surfaces both inline and recorded tool output.

## [0.8.1] - 2026-07-31

### Added
- **Cuckoo Sandbox** — Added Docker template for automated malware analysis. Allows submission and processing of files for behavioral analysis, network traffic capture, memory dumps, and reporting.

## [0.8.0] - 2026-07-29

### Added
- **`mimir mcp`** — Mimir is now a Model Context Protocol server. Exposes DFIR
  operations as MCP tools over stdio and Streamable HTTP so Claude Desktop /
  Claude Code and other MCP clients can create cases, add and verify evidence,
  read and correlate the timeline, extract and diff IOCs, list and run tools,
  and generate reports/exports.
- Stateless tool model (explicit `case` argument per tool); execution tools
  (`tool_run`/`tool_build`/`tool_install`) carry destructive MCP annotations.
- Streamable HTTP transport binds loopback by default; non-loopback bind
  requires a bearer token (`--token` / `MIMIR_MCP_TOKEN`, sent as
  `Authorization: Bearer <t>`).

### Security
- HTTP: an empty host (e.g. `--http :7777`) is normalized to `127.0.0.1` so the
  bound interface matches the auth decision — previously it bound all
  interfaces while being classified as tokenless loopback. An explicit
  non-loopback bind (`0.0.0.0:...`) still requires a token.
- `ioc_extract`'s `from_output` argument is now restricted to a bare filename
  under the case `output/` directory, rejecting `..`/path-separator traversal.

## [0.7.5] - 2026-07-12

### Added (final gap-analysis gaps: G3, G5, G12)
- **G3** `export --provenance` writes a full provenance `manifest.json` (hash of every archived file + Mimir version + tool runs + extracted indicators) so a bundle is self-describing and verifiable.
- **G5** `timeline correlate` — groups tool runs by tool and shows an indicator-by-type summary (cross-tool view of how the case was built).
- **G12** `correlate [--dot]` — cross memory/disk/network IOC graph; classifies each indicator by source domain and flags cross-domain indicators (same artifact seen via 2+ tools). `--dot` emits a Graphviz digraph.
- `correlate` registered as a top-level command (tab-completion added).

### Changed
- IOC dedupe now keys on (type, value, source) instead of (type, value), so the same indicator found by multiple tools is retained — required for cross-domain correlation (G12).

### Tests
- `ExportFull` manifest provenance; `timeline correlate` groups by tool; `correlate` flags cross-domain IOCs.

### Notes
- Closes every gap in `docs/gap-analysis.md`. Only optional follow-up: a strict DFXML sidecar format (provenance manifest already covers integrity).

### Added (medium/low gaps from docs/gap-analysis.md)
- **G4** `evidence classify [name]` suggests the right tool by extension + magic bytes (volatility/regripper/sleuthkit/tshark/zeek/chainsaw/oletools/capa/lnkparse). `evidence add` now auto-prints the suggestion.
- **G7** `ioc diff <a> <b>` compares extracted indicator sets (added / removed / shared) — for diffing two memory snaps or two tool runs.
- **G8** zeek/tshark output now yields network IOCs (conn.log src/dst IPs, tshark `-T fields` http.host / dns.qry.name / http.request.uri) via `networkHints` in the shared capture path.
- **G10** `volatility <plugin> --offline` passes `-u` (no symbol download) for airgapped/cached analysis.

### Tests
- `suggestTool` per-extension; `networkHints` from tshark fields; `ioc diff` added/removed/shared.

### Notes
- G9 (`install` auto-build) and G11 (tab-completion for tool names) were already present — confirmed, no change needed. Remaining: G3 (export sidecar), G5 (cross-tool timeline correlation), G12 (`correlate` graph).

### Fixed
- SleuthKit template ENTRYPOINT now passthroughs the tool name, so `icat`/`fsstat`/`istat`/`mmls`/`blkcalc` (and all Sleuth Kit binaries) run — previously only `fls` worked (bare `["fls"]` ENTRYPOINT prependend `fls` to every arg).
- Generalized tool-output capture (G2/G6): the generic `sandbox run <id> -- <tool> [args]` path now appends a `tool_run` timeline event + auto-extracts IOCs, the same way the dedicated `volatility` command does. Capture logic is now a single `captureInto` helper called by both paths, so every sandbox tool feeds the timeline, Findings, and IOCs.

### Tests
- `captureInto` appends timeline + IOCs (incl. volatility command hints); `sandbox run -- <tool>` invokes `Capture`.

### Added
- Report now surfaces actual tool output in a `## Tool Output (Findings)` section (captured from `tool_run` timeline events), so findings appear in the report — not just that a tool ran.
- `timeline build --auto` narrative now renders `tool_run` events with the command + "(output in Findings)" instead of dumping the full payload; `tool_run` correctly grouped under Acquisition & Analysis.
- Volatility plugin-aware hints: `volatility` captures executed commands from cmdscan/cmdline/consoles output and NTLM hashes from hashdump as IOCs (types `command` / `ntlm`).

### Tests
- BuildReport surfaces tool output; narrative shows tool_run command; volatilityHints extracts commands + NTLM hashes.

### Added
- `volatility` command (alias `mem`): first-class memory forensics via Volatility 3. Resolves the dump from `-f <dump>` or auto-selects evidence tagged `memory-dump`, mounts it read-only into `dfir-volatility:latest`, runs `vol <plugin> -f /evidence/<dump>`, prints output, appends a `tool_run` timeline event, and auto-extracts IOCs.
- `sandbox run <id> -- <command>` now accepts an inline command/args (via `--` separator) instead of only a preset TargetFile.

### Fixed
- Report's practice procedure corrected: `volatility` is the real command; `sandbox run` does not accept inline `vol` args.

### Added
- `report [path] [--format md|pdf] [--no-ai]` — full incident report (metadata + evidence + IOCs + timeline + notes + AI executive summary). PDF via optional pandoc Docker, degrades to Markdown with a warning. Records a `report_generated` timeline event.
- `ai summarize` — standalone case narrative from timeline + evidence + IOCs.
- `timeline build --auto` — correlates case events into a phase-labeled `narrative.md` (does not mutate timeline.jsonl).
- Auto-AI on `evidence add`: opt-in via `ai.auto_analyze_evidence` (env `MIMIR_AI_AUTO_ANALYZE`), non-blocking; attaches the analysis as an `ai_response` event.
- `internal/cases`: `BuildReport`, `BuildNarrative`/`Case.Narrative` for programmatic report + narrative generation.
- `ai.Summarize` returns a structured case narrative as a string (non-streaming).

### Changed
- Modular AI provider registry + `ai models` + `ai use <model> [--save]` (Light redesign).
- lnkparse now decodes LinkFlags from the correct MS-SHLLINK offset; event bus `Off()` actually removes handlers; silent append errors surfaced to stderr; config + events tests added.

## [0.6.0] - 2026-07-11

### Added
- Sandbox: Docker-based isolated execution environment for suspicious files
  - `sandbox create/run/shell/logs/report/cp/destroy/list` commands
  - Automated capture (filesystem, processes, network)
  - Interactive shell access via `docker exec -it`
  - Auto-destroy on case close with confirmation
  - Pre-built images: ubuntu-22.04, windows (future)
  - Isolated network (no network access)
- Plugin system: manifest-based plugin discovery, `plugin` shell command (list, run, info)
- Case templates: embedded YAML templates, `case -t <template>` flag, `case templates` command
- Evidence deduplication: SHA-256 check on `evidence add` prevents duplicate entries
- Timeline export: `timeline export [path] [--format csv|json]`
- Tool output search: `search --in-output <query>` and `grep <query>`
- **Sleuthkit**: Docker template for disk forensic analysis
  - Full suite: fls, fsstat, icat, mmls, blkcalc, istat, blkls, blkcat, dcat, dstat, ffind, hfind, ifind, ils, img_stat, jcat, jls, sigfind, sstat
  - `sleuthkit <tool> [args]` shell command
- **Registry analysis**: RegRipper + python-registry Docker templates
  - RegRipper: auto-detects hive type, supports all plugins, JSON output
  - python-registry: scriptable registry parsing
  - `regripper <hive> [-p plugin] [--json]` shell command

## [0.5.0] - 2026-06-22

### Added
- Built-in tool: **peinfo** — parse PE headers, imports, sections, compile timestamp (pure Go, `debug/pe`)
- Built-in tool: **elfinfo** — parse ELF headers, sections, symbols (pure Go, `debug/elf`)
- Built-in tool: **rtfscan** — detect RTF exploit objects (objdata, objclass, equation, DDE, hex blobs)
- Built-in tool: **lnkparse** — parse Windows LNK shortcut files (targets, paths, timestamps, flags)
- Built-in tool: **mimetype** — deep MIME type detection with format-specific inspection (PDF, PE, ELF, ZIP, OLE2, PNG, JPEG)
- Tool template: **oletools** — Office document macro and OLE analysis (`olevba`, `oleid`, `oleobj`, etc.) via Docker
- Tool template: **tshark** — CLI packet capture analysis and protocol dissection via Docker

### Fixed
- `build`/`install` now actually build the Docker image: `buildImage` shells out to `docker build --no-cache` and streams output, instead of printing the command and returning "build not implemented"
- Volatility Dockerfile no longer fails to build: dropped the `[full]` extra (which pulled in `leechcorepyc`, an FPGA/live-acquisition driver needing libusb + a C toolchain) in favor of `pip install volatility3`, which installs only the required deps as prebuilt wheels
- Corrected the Volatility image entrypoint to the `vol` console script
- Removed duplicate `tools/volatility/Dockerfile` (old location superseded by `internal/catalog/templates/`)

## [0.4.0] - 2026-06-19

### Added
- Evidence management: `evidence`/`ev` add/tag/verify/list, append-only `evidence.jsonl` (SHA-256), evidence timeline events
- Case export (`export` → tar.gz + manifest.json, `--no-output`, `--json`) and import (`import`, auto-rename on conflict, hash verification)
- Cross-case `search` and `cases --status` filter
- Timeline filtering: `timeline --type` and `--grep`
- Built-in tools: `hexdump`, `entropy`, `decode`
- IOC extraction & tracking: `ioc <file>` / `--from-output`, append-only `ioc.jsonl`
- Starship-style prompt (context line + `❯`, ASCII fallback via `MIMIR_ASCII`/`NO_COLOR`)
- Bordered box-table output for `cases`, `evidence`, `ioc`, `search`, and `timeline` (degrades to plain when piped/narrow)

## [0.3.0] - 2026-06-18

### Added
- Readline-based REPL: arrow-key line editing, persistent command history (`~/.mimir/.history`), Ctrl+R reverse search
- Context-aware Tab completion (commands, tools + built-ins, installable catalog, case names, file paths)
- Append-only per-case timeline (`timeline.jsonl`) with in-memory cache
- Tool runs recorded to case output + timeline (success and failure)
- Notes and case open/close recorded on the timeline
- Native-Go built-in tools: `hash`, `strings`, `file` (no Docker required)
- `timeline` command (chronological view, `-n` tail)
- `OutputCapture.Record` now returns the written output path

### Fixed
- Arrow keys no longer inject escape codes / corrupt the prompt (replaced `bufio.Scanner` input)
- Ctrl+C cancels the current line instead of killing Mimir; Ctrl+D exits cleanly
- Unknown commands passed to the shell no longer rewrap the exit code as `error: exit status N`
- Startup banner shows the correct version

### Changed
- Tab completion no longer does per-keystroke disk work: the embedded tool catalog is parsed once and cached, and case-name completion lists case directories without loading each case's metadata and timeline

## [0.1.0] - 2026-06-17

### Added
- Complete rewrite from Python to Go
- Modular architecture: config/events/cases/tools/shell/plugins
- Interactive REPL with colored prompt (user/case context)
- Case management (create/open/close/list) with JSON persistence
- Tool registry with Docker + local execution stubs
- Output capture to case directories (per-tool files + timeline JSONL)
- Event bus for inter-module communication (pub/sub with panic recovery)
- Plugin system with hooks + PluginAPI
- Built-in commands: help, exit, status, case, cases, tools, run, use, note, clear
- Shell passthrough (stub)
- Makefile for build/test/lint/run
- 17 tests passing across cases, events, tools packages
- Single static binary output

### Changed
- Rewritten from Python to Go for performance and single-binary distribution

### Removed
- Python v0.4 codebase (archived on `archive-python-v0.4` branch)
- CTI/threat intel API integrations (MalwareBazaar, AbuseIPDB, URLhaus)
- prompt_toolkit dependency (replaced with stdlib bufio.Scanner)

[0.1.0]: https://github.com/yusif-v/Mimir/releases/tag/v0.1.0
[0.3.0]: https://github.com/yusif-v/Mimir/compare/v0.1.0...v0.3.0
[0.4.0]: https://github.com/yusif-v/Mimir/compare/v0.3.0...v0.4.0
[0.5.0]: https://github.com/yusif-v/Mimir/compare/v0.4.0...v0.5.0
[0.6.0]: https://github.com/yusif-v/Mimir/compare/v0.5.0...v0.6.0
[0.8.2]: https://github.com/yusif-v/Mimir/compare/v0.8.1...v0.8.2
[0.8.3]: https://github.com/yusif-v/Mimir/compare/v0.8.2...v0.8.3
[0.8.4]: https://github.com/yusif-v/Mimir/compare/v0.8.3...v0.8.4
[Unreleased]: https://github.com/yusif-v/Mimir/compare/v0.8.4...HEAD
