package cases_test

import (
	"os"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
)

func TestLoadBuiltinTemplate(t *testing.T) {
	tmpl, err := cases.LoadTemplate("default")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if tmpl.Name != "default" {
		t.Errorf("name: got %q, want %q", tmpl.Name, "default")
	}
	if len(tmpl.Directories) == 0 {
		t.Error("expected directories in default template")
	}
}

func TestLoadTemplateNotFound(t *testing.T) {
	_, err := cases.LoadTemplate("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestListTemplates(t *testing.T) {
	names, err := cases.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("expected at least one template")
	}
	found := false
	for _, n := range names {
		if n == "default.yaml" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("default.yaml not found in templates: %v", names)
	}
}

func TestIncidentResponseTemplate(t *testing.T) {
	tmpl, err := cases.LoadTemplate("incident-response")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if tmpl.Name != "incident-response" {
		t.Errorf("name: got %q, want %q", tmpl.Name, "incident-response")
	}
	if len(tmpl.Directories) == 0 {
		t.Error("expected directories in incident-response template")
	}
	if len(tmpl.InitialNotes) == 0 {
		t.Error("expected initial notes in incident-response template")
	}
	if len(tmpl.RecommendedTools) == 0 {
		t.Error("expected recommended tools in incident-response template")
	}
}

func TestApplyTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	c := cases.NewCase("test", tmpDir)

	tmpl := &cases.Template{
		Name:         "test-template",
		Description:  "Test template",
		Directories:  []string{"custom-dir", "another-dir"},
		InitialNotes: []string{"Initial note from template"},
	}

	if err := c.ApplyTemplate(tmpl); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	// Verify directories were created
	for _, dir := range tmpl.Directories {
		path := tmpDir + "/" + dir
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected directory %s to exist: %v", dir, err)
		}
	}

	// Verify notes were added
	if len(c.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(c.Notes))
	}
	if c.Notes[0].Content != "Initial note from template" {
		t.Errorf("note content: got %q, want %q", c.Notes[0].Content, "Initial note from template")
	}
	if c.Notes[0].Author != "system" {
		t.Errorf("note author: got %q, want %q", c.Notes[0].Author, "system")
	}
}
