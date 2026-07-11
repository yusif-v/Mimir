# Mimir — Tooling Gap Analysis

Generated: 2026-07-11
Scope: builtins (in-shell commands) + Docker catalog templates + the `sandbox`
runner, vs. what a real DFIR case actually needs. Goal: find what we have,
what's weak, and what's missing — so the next build targets real gaps, not
features we already have.

## 1. What ships today (verified inventory)

### Builtin commands (run in-shell, no Docker)
Source: `internal/builtins/builtins.go` + `internal/shell/shell.go` dispatch.

| Command | What it does |
|---------|---------------|
| `case` / `cases` | create / open / list / close cases; evidence dir per case |
| `evidence add` | copy file into `case/evidence/`, hash, track, auto timeline |
| `evidence tag` | tag evidence (`memory-dump`, `malware`, …) |
| `timeline` | append events; `build --auto` → phase-labeled narrative |
| `ioc` | extract IP/domain/url/hash/email from text; list tracked IOCs |
| `report` | full incident report (md/pdf), embeds tool output as Findings |
| `ai` / `models` / `use` / `summarize` | modular provider registry + analysis |
| `analyze` / `ask` / `explain` | AI-assisted analysis (provider-backed) |
| `search` / `grep` / `nrep` | in-case content search |
| `note` | analyst notes (→ timeline + report) |
| `plugin` / `templates` / `theme` / `config` / `set` / `user` | config + extensibility |
| `brand` / `verify` | provenance / integrity checks |
| `import` / `export` | case I/O |
| `sandbox` | manage sandboxes: create/run/shell/logs/cp/destroy/list |
| `install` / `build` | install a catalog template + build its Docker image |
| `tools` | list available catalog templates |

### File/artifact builtins (no Docker, direct binary wrappers)
`hash`, `strings`, `file`, `hexdump`, `entropy`, `decode`,
`peinfo`/`peMachine`/`peTimestamp`/…, `elfinfo`/…, `rtfscan`,
`lnkparse` (LNK parser: filetime, attrs, linkflags, drive type),
`mimetype`, `inspectpdf`/`inspectzip`/`inspectpe`/`inspectpng`/`inspectjpeg`.

### Docker catalog templates (`install` + `build` → `sandbox run -- <tool>`)
Each wraps a real DFIR binary. Except `volatility`/`regripper`/`sleuthkit`
(which have dedicated commands), the rest run as `sandbox run <id> -- <bin> …`.

| Template | Binary | Domain |
|----------|--------|--------|
| `volatility` | vol (Volatility 3) | RAM / memory forensics |
| `regripper` | rip.pl | Windows registry hives |
| `sleuthkit` | fls (only — see gap) | disk filesystem |
| `capa` | capa | malware capability detection |
| `chainsaw` | chainsaw | Windows event-log triage |
| `yara` | yara | signature scanning |
| `zeek` | zeek | network PCAP analysis |
| `tshark` | tshark | packet capture |
| `bulk_extractor` | bulk_extractor | bulk artifact carving |
| `oletools` | olevba | Office macro analysis |
| `python-registry` | python -m registry | registry parsing (lib) |

## 2. Coverage matrix — case need vs. Mimir

| DFIR need | Mimir | Notes |
|------------|--------|-------|
| Manage case + chain-of-custody | ✅ case/evidence | hashes tracked |
| RAM / memory analysis | ✅ volatility (v0.7.1) | dedicated cmd, auto-mount, IOC |
| Registry analysis | ✅ regripper | dedicated cmd, auto-detect hive |
| Filesystem (disk) analysis | ⚠️ sleuthkit | **only fls** — icat/istat/mmls/blkcalc NOT exposed |
| Malware static (PE/ELF) | ✅ builtins + capa | capa via sandbox |
| Office macro (OLE) | ✅ oletools | sandbox |
| Signature scan | ✅ yara | sandbox |
| Event-log triage | ✅ chainsaw | sandbox |
| Network PCAP | ✅ zeek + tshark | sandbox |
| Bulk carving | ✅ bulk_extractor | sandbox |
| Timeline correlation | ✅ timeline build --auto | phase-labeled narrative |
| IOC extraction | ✅ ioc + volatility hints | commands + NTLM |
| Report generation | ✅ report (Findings section) | embeds tool output |
| AI summarization | ✅ ai registry | provider-agnostic |
| Chain-of-custody export | ⚠️ export | format limited (see gap) |

