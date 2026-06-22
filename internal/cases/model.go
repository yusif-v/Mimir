// Package cases manages investigation cases.
package cases

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	evidence  []Evidence        `json:"-"`
	iocs      []IOC             `json:"-"`
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
	if err := c.loadEvidence(); err != nil {
		return nil, fmt.Errorf("load evidence: %w", err)
	}
	if err := c.loadIOC(); err != nil {
		return nil, fmt.Errorf("load ioc: %w", err)
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

// --- Evidence ---

type EvidenceRecord struct {
	Op     string   `json:"op"`
	Name   string   `json:"name"`
	SHA256 string   `json:"sha256,omitempty"`
	Size   int64    `json:"size,omitempty"`
	Source string   `json:"source,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Time   string   `json:"time"`
}

type Evidence struct {
	Name    string
	SHA256  string
	Source  string
	AddedAt string
	Size    int64
	Tags    []string
}

func (c *Case) AppendEvidence(rec EvidenceRecord) error {
	if err := appendJSONL(filepath.Join(c.Path, "evidence.jsonl"), rec); err != nil {
		return err
	}
	// Re-fold from disk (which now includes rec) — do NOT append rec again.
	return c.loadEvidence()
}

func (c *Case) Evidence() []Evidence { return c.evidence }

// FindEvidenceByHash returns the evidence matching the given SHA-256 hash,
// or nil if no evidence with that hash exists.
func (c *Case) FindEvidenceByHash(sha256 string) *Evidence {
	for i := range c.evidence {
		if c.evidence[i].SHA256 == sha256 {
			return &c.evidence[i]
		}
	}
	return nil
}

func (c *Case) loadEvidence() error {
	recs, err := readEvidenceRecords(filepath.Join(c.Path, "evidence.jsonl"))
	if err != nil {
		return err
	}
	c.evidence = foldEvidence(recs)
	return nil
}

// --- IOC ---

type IOCRecord struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Time   string `json:"time"`
}

type IOC struct {
	Type   string
	Value  string
	Source string
	Time   string
}

func (c *Case) AppendIOC(rec IOCRecord) error {
	if err := appendJSONL(filepath.Join(c.Path, "ioc.jsonl"), rec); err != nil {
		return err
	}
	c.iocs = append(c.iocs, IOC(rec))
	c.iocs = dedupeIOCs(c.iocs)
	return nil
}

func (c *Case) IOCs() []IOC { return c.iocs }

func (c *Case) loadIOC() error {
	recs, err := readIOCRecords(filepath.Join(c.Path, "ioc.jsonl"))
	if err != nil {
		return err
	}
	var iocs []IOC
	for _, r := range recs {
		iocs = append(iocs, IOC(r))
	}
	c.iocs = dedupeIOCs(iocs)
	return nil
}

// --- Shared JSONL helpers ---

func appendJSONL(path string, v any) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func readEvidenceRecords(path string) ([]EvidenceRecord, error) {
	var out []EvidenceRecord
	err := scanJSONL(path, func(line []byte) {
		var r EvidenceRecord
		if json.Unmarshal(line, &r) == nil {
			out = append(out, r)
		} else {
			log.Printf("evidence: skipping corrupt line in %s", path)
		}
	})
	return out, err
}

func readIOCRecords(path string) ([]IOCRecord, error) {
	var out []IOCRecord
	err := scanJSONL(path, func(line []byte) {
		var r IOCRecord
		if json.Unmarshal(line, &r) == nil {
			out = append(out, r)
		} else {
			log.Printf("ioc: skipping corrupt line in %s", path)
		}
	})
	return out, err
}

func scanJSONL(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		fn(sc.Bytes())
	}
	return sc.Err()
}

func foldEvidence(recs []EvidenceRecord) []Evidence {
	byName := map[string]*Evidence{}
	var order []string
	for _, r := range recs {
		e, ok := byName[r.Name]
		if !ok {
			e = &Evidence{Name: r.Name}
			byName[r.Name] = e
			order = append(order, r.Name)
		}
		if r.Op == "add" {
			e.SHA256, e.Size, e.Source, e.AddedAt = r.SHA256, r.Size, r.Source, r.Time
		}
		for _, tag := range r.Tags {
			if !containsStr(e.Tags, tag) {
				e.Tags = append(e.Tags, tag)
			}
		}
	}
	sort.Strings(order)
	out := make([]Evidence, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out
}

func dedupeIOCs(in []IOC) []IOC {
	seen := map[string]bool{}
	var out []IOC
	for _, i := range in {
		k := i.Type + "\x00" + i.Value
		if !seen[k] {
			seen[k] = true
			out = append(out, i)
		}
	}
	return out
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// SearchResult is a single match from SearchOutput.
type SearchResult struct {
	File    string
	Line    int
	Content string
}

// SearchOutput walks the case output/ directory and returns every line whose
// lowercased text contains the lowercased query.
func (c *Case) SearchOutput(query string) []SearchResult {
	var results []SearchResult
	outputDir := filepath.Join(c.Path, "output")
	q := strings.ToLower(query)
	filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), q) {
				rel, _ := filepath.Rel(c.Path, path)
				results = append(results, SearchResult{
					File:    rel,
					Line:    i + 1,
					Content: strings.TrimSpace(line),
				})
			}
		}
		return nil
	})
	return results
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
