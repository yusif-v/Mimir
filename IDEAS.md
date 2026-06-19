# Mimir — Ideas & Future Concepts

A living document for ideas that don't belong to a specific version yet.
Some may land in v0.5, v0.6, or later. Some may never ship. That's fine —
the point is to capture them before they evaporate.

## How This File Works

- Each idea has a **status**: `raw` (just a thought), `explored` (discussed a bit),
  `backlog` (agreed it's worth doing), or `assigned` (mapped to a version).
- Ideas get **promoted** to PLAN.md when they're assigned to a version.
- Ideas get **removed** here once they ship or get rejected.
- No commitment. This is a brain dump with light structure.

---

## Ideas

### Remote / Multi-User

- **Collaborative case sessions** — multiple analysts on the same case, real-time
  timeline updates. Think Google Docs for DFIR. Probably needs a server component.
  Status: `raw`

- **Mimir server mode** — headless Mimir instance exposing a REST/gRPC API so
  other tools (or a web UI) can drive it. Cases become a service, not just files.
  Status: `raw`

- **Case sharing via git** — instead of tar.gz export/import, push a case to a
  git repo. Built-in audit trail, diffing, branching for parallel investigation
  theories. Status: `raw`

### AI / LLM

- **Auto-triage on case open** — feed the initial evidence list to an LLM and
  get a suggested investigation plan (which tools to run first, what to look for).
  Status: `raw`

- **Timeline summarization** — "summarize the last 2 hours of investigation"
  or "what did we find regarding network IOCs?" via natural language.
  Status: `raw`

- **Tool output interpretation** — after running `volatility` or `yara`,
  pass the output to an LLM for plain-English findings. Flag anomalies.
  Status: `raw`

- **Anomaly detection on timelines** — statistical/ML analysis of tool outputs
  and events to surface "this looks unusual" moments the analyst might miss.
  Status: `raw`

### Sandbox

- **Per-case Docker sandbox** — opt-in isolated container per case for running
  tools and ad-hoc commands. Not all cases need it — only the nasty ones
  (malware, APT). Zero behavior change for normal cases.
  Commands: `sandbox start|stop|exec|shell|status|destroy`.
  Case metadata tracks sandbox state (container ID, image, started-at).
  Mounts case evidence/ (ro) and output/ (rw) inside the container.
  Configurable image, network policy (`--network none` default), and
  resource limits (CPU/memory).
  Status: `backlog`

### Evidence & Analysis

- **Evidence deduplication** — across cases, identify when the same file/sample
  has been seen before. Link cases by shared evidence. Status: `raw`

- **Sandboxed auto-analysis** — drop a sample in and Mimir orchestrates a
  pipeline: hash → YARA → strings → entropy → sandbox → report. One command.
  Status: `raw`

- **Memory forensics integration** — deeper Volatility/MemProcFS integration
  with profile auto-detection and common plugin presets. Status: `raw`

- **PCAP analysis tools** — built-in or Docker-backed network capture analysis
  (tshark, zeek). Extract IOCs from traffic. Status: `raw`

- **Email analysis** — parse .eml/.mbox, extract headers, attachments, IOCs.
  Status: `raw`

- **Timeline correlation** — super-timeline mode that merges evidence timestamps
  (filesystem MFT, browser history, event logs) into a single chronological view.
  Status: `raw`

### Integrations

- **MISP integration** — push/pull IOCs to/from MISP instances. Case export as
  MISP event. Status: `raw`

- **TheHive / Cortex integration** — create cases in TheHive from Mimir,
  run Cortex analyzers. Status: `raw`

- **Yggdrasil integration** — push case data, timelines, IOCs, and evidence
  metadata into Yggdrasil's knowledge graph. Query from Mimir: "show me all
  cases involving this hash" or "map relationships between these IOCs."
  Yggdrasil becomes the central hub connecting Mimir, Heimdall, and Horus.
  Status: `raw`

- **Heimdall alert ingestion** — pull alerts from Heimdall (SIEM) and auto-create
  cases in Mimir. Close the loop: detection → investigation → response.
  Status: `raw`

- **EDR integration** — trigger containment actions (isolate host, kill process)
  from Mimir via EDR APIs. Status: `raw`

### Output & Reporting

- **Markdown report generation** — compile a case into a structured Markdown
  report: executive summary, timeline, evidence, IOCs, findings. Ready for
  client delivery. Status: `raw`

- **PDF/HTML export** — same as above but rendered. Status: `assigned` to v0.6
  (already in PLAN.md)

- **Case dashboard** — Bubble Tea TUI dashboard showing case stats, recent
  events, evidence summary at a glance. Status: `assigned` to v0.6
  (already in PLAN.md)

### Platform

- **Web UI** — browser-based interface for Mimir. Browse cases, view timelines,
  run tools. Useful for less CLI-comfortable team members. Status: `raw`

- **Mobile companion** — view case status, timeline, IOCs on mobile. Read-only.
  Status: `raw`

- **Slack/Teams bot** — query Mimir from chat: `/mimir status incident-42` or
  `/mimir timeline incident-42 -n 5`. Status: `raw`

- **Webhook triggers** — external systems can trigger Mimir actions via webhooks
  (e.g., "new alert → create case → run initial triage"). Status: `raw`

### Developer / Plugin Ecosystem

- **Plugin marketplace** — discover, install, rate community plugins.
  Status: `assigned` to v0.6 (already in PLAN.md)

- **Python plugin SDK** — allow plugins written in Python via subprocess/IPC.
  Lowers barrier for analysts who don't know Go. Status: `raw`

- **Tool template registry** — community-contributed `mimir.toml` templates
  for new tools. Search and install from the REPL. Status: `raw`

### Quality of Life

- **Case templates** — pre-defined investigation workflows (phishing, ransomware,
  insider threat) that auto-populate tool suggestions and checklists.
  Status: `raw`

- **Bulk operations** — run a tool against all evidence files in a case at once.
  Status: `raw`

- **Favorites / bookmarks** — bookmark specific timeline events or evidence
  files for quick reference. Status: `raw`

- **Case archiving** — compress old cases, move to cold storage, restore on
  demand. Status: `raw`

- **Config profiles** — switch between configurations (e.g., different Docker
  registries, tool paths, API keys) with `mimir --profile corp` vs
  `mimir --profile lab`. Status: `raw`

- **Undo / event correction** — append-only means no deletes, but allow
  appending correction/retraction events to the timeline. Status: `raw`

---

## Ecosystem Projects

### Yggdrasil — Knowledge Management Platform

Local-first, graph-based knowledge management. Think: between GitHub and
Obsidian. The central hub that connects all ecosystem projects.

_This section is for Yggdrasil-specific ideas, not Mimir features._

- **Knowledge graph** — bi-directional links between notes, auto-generated
  graph view, backlinks. Core Obsidian-like feature. Status: `raw`

- **Git-backed collaboration** — push/pull/share knowledge bases. PR-like
  merge workflow for contributions. Version history built in.
  Status: `raw`

- **Mimir case ingestion** — import Mimir case exports as knowledge nodes.
  Link investigations to threat intel, IOCs, reports. Status: `raw`

- **Heimdall alert feed** — ingest alerts as timestamped nodes in the graph.
  Correlate with Mimir cases and threat intel. Status: `raw`

- **Horus scan result import** — scan results become queryable knowledge.
  Track IOC sightings across time and sources. Status: `raw`

- **Graph queries** — query language for the knowledge graph: "find all paths
  between these nodes," "what's connected to this IP across all sources."
  Status: `raw`

- **Local-first + sync** — files are local markdown by default. Optional sync
  for teams. Status: `raw`

---

## Rejected / Won't Do

_An idea goes here when it's discussed and explicitly dropped, so we don't
revisit it._

- (empty — nothing rejected yet)
