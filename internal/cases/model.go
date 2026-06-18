// Package cases manages investigation cases.
package cases

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Case represents an investigation case.
type Case struct {
	Name      string            `json:"name"`
	Path      string            `json:"path"`
	CreatedAt string            `json:"created_at"`
	OpenedAt  string            `json:"opened_at,omitempty"`
	ClosedAt  string            `json:"closed_at,omitempty"`
	Status    string            `json:"status"` // open, closed
	ToolsUsed []string          `json:"tools_used"`
	Notes     []Note            `json:"notes"`
	Metadata  map[string]string `json:"metadata"`
	events    []TimelineEvent   `json:"-"`
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
	if err := c.loadTimeline(); err != nil {
		return nil, fmt.Errorf("load timeline: %w", err)
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

// AddNote adds a note to the case and records it on the timeline.
func (c *Case) AddNote(content, author string) error {
	ts := time.Now().Format(time.RFC3339)
	c.Notes = append(c.Notes, Note{
		Timestamp: ts,
		Author:    author,
		Content:   content,
	})
	return c.AppendEvent(TimelineEvent{
		Type:      "note",
		Timestamp: ts,
		Payload:   map[string]any{"author": author, "content": content},
	})
}

// TimelineEvent is one chronological entry in a case investigation.
type TimelineEvent struct {
	Type      string         `json:"type"`      // tool_run | note | case_opened | case_closed
	Timestamp string         `json:"timestamp"` // RFC3339
	Payload   map[string]any `json:"payload"`
}

// AppendEvent writes one event to the append-only timeline.jsonl and the
// in-memory cache. The JSONL file is the forensic source of truth.
func (c *Case) AppendEvent(ev TimelineEvent) error {
	path := filepath.Join(c.Path, "timeline.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open timeline: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write timeline: %w", err)
	}
	c.events = append(c.events, ev)
	return nil
}

// Timeline returns the in-memory cache of timeline events.
func (c *Case) Timeline() []TimelineEvent {
	return c.events
}

// loadTimeline streams timeline.jsonl into the in-memory cache. A missing file
// yields an empty timeline. Corrupt lines are skipped with a warning so one bad
// append cannot brick case loading.
func (c *Case) loadTimeline() error {
	path := filepath.Join(c.Path, "timeline.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.events = nil
			return nil
		}
		return fmt.Errorf("open timeline: %w", err)
	}
	defer f.Close()

	c.events = nil
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev TimelineEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			log.Printf("timeline: skipping corrupt line in %s: %v", path, err)
			continue
		}
		c.events = append(c.events, ev)
	}
	return scanner.Err()
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
