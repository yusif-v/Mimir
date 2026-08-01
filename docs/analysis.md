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

## Sleuthkit (disk)

```
sleuthkit <tool> [args...]
```

Runs a TSK command inside `dfir-sleuthkit:latest` (build with `build
sleuthkit`). The case `evidence/` directory is mounted read-only at `/evidence`
and output is written to `<case>/output/sleuthkit/`.

Allowed tools (whitelist):

`fls`, `fsstat`, `icat`, `mmls`, `blkcalc`, `istat`, `blkls`, `blkcat`,
`dcat`, `dstat`, `ffind`, `hfind`, `ifind`, `ils`, `img_stat`, `jcat`,
`jls`, `sigfind`, `sstat`

Example:

```
sleuthkit mmls evidence/disk.img
sleuthkit fls -r -o 2048 evidence/disk.img
sleuthkit icat evidence/disk.img 128-128-1 > output/recovered.bin
```

## RegRipper (registry)

```
regripper <hive-file> [-p <plugin>] [--json]
```

Runs RegRipper inside `dfir-regripper:latest` (build with `build regripper`).

- `<hive-file>` — path to a registry hive (e.g. `NTUSER.DAT`, `SYSTEM`,
  `SAM`), typically added as evidence first.
- `-p <plugin>` — run a specific plugin (default: auto-detect the hive type).
- `--json` — structured JSON output.
- Output is saved to `<case>/output/regripper/` and captured like any tool run.

Example:

```
evidence add NTUSER.DAT
regripper evidence/NTUSER.DAT -p userassist
regripper evidence/SYSTEM --json
```
