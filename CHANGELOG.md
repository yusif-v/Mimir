# Changelog

All notable changes to Mimir will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned
- Local subprocess execution (os/exec)
- Docker-based tool sandboxing
- Tool template system (mimir.toml)
- Built-in DFIR tools (hash, file info, strings, timeline)
- Case timeline view (unified tool runs + notes)
- Export case to JSON archive

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
[Unreleased]: https://github.com/yusif-v/Mimir/compare/v0.1.0...HEAD
