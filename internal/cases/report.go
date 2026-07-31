package cases

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ToolRunCommand reconstructs the command line of a tool_run event from its
// payload. Two payload shapes exist today: captureInto records "command"/
// "dump"/"output" inline, while recordRun (CLI run) and the MCP tool_run
// record "tool"/"args"/"output_file". Unifying them keeps the report,
// narrative, archive, and export manifest consistent regardless of which
// path produced the event.
func (ev TimelineEvent) ToolRunCommand() string {
	if cmd, ok := ev.Payload["command"].(string); ok && cmd != "" {
		return cmd
	}
	tool, _ := ev.Payload["tool"].(string)
	if tool == "" {
		return ""
	}
	var args []string
	switch a := ev.Payload["args"].(type) {
	case []string:
		args = a
	case []any:
		for _, v := range a {
			if s, ok := v.(string); ok {
				args = append(args, s)
			}
		}
	}
	if len(args) == 0 {
		return tool
	}
	return tool + " " + strings.Join(args, " ")
}

// ToolRunOutput returns the captured stdout of a tool_run event. It prefers
// the inline "output" payload (captureInto) and otherwise reads the
// "output_file" recorded by recordRun/MCP tool_run (relative to the case
// directory), truncated to match the inline payload's size limit.
func (c *Case) ToolRunOutput(ev TimelineEvent) string {
	if out, ok := ev.Payload["output"].(string); ok && out != "" {
		return out
	}
	rel, _ := ev.Payload["output_file"].(string)
	if rel == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(c.Path, rel))
	if err != nil {
		return ""
	}
	return truncateToolOutput(string(b))
}

// truncateToolOutput caps the rendered tool output so a huge capture (e.g. a
// full disk image listing) does not balloon the report.
func truncateToolOutput(s string) string {
	const max = 2000
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// ReportFormat is the output format for a case report.
type ReportFormat string

const (
	ReportMarkdown ReportFormat = "md"
	ReportPDF      ReportFormat = "pdf"
)

// ReportOptions controls report generation.
type ReportOptions struct {
	Format ReportFormat
	Path   string // output path; empty => <case>/output/report.<ext>
	NoAI   bool   // omit the AI executive summary section
}

// BuildReport assembles a full incident report for the case as a Markdown
// string. AI summarization is performed by the caller (the shell) which
// injects the summary via the summary parameter, keeping this function free of
// any AI dependency.
func BuildReport(c *Case, summary string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("cases: cannot build report for nil case")
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Incident Report: %s\n\n", c.Name))
	b.WriteString(fmt.Sprintf("- **Status:** %s\n", c.Status))
	b.WriteString(fmt.Sprintf("- **Created:** %s\n", c.CreatedAt))
	if c.ClosedAt != "" {
		b.WriteString(fmt.Sprintf("- **Closed:** %s\n", c.ClosedAt))
	}
	b.WriteString(fmt.Sprintf("- **Tools used:** %s\n", strings.Join(c.ToolsUsed, ", ")))
	if len(c.Metadata) > 0 {
		b.WriteString("\n## Metadata\n\n")
		for k, v := range c.Metadata {
			b.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
	}

	// Evidence
	b.WriteString("\n## Evidence\n\n")
	ev := c.Evidence()
	if len(ev) == 0 {
		b.WriteString("_No evidence recorded._\n")
	} else {
		b.WriteString("| Name | SHA-256 | Size | Tags |\n")
		b.WriteString("|------|---------|------|------|\n")
		for _, e := range ev {
			tags := strings.Join(e.Tags, ", ")
			sha := e.SHA256
			if len(sha) > 12 {
				sha = sha[:12] + "…"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %d | %s |\n", e.Name, sha, e.Size, tags))
		}
	}

	// IOCs
	b.WriteString("\n## Indicators of Compromise\n\n")
	iocs := c.IOCs()
	if len(iocs) == 0 {
		b.WriteString("_No IOCs tracked._\n")
	} else {
		b.WriteString("| Type | Value | Source |\n")
		b.WriteString("|------|-------|--------|\n")
		for _, ic := range iocs {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ic.Type, ic.Value, ic.Source))
		}
	}

	// Timeline
	b.WriteString("\n## Timeline\n\n")
	events := c.Timeline()
	if len(events) == 0 {
		b.WriteString("_No timeline events._\n")
	} else {
		b.WriteString("| Timestamp | Type | Detail |\n")
		b.WriteString("|-----------|------|--------|\n")
		for _, ev := range events {
			payload := ""
			if len(ev.Payload) > 0 {
				payload = fmt.Sprintf("%v", ev.Payload)
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ev.Timestamp, ev.Type, payload))
		}
	}

	// Tool Output / Findings (#4): surface actual command output, not just
	// that a tool ran. Captured from tool_run events' "output" payload.
	b.WriteString("\n## Tool Output (Findings)\n\n")
	var toolRuns int
	for _, ev := range events {
		if ev.Type != "tool_run" {
			continue
		}
		cmd := ev.ToolRunCommand()
		out := c.ToolRunOutput(ev)
		dump, _ := ev.Payload["dump"].(string)
		if cmd == "" && out == "" {
			continue
		}
		toolRuns++
		b.WriteString(fmt.Sprintf("### `%s`\n", cmd))
		if dump != "" {
			b.WriteString(fmt.Sprintf("_source: %s_\n\n", dump))
		}
		if out == "" {
			b.WriteString("_no output captured_\n\n")
			continue
		}
		b.WriteString("```\n" + out + "\n```\n\n")
	}
	if toolRuns == 0 {
		b.WriteString("_No tool output captured. Run tools (e.g. `volatility <plugin>`) to populate findings._\n")
	}

	// Notes
	if len(c.Notes) > 0 {
		b.WriteString("\n## Analyst Notes\n\n")
		for _, n := range c.Notes {
			b.WriteString(fmt.Sprintf("- %s\n", n.Content))
		}
	}

	// AI summary
	if summary != "" {
		b.WriteString("\n## AI Executive Summary\n\n")
		b.WriteString(summary + "\n")
	}

	return b.String(), nil
}

