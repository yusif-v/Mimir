package events_test

import (
	"testing"

	"github.com/yusif-v/mimir/internal/events"
)

func TestOnAndEmit(t *testing.T) {
	bus := events.NewBus()
	called := false

	bus.On(events.CaseCreated, func(e events.Event) {
		called = true
	})

	bus.Emit(events.CaseCreated, nil)
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestMultipleHandlers(t *testing.T) {
	bus := events.NewBus()
	count := 0

	bus.On(events.CaseCreated, func(e events.Event) { count++ })
	bus.On(events.CaseCreated, func(e events.Event) { count++ })

	bus.Emit(events.CaseCreated, nil)
	if count != 2 {
		t.Errorf("expected 2 handler calls, got %d", count)
	}
}

func TestEmitWithData(t *testing.T) {
	bus := events.NewBus()
	var gotName string

	bus.On(events.CaseCreated, func(e events.Event) {
		if name, ok := e.Data["name"].(string); ok {
			gotName = name
		}
	})

	bus.Emit(events.CaseCreated, map[string]any{"name": "test-case"})
	if gotName != "test-case" {
		t.Errorf("expected 'test-case', got '%s'", gotName)
	}
}

func TestNoHandlers(t *testing.T) {
	bus := events.NewBus()
	// Should not panic
	bus.Emit("nonexistent.event", nil)
}

func TestHasHandlers(t *testing.T) {
	bus := events.NewBus()
	if bus.HasHandlers(events.CaseCreated) {
		t.Error("expected no handlers initially")
	}
	bus.On(events.CaseCreated, func(e events.Event) {})
	if !bus.HasHandlers(events.CaseCreated) {
		t.Error("expected handlers after registration")
	}
}
