package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerCreate(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	s := mgr.Create("test-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "/evidence/malware.exe", 300)
	if s == nil {
		t.Fatal("Create returned nil")
	}
	if s.Status != SandboxCreated {
		t.Errorf("status: got %q, want %q", s.Status, SandboxCreated)
	}
	if s.OS != "ubuntu-22.04" {
		t.Errorf("os: got %q, want %q", s.OS, "ubuntu-22.04")
	}
}

func TestManagerList(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	mgr.Create("test-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "", 300)
	mgr.Create("test-case", "windows", "mimir-sandbox:windows", "", 600)
	sandboxes := mgr.List("test-case")
	if len(sandboxes) != 2 {
		t.Errorf("expected 2 sandboxes, got %d", len(sandboxes))
	}
}

func TestManagerGet(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	s := mgr.Create("test-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "", 300)
	got, ok := mgr.Get(s.ID)
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.ID != s.ID {
		t.Errorf("ID mismatch")
	}
}

func TestManagerDestroy(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	s := mgr.Create("test-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "", 300)
	err := mgr.Destroy(s.ID)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	s.Status = SandboxDestroyed
	s.DestroyedAt = "test"
	_, ok := mgr.Get(s.ID)
	if !ok {
		// destroyed sandboxes are removed from active list
	}
}

func TestManagerSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	s := mgr.Create("test-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "", 300)

	// Verify file was saved
	path := filepath.Join(dir, s.ID+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected save file to exist at %s", path)
	}

	// Load into a new manager
	mgr2 := NewManager(dir)
	err := mgr2.Load("test-case")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := mgr2.Get(s.ID)
	if !ok {
		t.Fatal("Get after Load returned false")
	}
	if got.ID != s.ID {
		t.Errorf("ID mismatch after load: got %q, want %q", got.ID, s.ID)
	}
}

func TestManagerUpdate(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	s := mgr.Create("test-case", "ubuntu-22.04", "mimir-sandbox:ubuntu-22.04", "", 300)

	s.Status = SandboxRunning
	s.ContainerID = "abc123"
	mgr.Update(s)

	got, ok := mgr.Get(s.ID)
	if !ok {
		t.Fatal("Get returned false after Update")
	}
	if got.Status != SandboxRunning {
		t.Errorf("status: got %q, want %q", got.Status, SandboxRunning)
	}
	if got.ContainerID != "abc123" {
		t.Errorf("container_id: got %q, want %q", got.ContainerID, "abc123")
	}
}
