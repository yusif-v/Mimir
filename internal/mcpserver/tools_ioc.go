package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/ioc"
)

type iocExtractIn struct {
	Case       string `json:"case" jsonschema:"case name"`
	Path       string `json:"path,omitempty" jsonschema:"file to extract IOCs from"`
	FromOutput string `json:"from_output,omitempty" jsonschema:"captured tool-output name (under the case's output/ dir) to extract from"`
}
type indicator struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
type iocExtractOut struct {
	Indicators []indicator `json:"indicators"`
}

type iocDiffIn struct {
	Case  string `json:"case" jsonschema:"case name"`
	FileA string `json:"file_a" jsonschema:"first file to extract indicators from"`
	FileB string `json:"file_b" jsonschema:"second file to extract indicators from"`
}
type iocDiffOut struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Shared  []string `json:"shared"`
}

type correlateIn struct {
	Case string `json:"case" jsonschema:"case name"`
	Dot  bool   `json:"dot,omitempty" jsonschema:"if true, include a Graphviz digraph in the summary"`
}
type correlateOut struct {
	DomainCounts map[string]int `json:"domain_counts"`
	CrossDomain  []string       `json:"cross_domain"`
	Dot          string         `json:"dot,omitempty"`
}

func (s *Server) registerIOCTools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "ioc_extract",
		Description: "Extract indicators of compromise (IPs, domains, URLs, emails, hashes) from a file or a captured tool output, and record them against the case.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.iocExtract)
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "ioc_diff",
		Description: "Compare the indicator sets extracted from two files and report what's added, removed, or shared.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.iocDiff)
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "correlate",
		Description: "Correlate tracked IOCs across domains (memory/disk/network/other) and surface cross-domain indicators.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.correlate)
}

func (s *Server) iocExtract(ctx context.Context, req *mcp.CallToolRequest, in iocExtractIn) (*mcp.CallToolResult, iocExtractOut, error) {
	if in.Path == "" && in.FromOutput == "" {
		return errResult("ioc_extract: one of path or from_output is required"), iocExtractOut{}, nil
	}
	if in.Path != "" && in.FromOutput != "" {
		return errResult("ioc_extract: path and from_output are mutually exclusive"), iocExtractOut{}, nil
	}

	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		var data []byte
		var source string
		if in.FromOutput != "" {
			source = in.FromOutput
			b, err := os.ReadFile(filepath.Join(c.Path, "output", source))
			if err != nil {
				return nil, fmt.Errorf("read output %q: %w", source, err)
			}
			data = b
		} else {
			source = in.Path
			b, err := os.ReadFile(source)
			if err != nil {
				return nil, fmt.Errorf("read %q: %w", source, err)
			}
			data = b
		}

		inds := ioc.Extract(data)
		out := make([]indicator, 0, len(inds))
		counts := map[string]int{}
		now := time.Now().Format(time.RFC3339)
		for _, ind := range inds {
			out = append(out, indicator{Type: ind.Type, Value: ind.Value})
			counts[ind.Type]++
			if err := c.AppendIOC(cases.IOCRecord{Type: ind.Type, Value: ind.Value, Source: source, Time: now}); err != nil {
				return nil, fmt.Errorf("append ioc: %w", err)
			}
		}
		if len(inds) > 0 {
			if err := c.AppendEvent(cases.TimelineEvent{
				Type: "ioc_extracted", Timestamp: now,
				Payload: map[string]any{"source": source, "counts": counts, "total": len(inds)},
			}); err != nil {
				return nil, fmt.Errorf("append timeline: %w", err)
			}
		}
		return out, nil
	})
	if err != nil {
		return errResult("ioc_extract: %v", err), iocExtractOut{}, nil
	}
	inds, _ := v.([]indicator)
	return textResult(fmt.Sprintf("%d indicator(s) extracted", len(inds))), iocExtractOut{Indicators: inds}, nil
}

func (s *Server) iocDiff(ctx context.Context, req *mcp.CallToolRequest, in iocDiffIn) (*mcp.CallToolResult, iocDiffOut, error) {
	read := func(p string) (map[string]bool, error) {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", p, err)
		}
		set := map[string]bool{}
		for _, ind := range ioc.Extract(b) {
			set[ind.Type+":"+ind.Value] = true
		}
		return set, nil
	}

	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		aSet, err := read(in.FileA)
		if err != nil {
			return nil, err
		}
		bSet, err := read(in.FileB)
		if err != nil {
			return nil, err
		}
		var added, removed, shared []string
		for k := range aSet {
			if bSet[k] {
				shared = append(shared, k)
			} else {
				removed = append(removed, k)
			}
		}
		for k := range bSet {
			if !aSet[k] {
				added = append(added, k)
			}
		}
		return iocDiffOut{Added: added, Removed: removed, Shared: shared}, nil
	})
	if err != nil {
		return errResult("ioc_diff: %v", err), iocDiffOut{}, nil
	}
	out := v.(iocDiffOut)
	return textResult(fmt.Sprintf("added=%d removed=%d shared=%d", len(out.Added), len(out.Removed), len(out.Shared))), out, nil
}

// domainOf classifies an IOC's domain by its source tool, mirroring cmdCorrelate.
func domainOf(src string) string {
	l := strings.ToLower(src)
	switch {
	case strings.Contains(l, "volatility"), strings.Contains(l, "mem"):
		return "memory"
	case strings.Contains(l, "sleuthkit"), strings.Contains(l, "icat"), strings.Contains(l, "fls"):
		return "disk"
	case strings.Contains(l, "zeek"), strings.Contains(l, "tshark"):
		return "network"
	default:
		return "other"
	}
}

func (s *Server) correlate(ctx context.Context, req *mcp.CallToolRequest, in correlateIn) (*mcp.CallToolResult, correlateOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		iocs := c.IOCs()

		byValue := map[string]map[string]bool{}
		byValueTypes := map[string]string{}
		for _, ic := range iocs {
			d := domainOf(ic.Source)
			if byValue[ic.Value] == nil {
				byValue[ic.Value] = map[string]bool{}
				byValueTypes[ic.Value] = ic.Type
			}
			byValue[ic.Value][d] = true
		}

		domCount := map[string]int{}
		for _, dmap := range byValue {
			for d := range dmap {
				domCount[d]++
			}
		}

		var cross []string
		for v, dmap := range byValue {
			if len(dmap) >= 2 {
				cross = append(cross, v)
			}
		}

		out := correlateOut{DomainCounts: domCount, CrossDomain: cross}
		if in.Dot {
			var b strings.Builder
			b.WriteString("digraph correlate {\n")
			for val, dmap := range byValue {
				label := strings.ReplaceAll(val, "\"", "")
				fmt.Fprintf(&b, "  \"%s\" [label=\"%s\\n(%s)\"];\n", label, label, byValueTypes[val])
				for d := range dmap {
					fmt.Fprintf(&b, "  \"%s\" -> \"domain:%s\";\n", label, d)
				}
			}
			b.WriteString("}\n")
			out.Dot = b.String()
		}
		return out, nil
	})
	if err != nil {
		return errResult("correlate: %v", err), correlateOut{}, nil
	}
	out := v.(correlateOut)
	summary := fmt.Sprintf("%d cross-domain indicator(s) across %d domain(s)", len(out.CrossDomain), len(out.DomainCounts))
	if in.Dot {
		summary = summary + "\n\n" + out.Dot
	}
	return textResult(summary), out, nil
}
