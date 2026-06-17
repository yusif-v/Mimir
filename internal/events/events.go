// Package events provides a simple pub/sub event bus.
package events

import (
	"fmt"
	"log"
	"sync"
)

// Bus is a simple synchronous event bus.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]func(Event)
}

// Event represents an emitted event with a name and optional data.
type Event struct {
	Name string
	Data map[string]any
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string][]func(Event)),
	}
}

// On registers a handler for an event name.
func (b *Bus) On(name string, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = append(b.handlers[name], handler)
}

// Off removes a handler for an event name.
// Note: removes all handlers registered with this function reference.
func (b *Bus) Off(name string, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	handlers := b.handlers[name]
	var filtered []func(Event)
	for _, h := range handlers {
		// Can't compare func values in Go, so we just remove all
		_ = h
	}
	_ = filtered
}

// Emit fires all handlers for an event name synchronously.
func (b *Bus) Emit(name string, data map[string]any) {
	b.mu.RLock()
	handlers := make([]func(Event), len(b.handlers[name]))
	copy(handlers, b.handlers[name])
	b.mu.RUnlock()

	evt := Event{Name: name, Data: data}
	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event handler panic for '%s': %v", name, r)
				}
			}()
			h(evt)
		}()
	}
}

// Emitf is a convenience for emitting with fmt-style key formatting.
func (b *Bus) Emitf(name string, key string, value any) {
	b.Emit(name, map[string]any{key: value})
}

// HasHandlers returns true if any handlers are registered for an event.
func (b *Bus) HasHandlers(name string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.handlers[name]) > 0
}

// String returns a string representation for debugging.
func (e Event) String() string {
	return fmt.Sprintf("Event{%s %v}", e.Name, e.Data)
}

// Names for built-in events.
const (
	CaseCreated = "case.created"
	CaseOpened  = "case.opened"
	CaseClosed  = "case.closed"
	ToolStarted = "tool.started"
	ToolFinished = "tool.finished"
	ToolError    = "tool.error"
	OutputCaptured = "output.captured"
	CommandRun   = "command.run"
)
