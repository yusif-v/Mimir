# Mimir — Project Plan

## Vision

A DFIR shell and tool multiplexer. Run Mimir, open a case, then use any tool
through the shell. All outputs captured, organized by case, ready for analysis.

Tyr (red team) and Mimir (blue team) are the two halves of the ecosystem.
Yggdrasil is the shared database/graph layer. Heimdrall is the monitoring/SIEM.

## Language Choice

**Go** — chosen for:
- Single static binary, no runtime dependencies
- Best-in-class Docker SDK (Docker is written in Go)
- Excellent cross-compilation
- Fast enough for a shell that orchestrates external tools
- Built-in concurrency and plugin system

## Current State — v0.1.0

- 17 Go source files, ~1450 lines
- 17 tests passing
- Interactive REPL, case management, tool registry, output capture
- Event bus, plugin system
- Single static binary

## v0.2 — Tool Execution (next)

**Goal:** Actually run tools, not just register them.

- [ ] Local subprocess execution via `os/exec`
- [ ] Docker-based tool sandboxing with evidence mounting
- [ ] Tool template system (`mimir.toml` format)
- [ ] Tool auto-discovery from `tools/` directory
- [ ] Built-in tools: `hash`, `file`, `strings`, `timeline`
- [ ] Shell passthrough for arbitrary commands

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

## v0.3 — Case Timeline

**Goal:** Unified view of everything that happened in a case.

- [ ] Timeline view: all tool runs + notes in chronological order
- [ ] Export case to JSON archive for sharing
- [ ] Case search and filtering
- [ ] Evidence file management (add, tag, hash)

## v0.4 — AI Integration

**Goal:** LLM-assisted investigation.

- [ ] LLM-powered case analysis (summarize findings)
- [ ] Automated timeline generation from tool outputs
- [ ] Investigation recommendations ("based on these findings, try...")
- [ ] Natural language case querying

## v0.5 — Platform

**Goal:** Beyond the terminal.

- [ ] Bubble Tea TUI (rich terminal UI)
- [ ] Report generation (PDF/HTML)
- [ ] Yggdrasil integration (graph database backend)
- [ ] Tool registry/marketplace

## Ecosystem

| Project | Role | Language | Status |
|---------|------|----------|--------|
| **Mimir** | Blue Team DFIR shell | Go | v0.1.0 done |
| **Tyr** | Red Team tool (reserved) | — | Not started |
| **Yggdrasil** | Database + graph visualizer | — | Not started |
| **Heimdrall** | SIEM / monitoring | — | Not started |

## License

MIT
