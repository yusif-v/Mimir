package cases

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// Template defines a case template with directory structure, default tags,
// initial notes, and recommended tools.
type Template struct {
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description"`
	Directories      []string `yaml:"directories"`
	DefaultTags      []string `yaml:"default_tags"`
	InitialNotes     []string `yaml:"initial_notes"`
	RecommendedTools []string `yaml:"recommended_tools"`
}

// LoadTemplate loads a template from the embedded filesystem by name.
// The name should be the template filename without the .yaml extension.
func LoadTemplate(name string) (*Template, error) {
	data, err := templateFS.ReadFile(fmt.Sprintf("templates/%s.yaml", name))
	if err != nil {
		return nil, fmt.Errorf("template %q not found: %w", name, err)
	}
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse template %q: %w", name, err)
	}
	return &t, nil
}

// ListTemplates returns the names of all embedded template files.
func ListTemplates() ([]string, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ApplyTemplate creates the template directories and adds initial notes.
// It is called after Scaffold() so the case directory already exists.
func (c *Case) ApplyTemplate(tmpl *Template) error {
	for _, dir := range tmpl.Directories {
		path := filepath.Join(c.Path, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", path, err)
		}
	}
	for _, note := range tmpl.InitialNotes {
		if err := c.AddNote(note, "system"); err != nil {
			return err
		}
	}
	return nil
}
