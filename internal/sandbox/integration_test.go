package sandbox

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSandboxFullLifecycleDryRun tests the full sandbox lifecycle using dry-run mode.
// This test does not require Docker and validates the create → list → report → destroy flow.
func TestSandboxFullLifecycleDryRun(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	// Create
	var buf bytes.Buffer
	err := sh.Create(&buf, []string{"--os", "ubuntu-22.04"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(buf.String(), "created") {
		t.Errorf("Create output missing 'created': %s", buf.String())
	}

	// List
	buf.Reset()
	err = sh.List(&buf, []string{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(buf.String(), "ubuntu-22.04") {
		t.Errorf("List output missing OS: %s", buf.String())
	}

	// Get sandbox ID from manager
	sandboxes := mgr.List("current-case")
	if len(sandboxes) == 0 {
		t.Fatal("no sandboxes found after create")
	}
	id := sandboxes[0].ID

	// Verify sandbox was persisted to disk
	path := filepath.Join(dir, id+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected sandbox JSON file at %s", path)
	}

	// Report
	buf.Reset()
	err = sh.Report(&buf, []string{id})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	reportOutput := buf.String()
	if !strings.Contains(reportOutput, "ubuntu-22.04") {
		t.Errorf("Report output missing OS: %s", reportOutput)
	}
	if !strings.Contains(reportOutput, "created") {
		t.Errorf("Report output missing status: %s", reportOutput)
	}
	if !strings.Contains(reportOutput, id) {
		t.Errorf("Report output missing sandbox ID: %s", reportOutput)
	}

	// Destroy
	buf.Reset()
	err = sh.Destroy(&buf, []string{id})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !strings.Contains(buf.String(), "destroyed") {
		t.Errorf("Destroy output missing confirmation: %s", buf.String())
	}

	// Verify sandbox is no longer in active list
	sandboxes = mgr.List("current-case")
	if len(sandboxes) != 0 {
		t.Errorf("expected 0 active sandboxes after destroy, got %d", len(sandboxes))
	}

	// Verify sandbox is no longer retrievable
	_, ok := mgr.Get(id)
	if ok {
		t.Error("expected sandbox to be removed from active map after destroy")
	}
}

// TestSandboxFullLifecycle tests the full sandbox lifecycle with a real Docker runtime.
// Skips if Docker is not available.
func TestSandboxFullLifecycle(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available")
	}

	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := NewRuntime()
	sh := NewShell(mgr, runtime)

	// Create
	var buf bytes.Buffer
	err := sh.Create(&buf, []string{"--os", "ubuntu-22.04"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// List
	buf.Reset()
	err = sh.List(&buf, []string{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(buf.String(), "ubuntu-22.04") {
		t.Errorf("List output missing OS: %s", buf.String())
	}

	// Get sandbox ID from list
	sandboxes := mgr.List("current-case")
	if len(sandboxes) == 0 {
		t.Fatal("no sandboxes found")
	}
	id := sandboxes[0].ID

	// Report
	buf.Reset()
	err = sh.Report(&buf, []string{id})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !strings.Contains(buf.String(), "ubuntu-22.04") {
		t.Errorf("Report output missing OS: %s", buf.String())
	}

	// Destroy
	buf.Reset()
	err = sh.Destroy(&buf, []string{id})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !strings.Contains(buf.String(), "destroyed") {
		t.Errorf("Destroy output missing confirmation: %s", buf.String())
	}
}

// TestSandboxMultipleCreateAndDestroy tests creating multiple sandboxes
// and destroying them individually.
func TestSandboxMultipleCreateAndDestroy(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	// Create two sandboxes
	var buf bytes.Buffer
	err := sh.Create(&buf, []string{"--os", "ubuntu-22.04"})
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}

	buf.Reset()
	err = sh.Create(&buf, []string{"--os", "ubuntu-22.04"})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	// Verify both exist
	sandboxes := mgr.List("current-case")
	if len(sandboxes) != 2 {
		t.Fatalf("expected 2 sandboxes, got %d", len(sandboxes))
	}

	// Destroy first
	buf.Reset()
	err = sh.Destroy(&buf, []string{sandboxes[0].ID})
	if err != nil {
		t.Fatalf("Destroy 1: %v", err)
	}

	// Verify one remains
	sandboxes = mgr.List("current-case")
	if len(sandboxes) != 1 {
		t.Fatalf("expected 1 sandbox after first destroy, got %d", len(sandboxes))
	}

	// Destroy second
	buf.Reset()
	err = sh.Destroy(&buf, []string{sandboxes[0].ID})
	if err != nil {
		t.Fatalf("Destroy 2: %v", err)
	}

	// Verify none remain
	sandboxes = mgr.List("current-case")
	if len(sandboxes) != 0 {
		t.Fatalf("expected 0 sandboxes after second destroy, got %d", len(sandboxes))
	}
}

// TestSandboxDispatch tests the Dispatch routing method for all subcommands.
func TestSandboxDispatch(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	// Test create via dispatch
	var buf bytes.Buffer
	err := sh.Dispatch(&buf, []string{"create", "--os", "ubuntu-22.04"})
	if err != nil {
		t.Fatalf("Dispatch create: %v", err)
	}
	if !strings.Contains(buf.String(), "created") {
		t.Errorf("Dispatch create output: %s", buf.String())
	}

	// Test list via dispatch
	buf.Reset()
	err = sh.Dispatch(&buf, []string{"list"})
	if err != nil {
		t.Fatalf("Dispatch list: %v", err)
	}
	if !strings.Contains(buf.String(), "ubuntu-22.04") {
		t.Errorf("Dispatch list output: %s", buf.String())
	}

	// Get ID
	sandboxes := mgr.List("current-case")
	if len(sandboxes) == 0 {
		t.Fatal("no sandboxes found")
	}
	id := sandboxes[0].ID

	// Test report via dispatch
	buf.Reset()
	err = sh.Dispatch(&buf, []string{"report", id})
	if err != nil {
		t.Fatalf("Dispatch report: %v", err)
	}
	if !strings.Contains(buf.String(), id) {
		t.Errorf("Dispatch report output: %s", buf.String())
	}

	// Test destroy via dispatch
	buf.Reset()
	err = sh.Dispatch(&buf, []string{"destroy", id})
	if err != nil {
		t.Fatalf("Dispatch destroy: %v", err)
	}
	if !strings.Contains(buf.String(), "destroyed") {
		t.Errorf("Dispatch destroy output: %s", buf.String())
	}
}

// TestSandboxDispatchInvalidCommand tests that Dispatch returns an error for unknown commands.
func TestSandboxDispatchInvalidCommand(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	var buf bytes.Buffer
	err := sh.Dispatch(&buf, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand, got nil")
	}
}

// TestSandboxPersistenceAfterDestroy tests that destroyed sandboxes are
// persisted to disk with destroyed status but removed from active list.
func TestSandboxPersistenceAfterDestroy(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	runtime := &Runtime{dryRun: true}
	sh := NewShell(mgr, runtime)

	// Create
	var buf bytes.Buffer
	err := sh.Create(&buf, []string{"--os", "ubuntu-22.04"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sandboxes := mgr.List("current-case")
	if len(sandboxes) == 0 {
		t.Fatal("no sandboxes found")
	}
	id := sandboxes[0].ID

	// Destroy
	buf.Reset()
	err = sh.Destroy(&buf, []string{id})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	// Verify JSON file still exists on disk
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading destroyed sandbox file: %v", err)
	}
	if !strings.Contains(string(data), "destroyed") {
		t.Errorf("destroyed sandbox file missing 'destroyed' status: %s", string(data))
	}

	// Load into a new manager — destroyed sandbox should be loaded but not listed
	mgr2 := NewManager(dir)
	err = mgr2.Load("current-case")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The destroyed sandbox should be loadable via Get
	s, ok := mgr2.Get(id)
	if !ok {
		t.Fatal("expected destroyed sandbox to be loadable")
	}
	if s.Status != SandboxDestroyed {
		t.Errorf("expected destroyed status, got %q", s.Status)
	}

	// But should not appear in List (which filters out destroyed)
	list := mgr2.List("current-case")
	if len(list) != 0 {
		t.Errorf("expected 0 active sandboxes after load, got %d", len(list))
	}
}
