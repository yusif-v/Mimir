// Package catalog provides an embedded set of installable tool templates.
package catalog

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/yusif-v/mimir/internal/tools"
)

//go:embed templates
var templatesFS embed.FS

// Entry is one installable tool from the embedded catalog.
type Entry struct {
	Name        string
	Description string
	Category    string
	Tags        []string
}

var (
	listOnce     sync.Once
	listCached   []Entry
	listCacheErr error
)

// List returns all catalog entries, parsed from each templates/<name>/mimir.toml.
// The embedded catalog is immutable at runtime, so the parse happens once and
// the result is cached; callers (e.g. the REPL completer on every keystroke)
// get a fresh copy without re-reading or re-parsing the TOML.
func List() ([]Entry, error) {
	listOnce.Do(func() {
		listCached, listCacheErr = parseEntries()
	})
	if listCacheErr != nil {
		return nil, listCacheErr
	}
	return append([]Entry(nil), listCached...), nil
}

// parseEntries reads and parses every templates/<name>/mimir.toml once.
func parseEntries() ([]Entry, error) {
	dirs, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	var entries []Entry
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		tomlPath := "templates/" + d.Name() + "/mimir.toml"
		data, err := templatesFS.ReadFile(tomlPath)
		if err != nil {
			return nil, fmt.Errorf("read catalog %q: %w", d.Name(), err)
		}
		def, err := tools.ParseTemplate(data, tomlPath)
		if err != nil {
			return nil, fmt.Errorf("parse catalog %q: %w", d.Name(), err)
		}
		entries = append(entries, Entry{
			Name:        def.Name,
			Description: def.Description,
			Category:    def.Category,
			Tags:        def.Tags,
		})
	}
	return entries, nil
}

// Get returns a single entry by name.
func Get(name string) (Entry, bool) {
	entries, err := List()
	if err != nil {
		return Entry{}, false
	}
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// Install copies templates/<name>/* into destDir.
func Install(name, destDir string) error {
	srcDir := "templates/" + name
	if _, err := fs.Stat(templatesFS, srcDir); err != nil {
		return fmt.Errorf("unknown tool %q: %w", name, err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}
	entries, err := fs.ReadDir(templatesFS, srcDir)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := templatesFS.ReadFile(srcDir + "/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(destDir, e.Name()), data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", e.Name(), err)
		}
	}
	return nil
}
