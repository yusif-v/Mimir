# Mimir

DFIR shell and tool multiplexer. Written in Go.

## Quick Start

```bash
go build -o mimir ./cmd/mimir
./mimir
```

## Config

Mimir stores everything under `~/.mimir/`:

```
~/.mimir/config.yaml       # Config file
~/.mimir/investigations/   # Cases
~/.mimir/tools/            # Tool templates
~/.mimir/plugins/          # Plugins
~/.mimir/cache/            # Cache
~/.mimir/logs/             # Logs
~/.mimir/logs/mimir.log    # Main log file
~/.mimir/.history          # Command history
```

Generate a default config:

```bash
./mimir config
# or specify a path:
./mimir config ~/.mimir/config.yaml
```

### Config File (`~/.mimir/config.yaml`)

```yaml
log_level: INFO
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `MIMIR_HOME` | Override base directory (default: `~/.mimir`) |
| `MIMIR_CASES_PATH` | Override cases directory |
| `MIMIR_LOG_LEVEL` | Log level (DEBUG, INFO, WARN, ERROR) |

## License

MIT
