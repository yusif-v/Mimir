package cases_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/events"
)

func newTestManager(t *testing.T) (*cases.Manager, string) {
	t.Helper()
	tmpDir := t.TempDir()
	bus := events.NewBus()
	mgr := cases.NewManager(tmpDir, bus, nil, nil)
	return mgr, tmpDir
}

func TestCreate(t *testing.T) {
	mgr, _ := newTestManager(t)
	c, err := mgr.Create("test-case")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if c.Name != "test-case" {
		t.Errorf("expected name 'test-case', got '%s'", c.Name)
	}
	if c.Status != "open" {
		t.Errorf("expected status 'open', got '%s'", c.Status)
	}
}

func TestCreateDuplicate(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Create("test-case")
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}
	_, err = mgr.Create("test-case")
	if err == nil {
		t.Fatal("expected error for duplicate case, got nil")
	}
}

func TestOpen(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Create("test-case")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	c, err := mgr.Open("test-case")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if mgr.Current() != c {
		t.Error("Current() should return the opened case")
	}
}

func TestOpenNonexistent(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Open("does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent case")
	}
}

func TestClose(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create("test-case")
	mgr.Open("test-case")
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if mgr.Current() != nil {
		t.Error("Current() should be nil after close")
	}
}

func TestCloseNoCase(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Close()
	if err == nil {
		t.Fatal("expected error when closing with no open case")
	}
}

func TestList(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create("case-a")
	mgr.Create("case-b")
	cases, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(cases) != 2 {
		t.Errorf("expected 2 cases, got %d", len(cases))
	}
}

func TestModelSerialization(t *testing.T) {
	tmpDir := t.TempDir()
	caseDir := filepath.Join(tmpDir, "test")
	os.MkdirAll(caseDir, 0755)

	c := cases.NewCase("test", caseDir)
	if err := c.AddNote("test note", "analyst"); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	c.AddToolUsage("volatility")
	if err := c.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := cases.LoadCase(caseDir)
	if err != nil {
		t.Fatalf("LoadCase failed: %v", err)
	}
	if loaded.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", loaded.Name)
	}
	if len(loaded.Notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(loaded.Notes))
	}
	if len(loaded.ToolsUsed) != 1 {
		t.Errorf("expected 1 tool, got %d", len(loaded.ToolsUsed))
	}
}

func TestScaffold(t *testing.T) {
	tmpDir := t.TempDir()
	caseDir := filepath.Join(tmpDir, "test")
	c := cases.NewCase("test", caseDir)
	if err := c.Scaffold(); err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}

	for _, subdir := range []string{"evidence", "output", "notes"} {
		path := filepath.Join(caseDir, subdir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", subdir)
		}
	}
}

func TestManagerNamesListsDirsWithoutLoading(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := mgr.Create("alpha"); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := mgr.Create("beta"); err != nil {
		t.Fatalf("create beta: %v", err)
	}
	names, err := mgr.Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("got %v, want [alpha beta]", names)
	}
}

func TestManagerNamesEmpty(t *testing.T) {
	mgr, _ := newTestManager(t)
	names, err := mgr.Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("got %v, want empty", names)
	}
}

func TestCreateRejectsPathTraversal(t *testing.T) {
	badNames := []string{
		"../escape",
		"a/../escape",
		"a/b",
		"/abs/path",
		"..",
		".",
		"",
		`..\escape`,
		"a\\b",
	}
	for _, name := range badNames {
		mgr, base := newTestManager(t)
		if _, err := mgr.Create(name); err == nil {
			t.Errorf("Create(%q): expected error, got nil", name)
		}
		// Nothing may be created outside the base path.
		escaped := filepath.Join(filepath.Dir(base), "escape")
		if _, err := os.Stat(escaped); !os.IsNotExist(err) {
			t.Errorf("Create(%q): traversal created %s", name, escaped)
		}
	}
}

func TestOpenRejectsPathTraversal(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create("good")
	for _, name := range []string{"../escape", "a/../good", "/abs/good", ".."} {
		if _, err := mgr.Open(name); err == nil {
			t.Errorf("Open(%q): expected error, got nil", name)
		}
	}
}

func TestLoadRejectsPathTraversal(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create("good")
	for _, name := range []string{"../escape", "a/../good", "/abs/good", ".."} {
		if _, err := mgr.Load(name); err == nil {
			t.Errorf("Load(%q): expected error, got nil", name)
		}
	}
}

func TestValidateCaseName(t *testing.T) {
	good := []string{"case-1", "incident_42", "CaseName", "a.b.c"}
	for _, name := range good {
		if err := cases.ValidateCaseName(name); err != nil {
			t.Errorf("ValidateCaseName(%q): unexpected error %v", name, err)
		}
	}
	bad := []string{"", ".", "..", "../x", "a/b", "a\\b", "/abs", "a/.."}
	for _, name := range bad {
		if err := cases.ValidateCaseName(name); err == nil {
			t.Errorf("ValidateCaseName(%q): expected error, got nil", name)
		}
	}
}

func TestCloseCaseByName(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create("case-a")
	c, err := mgr.CloseCase("case-a")
	if err != nil {
		t.Fatalf("CloseCase failed: %v", err)
	}
	if c.Status != "closed" {
		t.Errorf("expected status 'closed', got '%s'", c.Status)
	}
	if mgr.Current() != nil {
		t.Error("CloseCase should not disturb the current-case pointer")
	}

	// Reload from disk to confirm the status persisted.
	reloaded, err := mgr.Load("case-a")
	if err != nil {
		t.Fatalf("Load after close: %v", err)
	}
	if reloaded.Status != "closed" {
		t.Errorf("reloaded status = %q, want 'closed'", reloaded.Status)
	}
}

func TestCloseCaseNonexistent(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := mgr.CloseCase("nope"); err == nil {
		t.Fatal("expected error for nonexistent case")
	}
}

func TestCloseRejectsTraversal(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := mgr.CloseCase("../escape"); err == nil {
		t.Fatal("CloseCase with traversal name: expected error, got nil")
	}
}

func TestSaveIsAtomic(t *testing.T) {
	mgr, _ := newTestManager(t)
	c, err := mgr.Create("atomic-case")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	c.Metadata["k"] = "v"
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(c.Path)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "case.json" && e.Name() != "evidence" && e.Name() != "output" && e.Name() != "notes" {
			t.Errorf("unexpected leftover file after atomic Save: %q", e.Name())
		}
	}

	// Reload preserves the metadata (temp+rename didn't clobber anything).
	reloaded, err := mgr.Load("atomic-case")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reloaded.Metadata["k"]; got != "v" {
		t.Errorf("metadata k = %q, want 'v'", got)
	}
}
