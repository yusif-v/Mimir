package cases

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/yusif-v/mimir/internal/events"
)

// Manager handles case lifecycle.
type Manager struct {
	storage *Storage
	events  *events.Bus
	current *Case
}

// NewManager creates a new case manager.
func NewManager(basePath string, bus *events.Bus) *Manager {
	return &Manager{
		storage: NewStorage(basePath),
		events:  bus,
	}
}

// Create creates a new case.
func (m *Manager) Create(name string) (*Case, error) {
	path := filepath.Join(m.storage.basePath, name)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("case '%s' already exists", name)
	}

	c := NewCase(name, path)
	if err := c.Scaffold(); err != nil {
		return nil, fmt.Errorf("scaffold case: %w", err)
	}
	if err := c.Save(); err != nil {
		return nil, fmt.Errorf("save case: %w", err)
	}

	m.events.Emit(events.CaseCreated, map[string]any{"case": c})
	return c, nil
}

// CreateWithTemplate creates a new case using the named template.
func (m *Manager) CreateWithTemplate(name, templateName string) (*Case, error) {
	c, err := m.Create(name)
	if err != nil {
		return nil, err
	}
	tmpl, err := LoadTemplate(templateName)
	if err != nil {
		return nil, err
	}
	if err := c.ApplyTemplate(tmpl); err != nil {
		return nil, err
	}
	return c, nil
}

// Open opens an existing case and sets it as current.
func (m *Manager) Open(name string) (*Case, error) {
	path := filepath.Join(m.storage.basePath, name)
	c, err := LoadCase(path)
	if err != nil {
		return nil, fmt.Errorf("case '%s' not found", name)
	}

	c.Status = "open"
	c.OpenedAt = time.Now().Format(time.RFC3339)
	if err := c.Save(); err != nil {
		return nil, fmt.Errorf("save case: %w", err)
	}

	m.current = c
	os.Chdir(path)
	if err := c.AppendEvent(TimelineEvent{
		Type:      "case_opened",
		Timestamp: time.Now().Format(time.RFC3339),
		Payload:   map[string]any{},
	}); err != nil {
		return nil, fmt.Errorf("record open event: %w", err)
	}
	m.events.Emit(events.CaseOpened, map[string]any{"case": c})
	return c, nil
}

// Close closes the current case.
func (m *Manager) Close() error {
	if m.current == nil {
		return fmt.Errorf("no case is open")
	}

	m.current.Status = "closed"
	m.current.ClosedAt = time.Now().Format(time.RFC3339)
	if err := m.current.Save(); err != nil {
		return fmt.Errorf("save case: %w", err)
	}

	if err := m.current.AppendEvent(TimelineEvent{
		Type:      "case_closed",
		Timestamp: time.Now().Format(time.RFC3339),
		Payload:   map[string]any{},
	}); err != nil {
		return fmt.Errorf("record close event: %w", err)
	}

	name := m.current.Name
	m.current = nil
	os.Chdir(os.Getenv("HOME"))
	m.events.Emit(events.CaseClosed, map[string]any{"name": name})
	return nil
}

// Current returns the active case.
func (m *Manager) Current() *Case {
	return m.current
}

// List returns all cases, sorted by creation time.
func (m *Manager) List() ([]*Case, error) {
	return m.storage.List()
}

// Names returns case names without loading each case's metadata or timeline.
// A case's directory name is its name, so this is a cheap directory listing —
// suitable for hot paths like completion that only need names.
func (m *Manager) Names() ([]string, error) {
	return m.storage.Names()
}

// Storage provides filesystem-based case persistence.
type Storage struct {
	basePath string
}

// NewStorage creates new filesystem storage.
func NewStorage(basePath string) *Storage {
	return &Storage{basePath: basePath}
}

// List returns all cases in the storage directory.
func (s *Storage) List() ([]*Case, error) {
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cases []*Case
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		c, err := LoadCase(filepath.Join(s.basePath, entry.Name()))
		if err != nil {
			continue
		}
		cases = append(cases, c)
	}

	sort.Slice(cases, func(i, j int) bool {
		return cases[i].CreatedAt < cases[j].CreatedAt
	})
	return cases, nil
}

// Names lists case directory names without reading case files. Sorted
// alphabetically for stable completion ordering.
func (s *Storage) Names() ([]string, error) {
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
