*

# Mimir — Project Plan

## Vision

A DFIR shell and tool multiplexer. Run Mimir, open a case, then use any tool
through the shell. All outputs captured, organized by case, ready for analysis.

## v0.1 Goals

- Interactive REPL with colored prompt, tab completion, history
- Case management (create/open/close/list) with JSON persistence
- Tool registry + runner (local subprocess + Docker)
- Output capture to case directories
- Plugin system (hooks)
- Config via YAML + env vars
- Single static binary, cross-platform

## Ecosystem

| Project | Role | Status |
|---------|------|--------|
| **Mimir** | Blue Team DFIR shell | v0.1 in progress |
| **Tyr** | Red Team tool (reserved) | Not started |
| **Yggdrasil** | Database + graph visualizer | Not started |
| **Heimdrall** | SIEM / monitoring | Not started |

## License

MIT
