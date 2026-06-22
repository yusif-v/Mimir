package cases

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExportTimeline writes the case timeline to a file in the given format.
// Supported formats: "csv" (default), "json".
// If path is empty, defaults to <case>/output/timeline.<ext>.
func (c *Case) ExportTimeline(format, path string) error {
	if path == "" {
		ext := format
		if ext == "" {
			ext = "csv"
		}
		path = filepath.Join(c.Path, "output", "timeline."+ext)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	switch format {
	case "csv", "":
		return c.exportTimelineCSV(f)
	case "json":
		return c.exportTimelineJSON(f)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func (c *Case) exportTimelineCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"timestamp", "type", "details"}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, ev := range c.events {
		details, _ := json.Marshal(ev.Payload)
		if err := cw.Write([]string{ev.Timestamp, ev.Type, string(details)}); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func (c *Case) exportTimelineJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(c.events)
}
