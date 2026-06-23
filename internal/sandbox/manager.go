package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manager manages sandbox lifecycle and persistence.
type Manager struct {
	storagePath string
	sandboxes   map[string]*Sandbox
}

// NewManager creates a new sandbox manager that stores state in the given directory.
func NewManager(storagePath string) *Manager {
	return &Manager{
		storagePath: storagePath,
		sandboxes:   make(map[string]*Sandbox),
	}
}

// Create creates a new sandbox for the given case and persists it.
func (m *Manager) Create(caseID, os, image, targetFile string, timeout int) *Sandbox {
	s := NewSandbox(fmt.Sprintf("sandbox-%s-%d", caseID, len(m.sandboxes)+1), caseID, os, image, targetFile, timeout)
	m.sandboxes[s.ID] = s
	m.save(s)
	return s
}

// Get retrieves a sandbox by ID.
func (m *Manager) Get(id string) (*Sandbox, bool) {
	s, ok := m.sandboxes[id]
	return s, ok
}

// List returns all non-destroyed sandboxes for a case.
func (m *Manager) List(caseID string) []*Sandbox {
	var result []*Sandbox
	for _, s := range m.sandboxes {
		if s.CaseID == caseID && s.Status != SandboxDestroyed {
			result = append(result, s)
		}
	}
	return result
}

// Destroy marks a sandbox as destroyed, persists the state, and removes it from the active list.
func (m *Manager) Destroy(id string) error {
	s, ok := m.sandboxes[id]
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	s.Status = SandboxDestroyed
	s.DestroyedAt = time.Now().Format(time.RFC3339)
	m.save(s)
	delete(m.sandboxes, id)
	return nil
}

// Update updates a sandbox in memory and persists it.
func (m *Manager) Update(s *Sandbox) {
	m.sandboxes[s.ID] = s
	m.save(s)
}

// save writes a sandbox to a JSON file in the storage directory.
func (m *Manager) save(s *Sandbox) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(m.storagePath, s.ID+".json")
	return os.WriteFile(path, data, 0644)
}

// Load reads all sandbox JSON files from storage for the given case.
func (m *Manager) Load(caseID string) error {
	entries, err := os.ReadDir(m.storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.storagePath, entry.Name()))
		if err != nil {
			continue
		}
		var s Sandbox
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.CaseID == caseID {
			m.sandboxes[s.ID] = &s
		}
	}
	return nil
}
