# Mimir

DFIR shell and tool multiplexer. Written in Go.

## Quick Start

```bash
go build ./cmd/mimir
./mimir
```

## Architecture

- `internal/shell/` — REPL, prompt, commands
- `internal/cases/` — Case model, manager, storage
- `internal/tools/` — Registry, runner, templates, output capture
- `internal/plugins/` — Plugin system
- `internal/config/` — Configuration management
- `internal/events/` — Event bus

## License

MIT
