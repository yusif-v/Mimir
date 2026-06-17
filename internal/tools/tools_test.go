package tools_test

import (
	"testing"

	"github.com/yusif-v/mimir/internal/events"
	"github.com/yusif-v/mimir/internal/tools"
)

func TestRegister(t *testing.T) {
	bus := events.NewBus()
	reg := tools.NewRegistry(bus)

	tool := &tools.Definition{
		Name:        "test-tool",
		Description: "A test tool",
	}
	reg.Register(tool)

	got, ok := reg.Get("test-tool")
	if !ok {
		t.Fatal("expected to find registered tool")
	}
	if got.Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got '%s'", got.Name)
	}
}

func TestGetMissing(t *testing.T) {
	bus := events.NewBus()
	reg := tools.NewRegistry(bus)
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Fatal("expected false for missing tool")
	}
}

func TestList(t *testing.T) {
	bus := events.NewBus()
	reg := tools.NewRegistry(bus)

	reg.Register(&tools.Definition{Name: "b-tool", Category: "cat1"})
	reg.Register(&tools.Definition{Name: "a-tool", Category: "cat2"})

	all := reg.List("")
	if len(all) != 2 {
		t.Errorf("expected 2 tools, got %d", len(all))
	}
}

func TestFilterByCategory(t *testing.T) {
	bus := events.NewBus()
	reg := tools.NewRegistry(bus)

	reg.Register(&tools.Definition{Name: "t1", Category: "forensics"})
	reg.Register(&tools.Definition{Name: "t2", Category: "network"})

	filtered := reg.List("forensics")
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool, got %d", len(filtered))
	}
	if filtered[0].Name != "t1" {
		t.Errorf("expected 't1', got '%s'", filtered[0].Name)
	}
}

func TestRunsInDocker(t *testing.T) {
	dockerTool := &tools.Definition{Name: "vol", DockerImage: "dfir-vol"}
	if !dockerTool.RunsInDocker() {
		t.Error("expected RunsInDocker() to return true")
	}

	localTool := &tools.Definition{Name: "ls", LocalCmd: "ls"}
	if localTool.RunsInDocker() {
		t.Error("expected RunsInDocker() to return false")
	}
}

func TestResultSuccess(t *testing.T) {
	r := &tools.Result{ReturnCode: 0}
	if !r.Success() {
		t.Error("expected Success() to return true")
	}

	r2 := &tools.Result{ReturnCode: 1}
	if r2.Success() {
		t.Error("expected Success() to return false")
	}
}
