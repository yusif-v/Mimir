package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
)

type timelineReadIn struct {
	Case  string `json:"case" jsonschema:"case name"`
	Type  string `json:"type,omitempty" jsonschema:"filter by event type"`
	Grep  string `json:"grep,omitempty" jsonschema:"filter by substring match"`
	Limit int    `json:"limit,omitempty" jsonschema:"max events (0 = all)"`
}
type timelineEvent struct {
	Time string `json:"time"`
	Type string `json:"type"`
	Text string `json:"text"`
}
type timelineReadOut struct {
	Events []timelineEvent `json:"events"`
}

type timelineCorrelateIn struct {
	Case string `json:"case" jsonschema:"case name"`
}
type toolRunCount struct {
	Tool string `json:"tool"`
	Runs int    `json:"runs"`
}
type iocTypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}
type timelineCorrelateOut struct {
	TotalRuns    int            `json:"total_runs"`
	ByTool       []toolRunCount `json:"by_tool"`
	IndicatorsBy []iocTypeCount `json:"indicators_by_type"`
}

type noteAddIn struct {
	Case string `json:"case" jsonschema:"case name"`
	Text string `json:"text" jsonschema:"note content"`
}
type noteAddOut struct {
	Added bool `json:"added"`
}

func (s *Server) registerTimelineTools() {
	mcp.AddTool(s.srv, &mcp.Tool{Name: "timeline_read",
		Description: "Read a case timeline, optionally filtered by type/substring/limit.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.timelineRead)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "timeline_correlate",
		Description: "Group a case's tool runs and summarize indicators by type (cross-tool view).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.timelineCorrelate)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "note_add",
		Description: "Append an investigator note to a case timeline."}, s.noteAdd)
}

// eventText renders a TimelineEvent's payload into a short human-readable
// summary for MCP consumers, mirroring cmdTimeline's per-type formatting.
func eventText(ev cases.TimelineEvent) string {
	switch ev.Type {
	case "note":
		return fmt.Sprint(ev.Payload["content"])
	case "tool_run":
		return fmt.Sprintf("%v %v -> code %v", ev.Payload["tool"], ev.Payload["args"], ev.Payload["return_code"])
	case "case_opened":
		return "case opened"
	case "case_closed":
		return "case closed"
	default:
		if len(ev.Payload) == 0 {
			return ""
		}
		return fmt.Sprint(ev.Payload)
	}
}

// eventMatchesGrep reports whether the event's type or any payload value
// contains the (case-insensitive) substring sub.
func eventMatchesGrep(ev cases.TimelineEvent, sub string) bool {
	lower := strings.ToLower(sub)
	if strings.Contains(strings.ToLower(ev.Type), lower) {
		return true
	}
	for _, v := range ev.Payload {
		if strings.Contains(strings.ToLower(fmt.Sprint(v)), lower) {
			return true
		}
	}
	return false
}

func (s *Server) timelineRead(ctx context.Context, req *mcp.CallToolRequest, in timelineReadIn) (*mcp.CallToolResult, timelineReadOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		var evs []timelineEvent
		for _, e := range c.Timeline() {
			if in.Type != "" && !strings.EqualFold(e.Type, in.Type) {
				continue
			}
			if in.Grep != "" && !eventMatchesGrep(e, in.Grep) {
				continue
			}
			evs = append(evs, timelineEvent{Time: e.Timestamp, Type: e.Type, Text: eventText(e)})
		}
		if in.Limit > 0 && len(evs) > in.Limit {
			evs = evs[len(evs)-in.Limit:]
		}
		return evs, nil
	})
	if err != nil {
		return errResult("timeline_read: %v", err), timelineReadOut{}, nil
	}
	evs, _ := v.([]timelineEvent)
	return textResult(fmt.Sprintf("%d event(s)", len(evs))), timelineReadOut{Events: evs}, nil
}

func (s *Server) timelineCorrelate(ctx context.Context, req *mcp.CallToolRequest, in timelineCorrelateIn) (*mcp.CallToolResult, timelineCorrelateOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		out := timelineCorrelateOut{}
		byTool := map[string]int{}
		for _, ev := range c.Timeline() {
			if ev.Type != "tool_run" {
				continue
			}
			out.TotalRuns++
			tool, _ := ev.Payload["tool"].(string)
			if tool == "" {
				tool = "unknown"
			}
			byTool[tool]++
		}
		toolNames := make([]string, 0, len(byTool))
		for t := range byTool {
			toolNames = append(toolNames, t)
		}
		sort.Strings(toolNames)
		for _, t := range toolNames {
			out.ByTool = append(out.ByTool, toolRunCount{Tool: t, Runs: byTool[t]})
		}

		byType := map[string]int{}
		for _, ic := range c.IOCs() {
			byType[ic.Type]++
		}
		typeNames := make([]string, 0, len(byType))
		for t := range byType {
			typeNames = append(typeNames, t)
		}
		sort.Strings(typeNames)
		for _, t := range typeNames {
			out.IndicatorsBy = append(out.IndicatorsBy, iocTypeCount{Type: t, Count: byType[t]})
		}
		return out, nil
	})
	if err != nil {
		return errResult("timeline_correlate: %v", err), timelineCorrelateOut{}, nil
	}
	out := v.(timelineCorrelateOut)
	return textResult(fmt.Sprintf("%d tool run(s) across %d tool(s), %d indicator type(s)",
		out.TotalRuns, len(out.ByTool), len(out.IndicatorsBy))), out, nil
}

func (s *Server) noteAdd(ctx context.Context, req *mcp.CallToolRequest, in noteAddIn) (*mcp.CallToolResult, noteAddOut, error) {
	_, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		if err := c.AddNote(in.Text, "mcp"); err != nil {
			return nil, err
		}
		return nil, c.Save()
	})
	if err != nil {
		return errResult("note_add: %v", err), noteAddOut{}, nil
	}
	return textResult("note added"), noteAddOut{Added: true}, nil
}
