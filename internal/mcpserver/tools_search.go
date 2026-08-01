package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
)

type searchIn struct {
	Query    string `json:"query" jsonschema:"case-insensitive search query"`
	Case     string `json:"case,omitempty" jsonschema:"restrict search to a single case name"`
	InOutput bool   `json:"in_output,omitempty" jsonschema:"if true, also search captured tool output under each case's output/ dir"`
	Limit    int    `json:"limit,omitempty" jsonschema:"max hits per case (0 = all)"`
}
type searchHit struct {
	Case   string `json:"case"`
	Where  string `json:"where"` // name | note | timeline | ioc | evidence | metadata | output
	Detail string `json:"detail"`
}
type searchOut struct {
	Hits []searchHit `json:"hits"`
}

func (s *Server) registerSearchTool() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "search",
		Description: "Search across Mimir cases (notes, timeline, IOCs, evidence names, metadata) for a query. Optionally search captured tool output too.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.search)
}

func (s *Server) search(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
	if in.Query == "" {
		return errResult("search: query is required"), searchOut{}, nil
	}
	q := strings.ToLower(in.Query)

	s.mu.Lock()
	var list []*cases.Case
	if in.Case != "" {
		c, err := s.app.Cases.Load(in.Case)
		if err != nil {
			s.mu.Unlock()
			return errResult("search: %v", err), searchOut{}, nil
		}
		list = []*cases.Case{c}
	} else {
		var err error
		list, err = s.app.Cases.List()
		if err != nil {
			s.mu.Unlock()
			return errResult("search: %v", err), searchOut{}, nil
		}
	}
	s.mu.Unlock()

	out := searchOut{}
	for _, c := range list {
		hits := searchCase(c, q, in.InOutput)
		if in.Limit > 0 && len(hits) > in.Limit {
			hits = hits[:in.Limit]
		}
		out.Hits = append(out.Hits, hits...)
	}
	sort.Slice(out.Hits, func(i, j int) bool {
		if out.Hits[i].Case != out.Hits[j].Case {
			return out.Hits[i].Case < out.Hits[j].Case
		}
		return out.Hits[i].Where < out.Hits[j].Where
	})
	summary := fmt.Sprintf("%d hit(s) across %d case(s)", len(out.Hits), len(list))
	if len(out.Hits) == 0 {
		summary = "no matches"
	}
	return textResult(summary), out, nil
}

// searchCase finds query matches within a single case: name, notes, timeline
// events, IOCs, evidence names, metadata, and — when includeOutput is set —
// the contents of captured tool output under the case's output/ dir.
func searchCase(c *cases.Case, q string, includeOutput bool) []searchHit {
	var hits []searchHit
	hit := func(where, detail string) {
		hits = append(hits, searchHit{Case: c.Name, Where: where, Detail: detail})
	}

	if strings.Contains(strings.ToLower(c.Name), q) {
		hit("name", c.Name)
	}
	for _, n := range c.Notes {
		if strings.Contains(strings.ToLower(n.Content), q) {
			hit("note", n.Content)
		}
	}
	for _, ev := range c.Timeline() {
		if eventMatchesGrep(ev, q) {
			hit("timeline", eventText(ev))
		}
	}
	for _, ic := range c.IOCs() {
		if strings.Contains(strings.ToLower(ic.Value), q) || strings.Contains(strings.ToLower(ic.Type), q) {
			hit("ioc", fmt.Sprintf("[%s] %s", ic.Type, ic.Value))
		}
	}
	for _, e := range c.Evidence() {
		if strings.Contains(strings.ToLower(e.Name), q) {
			hit("evidence", e.Name)
		}
	}
	for k, v := range c.Metadata {
		if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
			hit("metadata", fmt.Sprintf("%s=%s", k, v))
		}
	}
	if includeOutput {
		for _, r := range c.SearchOutput(q) {
			hit("output", fmt.Sprintf("%s:%d: %s", r.File, r.Line, r.Content))
		}
	}
	return hits
}
