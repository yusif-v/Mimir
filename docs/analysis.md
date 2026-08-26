# Memory, Disk & Registry Analysis

Three dedicated commands wrap Docker images so you get first-class analysis
without remembering container plumbing. All run output is captured to the case,
recorded on the timeline, and scanned for IOCs automatically.

## Volatility (memory)

```
volatility <plugin> [-f <dump>] [--offline] [args...]
mem <plugin> [-f <dump>] [--offline] [args...]        # alias
```

Runs Volatility 3 inside `dfir-volatility:latest` (built from the catalog
template with `build volatility`).

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
volatility windows.info
volatility windows.pstree
volatility windows.cmdline.CmdLine --offline
```

## Disk (SleuthKit)

```
disk <tool> [-i <image>] [args...]
sleuthkit <tool> [args...]    (alias)
```

First-class disk-forensics command — the counterpart of `volatility`. Runs a
TSK command inside `dfir-sleuthkit:latest` (build with `build sleuthkit`).

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
regripper [hive] [-p <plugin>] [--json]
```

Runs RegRipper inside `dfir-regripper:latest` (build with `build regripper`).
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
regripper -p userassist
regripper NTUSER.DAT --json
regripper evidence/SYSTEM --json
```
