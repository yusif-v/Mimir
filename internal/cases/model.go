// Package cases manages investigation cases.
package cases

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Case represents an investigation case.
type Case struct {
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	CreatedAt  string            `json:"created_at"`
	OpenedAt   string            `json:"opened_at,omitempty"`
	ClosedAt   string            `json:"closed_at,omitempty"`
	Status     string            `json:"status"` // open, closed
	ToolsUsed  []string          `json:"tools_used"`
	Notes      []Note            `json:"notes"`
	Metadata   map[string]string `json:"metadata"`
}

// Note is an annotation added by the analyst.
type Note struct {
	Timestamp string `json:"timestamp"`
	Author    string `json:"author"`
	Content   string `json:"content"`
}

// NewCase creates a new case with the given name and path.
func NewCase(name, path string) *Case {
	return &Case{
		Name:      name,
		Path:      path,
		CreatedAt: time.Now().Format(time.RFC3339),
		Status:    "open",
		ToolsUsed: []string{},
		Notes:     []Note{},
		Metadata:  map[string]string{},
	}
}

// Save writes the case metadata to disk.
func (c *Case) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal case: %w", err)
	}
	metaPath := filepath.Join(c.Path, "case.json")
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("write case.json: %w", err)
	}
	return nil
}

// LoadCase reads a case from a directory.
func LoadCase(path string) (*Case, error) {
	metaPath := filepath.Join(path, "case.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("read case.json: %w", err)
	}
	var c Case
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("unmarshal case: %w", err)
	}
	return &c, nil
}

// AddToolUsage records that a tool was used in this case.
func (c *Case) AddToolUsage(tool string) {
	for _, t := range c.ToolsUsed {
		if t == tool {
			return
		}
	}
	c.ToolsUsed = append(c.ToolsUsed, tool)
}

// AddNote adds a note to the case.
func (c *Case) AddNote(content, author string) {
	c.Notes = append(c.Notes, Note{
		Timestamp: time.Now().Format(time.RFC3339),
		Author:    author,
		Content:   content,
	})
}

// Scaffold creates the case directory structure.
func (c *Case) Scaffold() error {
	dirs := []string{
		c.Path,
		filepath.Join(c.Path, "evidence"),
		filepath.Join(c.Path, "output"),
		filepath.Join(c.Path, "notes"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	return nil
}
