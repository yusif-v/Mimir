package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
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

type caseCreateIn struct {
	Name     string `json:"name" jsonschema:"case name"`
	Template string `json:"template,omitempty" jsonschema:"optional case template name"`
}
type caseCreateOut struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type caseStatusIn struct {
	Case string `json:"case" jsonschema:"case name"`
}
type caseStatusOut struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	EvidenceCount int    `json:"evidence_count"`
	NoteCount     int    `json:"note_count"`
	EventCount    int    `json:"event_count"`
	IOCCount      int    `json:"ioc_count"`
}

func (s *Server) registerCaseTools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "cases_list",
		Description: "List all Mimir cases, optionally filtered by open/closed status.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.casesList)
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "case_create",
		Description: "Create a new Mimir case, optionally from a template.",
	}, s.caseCreate)
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "case_status",
		Description: "Report status and counts (evidence, notes, timeline events, IOCs) for a case.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.caseStatus)
}

func (s *Server) caseCreate(ctx context.Context, req *mcp.CallToolRequest, in caseCreateIn) (*mcp.CallToolResult, caseCreateOut, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		c   *cases.Case
		err error
	)
	if in.Template != "" {
		c, err = s.app.Cases.CreateWithTemplate(in.Name, in.Template)
	} else {
		c, err = s.app.Cases.Create(in.Name)
	}
	if err != nil {
		return errResult("create case: %v", err), caseCreateOut{}, nil
	}
	out := caseCreateOut{Name: c.Name, Path: c.Path}
	return textResult(fmt.Sprintf("created case %q", c.Name)), out, nil
}

func (s *Server) caseStatus(ctx context.Context, req *mcp.CallToolRequest, in caseStatusIn) (*mcp.CallToolResult, caseStatusOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		return caseStatusOut{
			Name:          c.Name,
			Status:        c.Status,
			EvidenceCount: len(c.Evidence()),
			NoteCount:     len(c.Notes),
			EventCount:    len(c.Timeline()),
			IOCCount:      len(c.IOCs()),
		}, nil
	})
	if err != nil {
		return errResult("case_status: %v", err), caseStatusOut{}, nil
	}
	out := v.(caseStatusOut)
	return textResult(fmt.Sprintf("case %q [%s]: %d evidence, %d notes, %d events, %d iocs",
		out.Name, out.Status, out.EvidenceCount, out.NoteCount, out.EventCount, out.IOCCount)), out, nil
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
