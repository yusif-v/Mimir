package cases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExportTimelineCSV(t *testing.T) {
	dir := t.TempDir()
	c := &Case{
		Name: "test-case",
		Path: dir,
		events: []TimelineEvent{
			{
				Type:      "case_opened",
			Timestamp: "2025-01-01T10:00:00Z",
				Payload:   map[string]any{},
			},
			{
				Type:      "note",
				Timestamp: "2025-01-01T11:00:00Z",
				Payload:   map[string]any{"author": "analyst", "content": "Initial triage"},
			},
			{
				Type:      "tool_run",
				Timestamp: "2025-01-01T12:00:00Z",
				Payload:   map[string]any{"tool": "yara", "args": []string{"rules.yar", "sample.exe"}, "success": true},
			},
		},
	}

	outPath := filepath.Join(dir, "timeline.csv")
	if err := c.ExportTimeline("csv", outPath); err != nil {
		t.Fatalf("ExportTimeline csv: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 { // header + 3 events
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), string(data))
	}

	// Check header
	if !strings.HasPrefix(lines[0], "timestamp,type,details") {
		t.Errorf("unexpected header: %s", lines[0])
	}

	// Check that each event type appears in the CSV
	content := string(data)
	if !strings.Contains(content, "case_opened") {
		t.Error("missing case_opened in csv")
	}
	if !strings.Contains(content, "note") {
		t.Error("missing note in csv")
	}
	if !strings.Contains(content, "tool_run") {
		t.Error("missing tool_run in csv")
	}
}

func TestExportTimelineJSON(t *testing.T) {
	dir := t.TempDir()
	c := &Case{
		Name: "test-case",
		Path: dir,
		events: []TimelineEvent{
			{
				Type:      "case_opened",
				Timestamp: "2025-01-01T10:00:00Z",
				Payload:   map[string]any{},
			},
			{
				Type:      "note",
				Timestamp: "2025-01-01T11:00:00Z",
				Payload:   map[string]any{"author": "analyst", "content": "Initial triage"},
			},
		},
	}

	outPath := filepath.Join(dir, "timeline.json")
	if err := c.ExportTimeline("json", outPath); err != nil {
		t.Fatalf("ExportTimeline json: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}

	var events []TimelineEvent
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "case_opened" {
		t.Errorf("first event type: got %q, want %q", events[0].Type, "case_opened")
	}
	if events[1].Type != "note" {
		t.Errorf("second event type: got %q, want %q", events[1].Type, "note")
	}
}

func TestExportTimelineDefaultPath(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}

	c := &Case{
		Name: "test-case",
		Path: dir,
		events: []TimelineEvent{
			{
				Type:      "case_opened",
				Timestamp: time.Now().Format(time.RFC3339),
				Payload:   map[string]any{},
			},
		},
	}

	// Empty path should default to <case>/output/timeline.csv
	if err := c.ExportTimeline("", ""); err != nil {
		t.Fatalf("ExportTimeline default: %v", err)
	}

	// Default format is CSV
	defaultPath := filepath.Join(dir, "output", "timeline.csv")
	if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s, but it does not exist", defaultPath)
	}
}

func TestExportTimelineDefaultPathJSON(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}

	c := &Case{
		Name: "test-case",
		Path: dir,
		events: []TimelineEvent{
			{
				Type:      "case_opened",
				Timestamp: time.Now().Format(time.RFC3339),
				Payload:   map[string]any{},
			},
		},
	}

	// format=json with empty path should default to <case>/output/timeline.json
	if err := c.ExportTimeline("json", ""); err != nil {
		t.Fatalf("ExportTimeline json default: %v", err)
	}

	defaultPath := filepath.Join(dir, "output", "timeline.json")
	if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s, but it does not exist", defaultPath)
	}
}

func TestExportTimelineUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	c := &Case{
		Name:   "test-case",
		Path:   dir,
		events: []TimelineEvent{},
	}

	err := c.ExportTimeline("xml", filepath.Join(dir, "timeline.xml"))
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExportTimelineEmpty(t *testing.T) {
	dir := t.TempDir()
	c := &Case{
		Name:   "test-case",
		Path:   dir,
		events: []TimelineEvent{},
	}

	outPath := filepath.Join(dir, "timeline.csv")
	if err := c.ExportTimeline("csv", outPath); err != nil {
		t.Fatalf("ExportTimeline empty: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 { // header only
		t.Fatalf("expected 1 line (header only), got %d:\n%s", len(lines), string(data))
	}
}
