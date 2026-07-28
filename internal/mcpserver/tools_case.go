package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type casesListIn struct {
	Status string `json:"status,omitempty" jsonschema:"filter by status: open or closed"`
}
type caseSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type casesListOut struct {
	Cases []caseSummary `json:"cases"`
}

func (s *Server) registerCaseTools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "cases_list",
		Description: "List all Mimir cases, optionally filtered by open/closed status.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.casesList)
}

func (s *Server) casesList(ctx context.Context, req *mcp.CallToolRequest, in casesListIn) (*mcp.CallToolResult, casesListOut, error) {
	s.mu.Lock()
	list, err := s.app.Cases.List()
	s.mu.Unlock()
	if err != nil {
		return errResult("list cases: %v", err), casesListOut{}, nil
	}
	out := casesListOut{}
	for _, c := range list {
		st := c.Status
		if in.Status != "" && !strings.EqualFold(st, in.Status) {
			continue
		}
		out.Cases = append(out.Cases, caseSummary{Name: c.Name, Status: st})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d case(s):\n", len(out.Cases))
	for _, c := range out.Cases {
		fmt.Fprintf(&b, "  %s [%s]\n", c.Name, c.Status)
	}
	return textResult(b.String()), out, nil
}
