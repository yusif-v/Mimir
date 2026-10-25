# Tools: Builtins & Catalog

Mimir runs analysis through three kinds of tools, all invoked the same way:

```
tool run <name> [args...]
```

Every `tool run` captures stdout into `<case>/output/`, records a `tool_run`
timeline event (tool, args, return code, duration, output file), and
automatically scans the output for IOCs.

## Tool kinds

| Kind | Where it runs | Examples |
|---|---|---|
| **Builtin** | Native Go, in-process | `hash`, `strings`, `entropy`, `peinfo`, ... |
| **Docker** | Container from a built image | `volatility`, `yara`, `zeek`, `tshark` |
| **Local** | Executable on the host | any `cmd` defined in a tool manifest |

`tool list` lists everything: builtins, installed tools (with build status),
and installable catalog entries.

## Built-in tools

| Tool | Description |
|---|---|
| `hash` | MD5 / SHA1 / SHA-256 digests of a file |
| `strings` | Printable ASCII runs in a file (`-n <min length>`) |
| `file` | Identify file type by magic bytes |
| `hexdump` | Canonical hex + ASCII dump |
| `entropy` | Shannon entropy in bits/byte; `--packed` / `--encrypted` hints |
| `decode` | Decode base64 / hex / url input (`--base64`, `--hex`, `--url`, else auto) |
| `peinfo` | Parse PE headers, imports, sections, compile timestamp |
| `elfinfo` | Parse ELF headers, sections, symbols |
| `rtfscan` | Detect RTF exploit objects (objdata, objclass, equation) |
| `lnkparse` | Parse Windows LNK shortcut files |
| `mimetype` | Deep MIME-type detection beyond magic bytes |
| `unzip` | List/extract ZIP and 7-Zip archives (supports AES / password-protected) |

## The catalog

The catalog (`internal/catalog/templates/`) is a set of installable tool
manifests shipped with Mimir:

`bulk_extractor`, `capa`, `chainsaw`, `cuckoo`, `oletools`, `python-registry`,
`regripper`, `sleuthkit`, `tshark`, `volatility`, `yara`, `zeek`

### Installing

| Command | Description |
|---|---|
| `tool install <name>` | Copy the catalog template into `~/.mimir/tools/<name>/` |
| `tool build <name>` | `docker build --no-cache` the tool's Docker image |
| `tool info <name>` | Show tool details (description, status, resolved command or image) |

If a tool is already installed, `tool install` fails with a hint to use
`tool build` instead (`use 'tool build <name>' to rebuild`).

### Tool manifest (`mimir.toml`)

Each catalog template ships a `mimir.toml` describing the tool:

```toml
[tool]
name        = "volatility"
description = "Memory forensics with Volatility 3"
category    = "forensics"
tags        = ["memory", "windows", "linux"]

[docker]
image    = "dfir-volatility:latest"
workdir  = "/output"

[docker.volumes]
"<case>/evidence" = "/evidence:ro"
"<case>/output"   = "/output:rw"

# A [local] tool instead declares:
# [local]
# cmd = "yara"
```

- `[tool]` — metadata: name, description, category, tags.
- `[docker]` — `image`, optional `entrypoint`, `workdir`, `version`, `volumes`,
  `ports`. Volumes commonly map case `evidence/` read-only and `output/`
  read-write.
- `[local]` — run a host binary by name instead of a container.

## Execution limits

To keep output bounded, Mimir applies:

| Limit | Value |
|---|---|
| Local tool timeout | 1 hour |
| Docker tool timeout | 30 minutes |
| Output cap (per stream) | 8 MiB (silently truncated) |

## The `memory` shortcut

`memory` is a special first-class command; see
[Memory, disk & registry analysis](analysis.md).
