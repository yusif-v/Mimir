package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/casearchive"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/shell"
)

type reportIn struct {
	Case    string `json:"case" jsonschema:"case name"`
	Summary string `json:"summary,omitempty" jsonschema:"optional executive summary to embed (e.g. from an external AI call); omitted if empty"`
}
type reportOut struct {
	Markdown string `json:"markdown"`
}

type exportIn struct {
	Case       string `json:"case" jsonschema:"case name"`
	Format     string `json:"format,omitempty" jsonschema:"export format: csv (default) or json"`
	Path       string `json:"path" jsonschema:"output file path"`
	Provenance bool   `json:"provenance,omitempty" jsonschema:"also write a <path>.manifest.json with file hash, tool runs, and indicators"`
}
type exportOut struct {
	Path         string `json:"path"`
	ManifestPath string `json:"manifest_path,omitempty"`
}

func (s *Server) registerReportTools() {
	mcp.AddTool(s.srv, &mcp.Tool{Name: "report",
		Description: "Build a full Markdown incident report for a case (evidence, IOCs, timeline, tool output, notes).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, s.report)
	mcp.AddTool(s.srv, &mcp.Tool{Name: "export",
		Description: "Export a case timeline to CSV or JSON at a given path, optionally with a provenance manifest.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false}}, s.export)
}

func (s *Server) report(ctx context.Context, req *mcp.CallToolRequest, in reportIn) (*mcp.CallToolResult, reportOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		return cases.BuildReport(c, in.Summary)
	})
	if err != nil {
		return errResult("report: %v", err), reportOut{}, nil
	}
	md, _ := v.(string)
	return textResult(md), reportOut{Markdown: md}, nil
}

func (s *Server) export(ctx context.Context, req *mcp.CallToolRequest, in exportIn) (*mcp.CallToolResult, exportOut, error) {
	if in.Path == "" {
		return errResult("export: path is required"), exportOut{}, nil
	}
	format := in.Format
	if format == "" {
		format = "csv"
	}

	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		if err := c.ExportTimeline(format, in.Path); err != nil {
			return nil, err
		}
		out := exportOut{Path: in.Path}
		if in.Provenance {
			manPath, err := writeExportManifest(c, in.Path)
			if err != nil {
				return nil, err
			}
			out.ManifestPath = manPath
		}
		return out, nil
	})
	if err != nil {
		return errResult("export: %v", err), exportOut{}, nil
	}
	out, _ := v.(exportOut)
	summary := fmt.Sprintf("exported %s", out.Path)
	if out.ManifestPath != "" {
		summary += fmt.Sprintf(" (with provenance manifest: %s)", out.ManifestPath)
	}
	return textResult(summary), out, nil
}

// writeExportManifest writes a provenance sidecar manifest.json next to the
// exported timeline file, mirroring the fields casearchive.ExportFull embeds
// in a full archive's manifest.json: Mimir version, tool runs, and indicators
// -- plus a SHA-256 of the exported file itself so the export is verifiable.
func writeExportManifest(c *cases.Case, exportedPath string) (string, error) {
	f, err := os.Open(exportedPath)
	if err != nil {
		return "", fmt.Errorf("open exported file for hashing: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash exported file: %w", err)
	}

	man := struct {
		Name               string                 `json:"name"`
		Exported           string                 `json:"exported"`
		Mimir              string                 `json:"mimir_version"`
		ExportedFile       string                 `json:"exported_file"`
		ExportedFileSHA256 string                 `json:"exported_file_sha256"`
		ToolRuns           []string               `json:"tool_runs,omitempty"`
		Indicators         []casearchive.IOCEntry `json:"indicators,omitempty"`
	}{
		Name:               c.Name,
		Exported:           time.Now().Format(time.RFC3339),
		Mimir:              shell.Version,
		ExportedFile:       exportedPath,
		ExportedFileSHA256: hex.EncodeToString(h.Sum(nil)),
	}
	for _, ev := range c.Timeline() {
		if ev.Type != "tool_run" {
			continue
		}
		if cmd := ev.ToolRunCommand(); cmd != "" {
			man.ToolRuns = append(man.ToolRuns, cmd)
		}
	}
	for _, i := range c.IOCs() {
		man.Indicators = append(man.Indicators, casearchive.IOCEntry{Type: i.Type, Value: i.Value, Source: i.Source})
	}

	manBytes, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	manPath := exportedPath + ".manifest.json"
	if err := os.WriteFile(manPath, manBytes, 0644); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}
	return manPath, nil
}
