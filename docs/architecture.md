# Mimir — Architecture

## Design Principles

1. **Modular** — Each package is self-contained with a clear interface
2. **Event-driven** — Components communicate via events, not direct calls
3. **Storage-agnostic** — Cases use an interface; swap filesystem for Yggdrasil later
4. **Plugin-first** — Even v0.1 supports hooks for extensions

## Package Layout

```
cmd/mimir/          # Binary entry point
internal/
  config/           # Config loading (YAML + env)
  events/           # Event bus + event name constants
  cases/            # Case model, CRUD, storage interface
  tools/            # Registry, runner (local + Docker), templates, output
  shell/            # App, REPL, prompt, commands, completion
  plugins/          # Manager, hooks, public API
pkg/
  api/              # Public API surface for plugins
tools/              # Bundled tool templates (Dockerfile + mimir.toml)
docs/               # Documentation
```

## Data Flow

```
User input → REPL → Command dispatch → Tool runner → Output capture → Case storage
                                  ↕                                  ↕
                            Event bus ←←←←←←←←←←←←←←←←←←←←←←←←←
```

## Case Storage Structure

```
~/Mimir/Investigations/
  <case-name>/
    case.json          # Case metadata
    evidence/          # Input files (read-only mounts)
    output/            # Tool output files
    notes/             # Analyst notes
```
