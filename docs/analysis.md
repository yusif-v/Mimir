# Memory, Disk & Registry Analysis

Three dedicated commands wrap Docker images so you get first-class analysis
without remembering container plumbing. All run output is captured to the case,
recorded on the timeline, and scanned for IOCs automatically.

## Memory (Volatility)

```
memory <plugin> [-f <dump>] [--offline] [args...]
```

Runs Volatility 3 inside `dfir-volatility:latest` (built from the catalog
template with `tool build volatility`).

- **Dump selection**: `-f <dump>` (a case-relative or absolute path, also
  accepted as `-f=<dump>`) picks the image explicitly. Without `-f`, the most
  recently added evidence tagged `memory-dump` is used; otherwise the first
  evidence file in the case. The dump is bind-mounted read-only as
  `/evidence/<name>`.
- **`--offline`** disables online symbol server lookups (offline analysis).
- Output is printed, saved to `<case>/output/`, appended to the timeline, and
  auto-scanned for IOCs.
- Plugin output feeds `volatilityHints`: memory commands (`cmdscan`, `cmdline`,
  `consoles`) and credential material (`hashdump`) get highlighted.

Example:

```
evidence add ram.bin --tag memory-dump
memory windows.info
memory windows.pstree
memory windows.cmdline.CmdLine --offline
```

## Disk (SleuthKit)

```
disk <tool> [-i <image>] [args...]
```

First-class disk-forensics command — the counterpart of `memory`. Runs a
TSK command inside `dfir-sleuthkit:latest` (build with `tool build sleuthkit`).

The image is resolved from `-i` (absolute, or relative to the case evidence
dir; substring match supported), or auto-selected from evidence tagged
`disk-image`, else the most recently added evidence. Only that single image is
mounted read-only into the container.

Output is printed, captured to the timeline via the shared capture path, and
scanned for IOCs plus tool-specific hints:
- `mmls` -> `partition` indicators (start/end/length + description)
- `fls` -> `deleted_file` indicators (entries marked with `*`)

Allowed tools (whitelist):

`fls`, `fsstat`, `icat`, `mmls`, `blkcalc`, `istat`, `blkls`, `blkcat`,
`dcat`, `dstat`, `ffind`, `hfind`, `ifind`, `ils`, `img_stat`, `jcat`,
`jls`, `sigfind`, `sstat`

Example:

```
evidence add disk.img --tag disk-image
disk mmls
disk fls -r -o 2048 -i disk.img
disk icat -i disk.img 128-128-1 > output/recovered.bin   # shell redirect outside disk
```

## RegRipper (registry)

```
registry [hive] [-p <plugin>] [--json]
```

Runs RegRipper inside `dfir-regripper:latest` (build with `tool build regripper`).
The hive is resolved like `disk`: positional name (absolute / relative to case
evidence / substring), auto-selected from evidence tagged `registry-hive`, else
the most recently added evidence. Only the hive is mounted read-only into the
container.

Output is printed, captured to the timeline via the shared capture path, and
scanned for IOCs plus registry-specific hints:
- `usb_device` — USBSTOR device keys (USB history)
- `winpath` — referenced `.exe`/`.dll`/script paths (autoruns, services)
- `autostart` — Run/RunOnce/RunOnceEx key lines

- `-p <plugin>` — run a specific plugin (default: auto-detect the hive type).
- `--json` — structured JSON output.

Example:

```
evidence add NTUSER.DAT --tag registry-hive
registry -p userassist
registry NTUSER.DAT --json
registry evidence/SYSTEM --json
```

## Static Analysis (tool analyze)

```
tool analyze <path> [--rules <path>]
```

Auto-detects the sample's file type by magic bytes (PE, ELF, legacy OLE
Office docs, OOXML Office docs) and runs the applicable static tools:

- **PE/ELF**: `yara`, `capa`
- **OLE/OOXML** (Office documents): `yara`, `oletools` (`olevba`)
- **unknown**: `yara` only

The sample is registered as case evidence first. Each tool's output is
recorded to the timeline and scanned for IOCs exactly like a manual `tool run`.
capa's ATT&CK-mapped capabilities are recorded as `capability` IOCs (e.g.
`T1055 Process Injection`) instead of raw text.

`yara` needs a rules file. All tool runs happen in Docker containers that
only have `<case>/evidence/` and `<case>/output/` mounted, so the rules file
must live under `<case>/evidence/` for yara to be able to read it:

- Default: place `rules.yar` in `<case>/evidence/`.
- `--rules <path>`: if `<path>` is already inside `<case>/evidence/`, it's
  used as-is; otherwise it's copied into `<case>/evidence/` under its
  basename first, then referenced from there.

Without a resolvable rules file, yara is skipped with a warning.

A tool that isn't installed is skipped with a warning (`tool install <name>` to
add it) — `tool analyze` always finishes and reports what it could gather.

Example:

```
tool install capa
tool build capa
tool analyze suspicious.exe
```

`tool analyze` registers the sample as case evidence automatically — no need to
run `evidence add` first.
