# Changelog

All notable changes to Mimir will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.0] - 2026-07-11

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
[Unreleased]: https://github.com/yusif-v/Mimir/compare/v0.6.0...HEAD