## 3. Gaps — what's missing or weak

### HIGH (blocks a real case end-to-end)
1. **SleuthKit only exposes `fls`.** The Dockerfile ENTRYPOINT is
   `["fls"]`, so `icat`, `istat`, `mmls`, `blkcalc`, `fsstat`, `ifind`
   can't be run through the dedicated `sleuthkit` command. Either broaden the
   ENTRYPOINT handling (run any sleuthkit binary) or add per-tool commands.
   Right now disk forensics is half-wired.
2. **No `timeline` from disk/network tools.** `tool_run` events land for
   `volatility` (v0.7.2) but the generic `sandbox run -- <tool>` path
   does NOT append a `tool_run` timeline event or auto-IOC — only the
   dedicated `volatility` command does. So capa/chainsaw/zeek runs don't show
   in the narrative or Findings unless manually noted. **The #4/#6 fix was
   volatility-only; the generic sandbox path still doesn't capture output.**
3. **`export` provenance is thin.** No DFXML/JSON-sidecar or hash
   manifest in the export; chain-of-custody is implicit (hashes in evidence
   records) but not a formal export artifact.

### MEDIUM (makes it a real tool, not a demo)
4. **No artifact-type auto-routing.** `evidence add` takes any file; the
   analyst must remember `volatility` for dumps, `regripper` for hives,
   `chainsaw` for EVTX. A `evidence classify` step (by magic/extension)
   that suggests the right tool would cut friction.
5. **No timeline from `sandbox run` output.** Same as #2 — generic tools
   don't feed the timeline/report Findings.
6. **`report` doesn't embed sandbox-tool output.** Only `tool_run`
   (volatility) events render in Findings. capa/chainsaw/zeek output is
   lost from the report unless copied by hand.
7. **No diffing / comparison.** `impfuzzy`/`ssdeep`/timeline diff
   between two memory snaps or two runs isn't supported.
8. **Limited network enrichment.** zeek/tshark run, but no auto-IOC
   from PCAP (conn logs → IPs) the way volatility hints work.

### LOW / POLISH
9. `install` doesn't auto-build (you must `build <tool>` after).
10. No offline/airgapped symbol cache mode for volatility.
11. No tab-completion for tool/plugin names.
12. No `correlate` command that crosses memory + disk + network IOCs
    into one graph.

## 4. Recommended build order (next)

P0 (unblock real cases):
- **G1** SleuthKit: expose icat/istat/mmls/blkcalc/fsstat (broaden
  command or ENTRYPOINT shim).
- **G2/G6** Make `sandbox run -- <tool>` append a `tool_run` timeline
  event + auto-IOC + feed report Findings — generalize the volatility
  capture logic into the sandbox runner so EVERY tool benefits.

P1 (real-tool feel):
- **G4** `evidence classify` → suggests tool per artifact type.
- **G5** timeline from all tool runs (done for volatility; extend to sandbox).
- **G8** network IOC auto-extraction from zeek/tshark.

P2 (maturity):
- **G3** formal `export` provenance (hash manifest + DFXML sidecar).
- **G7** snapshot/run diffing.
- **G9–G12** polish.

> Note: G2/G5/G6 are the same root cause — the dedicated `volatility`
> command duplicates capture logic that belongs in the sandbox runner. Refactor
> the capture into `runtime.RunTarget`/`Exec` so it's uniform.
