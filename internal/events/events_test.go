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

func TestOffRemovesOnlyTargetedHandler(t *testing.T) {
	bus := events.NewBus()
	a, b := 0, 0

	idA := bus.On(events.CaseCreated, func(e events.Event) { a++ })
	bus.On(events.CaseCreated, func(e events.Event) { b++ })

	bus.Off(events.CaseCreated, idA)
	bus.Emit(events.CaseCreated, nil)

	if a != 0 {
		t.Errorf("handler A was removed but still fired %d time(s)", a)
	}
	if b != 1 {
		t.Errorf("handler B should have fired once, got %d", b)
	}
}

func TestOffLastHandlerClearsEvent(t *testing.T) {
	bus := events.NewBus()
	id := bus.On(events.CaseCreated, func(e events.Event) {})
	bus.Off(events.CaseCreated, id)
	if bus.HasHandlers(events.CaseCreated) {
		t.Error("expected no handlers after removing the last one")
	}
}

func TestOffUnknownIDIsNoOp(t *testing.T) {
	bus := events.NewBus()
	bus.On(events.CaseCreated, func(e events.Event) {})
	// Removing an ID that was never issued must not panic or drop real handlers.
	bus.Off(events.CaseCreated, 9999)
	if !bus.HasHandlers(events.CaseCreated) {
		t.Error("removing unknown ID should not affect existing handlers")
	}
}
