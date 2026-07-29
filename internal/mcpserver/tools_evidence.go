package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
)

type evidenceListIn struct {
	Case string `json:"case" jsonschema:"case name"`
}
type evidenceItem struct {
	Name   string   `json:"name"`
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	Tags   []string `json:"tags"`
}
type evidenceListOut struct {
	Items []evidenceItem `json:"items"`
}

type evidenceAddIn struct {
	Case string   `json:"case" jsonschema:"case name"`
	Path string   `json:"path" jsonschema:"path to the file to add as evidence"`
	Tags []string `json:"tags,omitempty" jsonschema:"optional tags"`
}
type evidenceAddOut struct {
	Item evidenceItem `json:"item"`
}

type evidenceTagIn struct {
	Case string   `json:"case" jsonschema:"case name"`
	Name string   `json:"name" jsonschema:"evidence file name"`
	Tags []string `json:"tags" jsonschema:"tags to apply"`
}
type evidenceTagOut struct {
	Tagged bool `json:"tagged"`
}

type evidenceVerifyIn struct {
	Case string `json:"case" jsonschema:"case name"`
	Name string `json:"name,omitempty" jsonschema:"optional evidence name to verify; verifies all if omitted"`
}
type evidenceCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok, mismatch, missing
}
type evidenceVerifyOut struct {
	Checks []evidenceCheck `json:"checks"`
	AllOK  bool            `json:"all_ok"`
}

func (s *Server) registerEvidenceTools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "evidence_list",
		Description: "List evidence items recorded in a case.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.evidenceList)
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "evidence_add",
		Description: "Copy a file into the case evidence directory, hash it (SHA-256), and record it.",
	}, s.evidenceAdd)
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "evidence_tag",
		Description: "Add tags to an existing evidence item.",
	}, s.evidenceTag)
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "evidence_verify",
		Description: "Verify evidence integrity by re-hashing files against their recorded SHA-256.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.evidenceVerify)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (s *Server) evidenceList(ctx context.Context, req *mcp.CallToolRequest, in evidenceListIn) (*mcp.CallToolResult, evidenceListOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		var items []evidenceItem
		for _, e := range c.Evidence() {
			items = append(items, evidenceItem{Name: e.Name, SHA256: e.SHA256, Size: e.Size, Tags: e.Tags})
		}
		return items, nil
	})
	if err != nil {
		return errResult("evidence_list: %v", err), evidenceListOut{}, nil
	}
	items, _ := v.([]evidenceItem)
	return textResult(fmt.Sprintf("%d evidence item(s)", len(items))), evidenceListOut{Items: items}, nil
}

func (s *Server) evidenceAdd(ctx context.Context, req *mcp.CallToolRequest, in evidenceAddIn) (*mcp.CallToolResult, evidenceAddOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		info, err := os.Stat(in.Path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", in.Path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", in.Path)
		}
		name := filepath.Base(in.Path)
		dest := filepath.Join(c.Path, "evidence", name)
		srcAbs, _ := filepath.Abs(in.Path)
		destAbs, _ := filepath.Abs(dest)
		sum, err := hashFile(in.Path)
		if err != nil {
			return nil, fmt.Errorf("hash evidence: %w", err)
		}

		if existing := c.FindEvidenceByHash(sum); existing != nil {
			return nil, fmt.Errorf("evidence already exists: %s (added %s)", existing.Name, existing.AddedAt)
		}

		if srcAbs != destAbs {
			if existingInfo, statErr := os.Stat(dest); statErr == nil && existingInfo.Mode().IsRegular() {
				destSum, err := hashFile(dest)
				if err != nil {
					return nil, fmt.Errorf("hash existing evidence: %w", err)
				}
				if destSum != sum {
					return nil, fmt.Errorf("evidence %q already exists with a different hash — refusing to overwrite", name)
				}
			} else if err := copyFile(in.Path, dest); err != nil {
				return nil, fmt.Errorf("copy evidence: %w", err)
			}
		}

		now := time.Now().Format(time.RFC3339)
		if err := c.AppendEvidence(cases.EvidenceRecord{
			Op: "add", Name: name, SHA256: sum, Size: info.Size(), Source: in.Path, Tags: in.Tags, Time: now,
		}); err != nil {
			return nil, err
		}
		if err := c.AppendEvent(cases.TimelineEvent{
			Type: "evidence_added", Timestamp: now,
			Payload: map[string]any{"name": name, "sha256": sum, "tags": in.Tags},
		}); err != nil {
			return nil, err
		}
		return evidenceItem{Name: name, SHA256: sum, Size: info.Size(), Tags: in.Tags}, nil
	})
	if err != nil {
		return errResult("evidence_add: %v", err), evidenceAddOut{}, nil
	}
	item := v.(evidenceItem)
	return textResult(fmt.Sprintf("added evidence %q (%s)", item.Name, item.SHA256[:12])), evidenceAddOut{Item: item}, nil
}

func (s *Server) evidenceTag(ctx context.Context, req *mcp.CallToolRequest, in evidenceTagIn) (*mcp.CallToolResult, evidenceTagOut, error) {
	_, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		now := time.Now().Format(time.RFC3339)
		if err := c.AppendEvidence(cases.EvidenceRecord{Op: "tag", Name: in.Name, Tags: in.Tags, Time: now}); err != nil {
			return nil, err
		}
		if err := c.AppendEvent(cases.TimelineEvent{
			Type: "evidence_tagged", Timestamp: now,
			Payload: map[string]any{"name": in.Name, "tags": in.Tags},
		}); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return errResult("evidence_tag: %v", err), evidenceTagOut{}, nil
	}
	return textResult(fmt.Sprintf("tagged %s: %s", in.Name, strings.Join(in.Tags, ", "))), evidenceTagOut{Tagged: true}, nil
}

func (s *Server) evidenceVerify(ctx context.Context, req *mcp.CallToolRequest, in evidenceVerifyIn) (*mcp.CallToolResult, evidenceVerifyOut, error) {
	v, err := s.withCase(in.Case, func(c *cases.Case) (any, error) {
		var checks []evidenceCheck
		allOK := true
		checked := 0
		for _, e := range c.Evidence() {
			if in.Name != "" && e.Name != in.Name {
				continue
			}
			checked++
			path := filepath.Join(c.Path, "evidence", e.Name)
			sum, err := hashFile(path)
			status := "ok"
			if err != nil {
				status = "missing"
				allOK = false
			} else if sum != e.SHA256 {
				status = "mismatch"
				allOK = false
			}
			checks = append(checks, evidenceCheck{Name: e.Name, Status: status})
		}
		if in.Name != "" && checked == 0 {
			return nil, fmt.Errorf("evidence not found: %s", in.Name)
		}
		return evidenceVerifyOut{Checks: checks, AllOK: allOK}, nil
	})
	if err != nil {
		return errResult("evidence_verify: %v", err), evidenceVerifyOut{}, nil
	}
	out := v.(evidenceVerifyOut)
	return textResult(fmt.Sprintf("%d checked, all_ok=%v", len(out.Checks), out.AllOK)), out, nil
}
