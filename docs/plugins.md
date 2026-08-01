# Plugins

Plugins extend Mimir with external programs or event-driven behavior. They live
in `~/.mimir/plugins/` (one directory per plugin) and are discovered at
startup.

## Commands

| Command | Description |
|---|---|
| `plugin list` | List discovered plugins |
| `plugin info <name>` | Show plugin metadata (name, description, version, enabled) |
| `plugin run <name> [args...]` | Execute the plugin's entrypoint |

`plugin run` invokes the entrypoint binary with the remaining arguments,
streaming stdout/stderr.

## Layout

```
~/.mimir/plugins/<name>/
├── plugin.toml      # manifest (required)
└── <entrypoint>     # executable binary/script
```

### Manifest (`plugin.toml`)

```toml
name        = "my-plugin"
description = "Does a thing"
version     = "1.0.0"
entrypoint  = "my-plugin"   # path relative to the plugin dir
enabled     = true
```

`name` is required; malformed manifests are skipped with a startup warning.

## Event hooks

Plugins can subscribe to Mimir's internal event bus (exposed via the plugin
`API`) and register named hooks. Built-in event names:

| Event | Fired when |
|---|---|
| `case.created` | A case is created |
| `case.opened` | A case is opened |
| `case.closed` | A case is closed |
| `tool.started` | A tool begins running |
| `tool.finished` | A tool completes |
| `tool.error` | A tool fails |
| `output.captured` | Tool output is captured to a case |
| `command.run` | A shell command is executed |

The API supports `On`/`Off` (subscription with a returned ID) and
`RegisterHook`/`FireHook` for named lifecycle hooks.
