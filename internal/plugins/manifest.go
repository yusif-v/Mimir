package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Manifest struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Version     string `toml:"version"`
	Entrypoint  string `toml:"entrypoint"`
}

func LoadManifest(dir string) (*Plugin, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.toml"))
	if err != nil {
		return nil, fmt.Errorf("read plugin.toml: %w", err)
	}
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse plugin.toml: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("plugin.toml: name is required")
	}
	if m.Entrypoint == "" {
		m.Entrypoint = "run"
	}
	return &Plugin{
		Name:        m.Name,
		Description: m.Description,
		Version:     m.Version,
		Entrypoint:  m.Entrypoint,
		Enabled:     true,
	}, nil
}