// BuildNarrative correlates the case timeline events into an ordered,
// phase-labeled narrative. It does NOT mutate timeline.jsonl. The phases are
// inferred from event types: acquisition (evidence/tool runs early), analysis
// (IOC/analysis events), and findings (notes/closures).
func BuildNarrative(c *Case) (string, error) {
	if c == nil {
		return "", fmt.Errorf("cases: cannot build narrative for nil case")
	}
	events := c.Timeline()
	if len(events) == 0 {
		return "_No timeline events to correlate._\n", nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Investigation Narrative: %s\n\n", c.Name))

	phase := ""
	for _, ev := range events {
		p := phaseFor(ev.Type)
		if p != phase {
			phase = p
			b.WriteString(fmt.Sprintf("\n## %s\n\n", phase))
		}
		if ev.Type == "tool_run" {
			cmd := ev.ToolRunCommand()
			dump, _ := ev.Payload["dump"].(string)
			line := fmt.Sprintf("- [%s] %s", ev.Timestamp, cmd)
			if dump != "" {
				line += fmt.Sprintf(" _(source: %s)_", dump)
			}
			line += " — output captured (see Findings)"
			b.WriteString(line + "\n")
			continue
		}
		payload := ""
		if len(ev.Payload) > 0 {
			payload = fmt.Sprintf(" — %v", ev.Payload)
		}
		b.WriteString(fmt.Sprintf("- [%s] %s%s\n", ev.Timestamp, ev.Type, payload))
	}

	return b.String(), nil
}

// Narrative builds a phase-labeled narrative for the case (see BuildNarrative).
func (c *Case) Narrative() (string, error) {
	return BuildNarrative(c)
}

// phaseFor maps a timeline event type to an investigation phase label.
func phaseFor(eventType string) string {
	switch {
	case eventType == "evidence_added" || eventType == "evidence_tagged" ||
		eventType == "tool.started" || eventType == "tool.finished" ||
		eventType == "tool.run" || eventType == "tool_run" || eventType == "ioc_extracted" ||
		strings.HasPrefix(eventType, "sandbox."):
		return "Acquisition & Analysis"
	case eventType == "note" || eventType == "ai_query" || eventType == "ai_response" ||
		eventType == "analysis":
		return "Findings"
	case eventType == "case.opened":
		return "Case Opened"
	case eventType == "case.closed":
		return "Case Closed"
	default:
		return "Activity"
	}
}
