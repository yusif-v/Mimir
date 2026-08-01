package cases

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/sandbox"
)

// Manager handles case lifecycle.
type Manager struct {
	storage        *Storage
	events         *events.Bus
	current        *Case
	sandboxMgr     *sandbox.Manager
	sandboxRuntime *sandbox.Runtime
}

// NewManager creates a new case manager.
func NewManager(basePath string, bus *events.Bus, sandboxMgr *sandbox.Manager, sandboxRuntime *sandbox.Runtime) *Manager {
	return &Manager{
		storage:        NewStorage(basePath),
		events:         bus,
		sandboxMgr:     sandboxMgr,
		sandboxRuntime: sandboxRuntime,
	}
}

// ValidateCaseName ensures name can be safely joined onto the storage base
// path. It rejects empty names, absolute paths, path separators, "." / "..",
// and any name that would escape the base directory.
func ValidateCaseName(name string) error {
	if name == "" {
		return fmt.Errorf("case name is required")
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("case name %q must be relative", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("case name %q must be a single directory name", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("case name %q is not allowed", name)
	}
	clean := filepath.Clean(name)
	if clean != name || strings.HasPrefix(clean, "..") {
		return fmt.Errorf("case name %q is not allowed", name)
	}
	return nil
}

// casePath validates name and returns its path under the storage base.
func (m *Manager) casePath(name string) (string, error) {
	if err := ValidateCaseName(name); err != nil {
		return "", err
	}
	return filepath.Join(m.storage.basePath, name), nil
}

// Create creates a new case.
func (m *Manager) Create(name string) (*Case, error) {
	path, err := m.casePath(name)
	if err != nil {
		return nil, err
	}
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
	path, err := m.casePath(name)
	if err != nil {
		return nil, err
	}
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

// Load opens an existing case without side effects. Unlike Open it does not
// change the case status, chdir, append a "case_opened" timeline event, or
// emit events. It is intended for read-oriented consumers such as the MCP
// server, which resolves a case by name on every tool call and must not
// mutate the timeline or working directory in the process.
func (m *Manager) Load(name string) (*Case, error) {
	path, err := m.casePath(name)
	if err != nil {
		return nil, err
	}
	c, err := LoadCase(path)
	if err != nil {
		return nil, fmt.Errorf("case '%s' not found", name)
	}
	m.current = c
	return c, nil
}

// Close closes the current case. Any sandboxes still running for the case are
// torn down automatically — this is non-interactive: there is no destroy
// prompt and closing never deletes the case or its evidence. Sandbox teardown
// errors are joined and returned so callers can surface them. The process
// working directory is left untouched.
func (m *Manager) Close() error {
	if m.current == nil {
		return fmt.Errorf("no case is open")
	}
	c := m.current
	if err := m.closeSandboxes(c.Name); err != nil {
		return err
	}
	if err := m.markClosed(c); err != nil {
		return err
	}
	name := c.Name
	m.current = nil
	m.events.Emit(events.CaseClosed, map[string]any{"name": name})
	return nil
}

// CloseCase closes the named case without disturbing the current-case pointer.
// It is used by read-oriented consumers such as the MCP server, which resolve
// a case by name on every tool call. Like Close it is non-interactive, tears
// down running sandboxes, and never deletes the case.
func (m *Manager) CloseCase(name string) (*Case, error) {
	path, err := m.casePath(name)
	if err != nil {
		return nil, err
	}
	c, err := LoadCase(path)
	if err != nil {
		return nil, fmt.Errorf("case '%s' not found", name)
	}
	if err := m.closeSandboxes(c.Name); err != nil {
		return nil, err
	}
	if err := m.markClosed(c); err != nil {
		return nil, err
	}
	m.events.Emit(events.CaseClosed, map[string]any{"name": c.Name})
	return c, nil
}

// closeSandboxes stops and removes any running sandbox containers for a case
// and destroys their sandbox records. Errors are joined so one failure does
// not prevent teardown of the rest. A nil sandbox manager (or runtime) is a
// no-op, which keeps Close usable in tests that don't provision sandboxes.
func (m *Manager) closeSandboxes(caseName string) error {
	if m.sandboxMgr == nil {
		return nil
	}
	var errs []error
	for _, s := range m.sandboxMgr.List(caseName) {
		if s.ContainerID != "" && m.sandboxRuntime != nil {
			if err := m.sandboxRuntime.StopContainer(s.ContainerID); err != nil {
				errs = append(errs, fmt.Errorf("stop sandbox %s: %w", s.ID, err))
			}
			if err := m.sandboxRuntime.RemoveContainer(s.ContainerID); err != nil {
				errs = append(errs, fmt.Errorf("remove sandbox %s: %w", s.ID, err))
			}
		}
		if err := m.sandboxMgr.Destroy(s.ID); err != nil {
			errs = append(errs, fmt.Errorf("destroy sandbox %s: %w", s.ID, err))
		}
	}
	return errors.Join(errs...)
}

// markClosed sets the case status to closed, persists it, and records the
// close event on the timeline.
func (m *Manager) markClosed(c *Case) error {
	c.Status = "closed"
	c.ClosedAt = time.Now().Format(time.RFC3339)
	if err := c.Save(); err != nil {
		return fmt.Errorf("save case: %w", err)
	}
	if err := c.AppendEvent(TimelineEvent{
		Type:      "case_closed",
		Timestamp: time.Now().Format(time.RFC3339),
		Payload:   map[string]any{},
	}); err != nil {
		return fmt.Errorf("record close event: %w", err)
	}
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
