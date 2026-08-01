# Sandbox

The sandbox manages disposable Docker environments for file analysis — detonate
a sample, inspect filesystem/process activity, and run arbitrary tools inside
the container without touching your host.

```
sandbox <create|run|shell|logs|report|cp|destroy|list> [args...]
```

## Subcommands

| Command | Description |
|---|---|
| `sandbox create [--os <os>] [--image <img>] [--file <path>] [--timeout <sec>]` | Create a sandbox |
| `sandbox run <id>` | Run the preset target file |
| `sandbox run <id> -- <cmd> [args...]` | Run an arbitrary command inside the container |
| `sandbox shell <id>` | Open an interactive shell (Ctrl+D to detach) |
| `sandbox logs <id>` | Show container logs |
| `sandbox report <id>` | Show sandbox metadata (OS, image, status, files, network, timeout) |
| `sandbox cp <file> <id>` | Copy a file into the container |
| `sandbox destroy <id>` | Stop, remove, and forget a sandbox |
| `sandbox list` | List active sandboxes |

## Create flags

| Flag | Default | Description |
|---|---|---|
| `--os` | `ubuntu-22.04` | Guest OS; used to derive the image name |
| `--image` | `mimir-sandbox:<os>` | Container image to run |
| `--file` | *(empty)* | Preset target file to auto-execute on `sandbox run <id>` |
| `--timeout` | `300` | Execution timeout in seconds |

If the container cannot be started, `create` still records the sandbox and
prints a warning (so you can inspect/destroy it).

## Copying files in

`sandbox cp <file> <id>` stages the file under **`/work`** inside the container.
`/tmp` is mounted **noexec**, so anything copied there could never be executed —
`/work` is the exec-capable staging area. Copied files are tracked on the
sandbox record and shown in `sandbox report`.

## Capturing tool runs

`sandbox run <id> -- <tool> [args...]` captures the tool output back into the
open case, exactly like the dedicated `volatility` command: it appends a
`tool_run` timeline event and auto-extracts IOCs from the output. Use it to run
tools that are not Mimir commands — e.g.

```
sandbox create --os ubuntu-22.04 --file sample.exe
sandbox run <id>                        # runs the preset file
sandbox run <id> -- strings /work/sample.exe
```
