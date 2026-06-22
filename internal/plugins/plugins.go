// Package plugins manages plugin discovery and lifecycle.
package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yusif-v/mimir/internal/events"
)

// API is the public surface exposed to plugins.
type API struct {
	events *events.Bus
	hooks  map[string][]func(events.Event)
}

// NewAPI creates a new plugin API.
func NewAPI(bus *events.Bus) *API {
	return &API{
		events: bus,
		hooks:  make(map[string][]func(events.Event)),
	}
}

// On subscribes to an event.
func (a *API) On(event string, handler func(events.Event)) {
	a.events.On(event, handler)
}

// Emit fires an event.
func (a *API) Emit(event string, data map[string]any) {
	a.events.Emit(event, data)
}

// RegisterHook registers a callback for a named hook.
func (a *API) RegisterHook(name string, callback func(events.Event)) {
	a.hooks[name] = append(a.hooks[name], callback)
}

// FireHook calls all callbacks registered for a hook.
func (a *API) FireHook(name string, evt events.Event) {
	for _, cb := range a.hooks[name] {
		cb(evt)
	}
}

// Manager handles plugin discovery and loading.
type Manager struct {
	api      *API
	plugins  map[string]*Plugin
}

// NewManager creates a new plugin manager.
func NewManager(bus *events.Bus) *Manager {
	return &Manager{
		api:     NewAPI(bus),
		plugins: make(map[string]*Plugin),
	}
}

// Discover finds and loads plugins.
func (m *Manager) Discover() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	pluginsDir := filepath.Join(home, ".mimir", "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(pluginsDir, entry.Name())
		p, err := LoadManifest(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: plugin %s: %v\n", entry.Name(), err)
			continue
		}
		m.Register(p)
	}
}

// Register adds a plugin.
func (m *Manager) Register(p *Plugin) {
	m.plugins[p.Name] = p
}

// Get returns a plugin by name.
func (m *Manager) Get(name string) (*Plugin, bool) {
	p, ok := m.plugins[name]
	return p, ok
}

// List returns all plugin names.
func (m *Manager) List() []string {
	var names []string
	for name := range m.plugins {
		names = append(names, name)
	}
	return names
}

// Plugin represents a loaded plugin.
type Plugin struct {
	Name        string
	Description string
	Version     string
	Entrypoint  string
	Enabled     bool
}
