# Mimir — Project Plan

## Vision

A DFIR shell and tool multiplexer. Run Mimir, open a case, then use any tool
through the shell. All outputs captured, organized by case, ready for analysis.

Tyr (red team) and Mimir (blue team) are the two halves of the ecosystem.
Yggdrasil is the knowledge management platform — local-first, graph-based,
collaboration-capable (think: between GitHub and Obsidian). Mimir, Heimdall,
and Horus all feed data into it. Heimdall is the monitoring/SIEM.

## Language Choice

**Go** — chosen for:
- Single static binary, no runtime dependencies
- Best-in-class Docker SDK (Docker is written in Go)
- Excellent cross-compilation
- Fast enough for a shell that orchestrates external tools
- Built-in concurrency and plugin system

## Current State — v0.3.0 (released 2026-06-18)

- Interactive readline REPL (history, line editing, Tab completion), case management
- Docker + local + native built-in tool execution; run output captured to cases
- Append-only per-case timeline; event bus; plugin system
- Single static binary
- All v0.2 and v0.3 work shipped together in the v0.3.0 release (v0.2.0 was never tagged separately)

## v0.2 — Tool Execution (done, shipped in v0.3.0)

**Goal:** Actually run tools, not just register them.

- [x] Local subprocess execution via `os/exec`
- [x] Docker-based tool sandboxing with evidence mounting
- [x] Tool template system (`mimir.toml` format)
- [x] Tool auto-discovery from `tools/` directory
- [x] Built-in tools: `hash`, `file`, `strings`
- [x] Append-only per-case timeline with in-memory cache
- [x] `timeline` command (chronological view with `-n` tail)
- [x] Shell passthrough for arbitrary commands

### Tool Template Format (mimir.toml)

```toml
[tool]
name = "volatility"
description = "Memory forensics with Volatility 3"
category = "forensics"
tags = ["memory", "windows", "linux"]

[docker]
image = "dfir-volatility:latest"

[[docker.volumes]]
host = "/evidence"
container = "/evidence"
mode = "ro"

[[docker.volumes]]
host = "/output"
container = "/output"
mode = "rw"
```

## v0.3 — REPL Upgrade & Case Timeline (done, released 2026-06-18)

**Goal:** Professional readline-based REPL with context-aware completion; unified view of everything that happened in a case.

*Notes:*
- *Case timeline (`timeline.jsonl` + `timeline` command) landed early as part of v0.2.*
- *REPL upgrade (readline, history, tab completion, arrow-key fixes) completed in v0.3.*

### REPL Upgrade (v0.3.0)
- [x] Readline-based REPL with arrow-key line editing
- [x] Persistent command history (`~/.mimir/.history`)
- [x] Ctrl+R reverse search
- [x] Context-aware Tab completion (commands, tools, catalog, cases, files)
- [x] Arrow key escape-code corruption fixed
- [x] Ctrl+C cancels line; Ctrl+D exits cleanly
- [x] Unknown commands no longer rewrap exit codes
- [x] Version display fix

### Case Timeline (v0.2/v0.3)
- [x] Timeline view: all tool runs + notes in chronological order (completed in v0.2)

## v0.4 — Case Management, Investigation & UX (planned)

**Goal:** Manage evidence, extract IOCs, share cases (export/import), search
across investigations, and overhaul the look (Starship-style prompt + box
tables). Design: `docs/superpowers/specs/2026-06-18-v0.4-case-management-design.md`.

*Investigation / case data*
- [ ] Evidence management — `evidence`/`ev add/tag/verify`, append-only `evidence.jsonl` (sha256, tags, source), `evidence_added`/`evidence_tagged` timeline events
- [ ] Export — `export` → `<case>.tar.gz` (+ `manifest.json`), `--no-output`, `--json` document
- [ ] Cross-case search + filter — `search <query>`; `cases --status open|closed`
- [ ] Timeline filtering — `timeline --type <t>` / `--grep <substr>` (composes with `-n`)
- [ ] More built-in tools — `hexdump`, `entropy`, `decode`
- [ ] IOC extraction & tracking — `ioc <file>` / `--from-output`, append-only `ioc.jsonl`, `ioc_extracted` event
- [ ] Case import — `import <archive>` (inverse of export; auto-rename on conflict; verifies hashes)

*UX / presentation*
- [ ] Starship-style prompt — segmented context line + `❯`, nerd-font icons/color, ASCII fallback (`NO_COLOR`/`MIMIR_ASCII`)
- [ ] Bordered box-table output via a native `internal/ui` render helper (degrades to plain when piped/narrow)

## v0.5 — AI Integration

**Goal:** LLM-assisted investigation.

- [ ] LLM-powered case analysis (summarize findings)
- [ ] Automated timeline generation from tool outputs
- [ ] Investigation recommendations ("based on these findings, try...")
- [ ] Natural language case querying

## v0.6 — Platform

**Goal:** Beyond the terminal.

- [ ] Bubble Tea TUI (rich terminal UI)
- [ ] Report generation (PDF/HTML)
- [ ] Yggdrasil integration (graph database backend)
- [ ] Tool registry/marketplace

## Ecosystem

| Project | Role | Language | Status |
|---------|------|----------|--------|
| **Mimir** | Blue Team DFIR shell | Go | v0.3.0 released |
| **Tyr** | Red Team tool (reserved) | — | Not started |
| **Yggdrasil** | Knowledge management platform (GitHub + Obsidian) | — | Not started |
| **Heimdrall** | SIEM / monitoring | — | Not started |

## Notes / Deferred

- **Event-bus handler unsubscribe** (`internal/events/events.go:38`) — Go cannot compare function values, so the `Off()` method is a broken no-op. Real unsubscribe needs handler IDs. Deferred to v0.4+ robustness pass.
- **Evidence management workflow** (add, tag, hash), case export/search, and timeline type-filtering were not included in v0.3.0 — open candidates for a future release.

## License

MIT
