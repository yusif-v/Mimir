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
	handlers map[string]map[int]func(Event)
	nextID   int
}

// Event represents an emitted event with a name and optional data.
type Event struct {
	Name string
	Data map[string]any
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string]map[int]func(Event)),
	}
}

// On registers a handler for an event name and returns a subscription ID
// that can be passed to Off to remove exactly this handler.
func (b *Bus) On(name string, handler func(Event)) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.handlers[name] == nil {
		b.handlers[name] = make(map[int]func(Event))
	}
	b.nextID++
	id := b.nextID
	b.handlers[name][id] = handler
	return id
}

// Off removes the handler registered under the given subscription ID for an
// event name. Removing an unknown ID is a no-op.
func (b *Bus) Off(name string, id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if hs := b.handlers[name]; hs != nil {
		delete(hs, id)
		if len(hs) == 0 {
			delete(b.handlers, name)
		}
	}
}

// Emit fires all handlers for an event name synchronously.
func (b *Bus) Emit(name string, data map[string]any) {
	b.mu.RLock()
	handlers := make([]func(Event), 0, len(b.handlers[name]))
	for _, h := range b.handlers[name] {
		handlers = append(handlers, h)
	}
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
	CaseCreated    = "case.created"
	CaseOpened     = "case.opened"
	CaseClosed     = "case.closed"
	ToolStarted    = "tool.started"
	ToolFinished   = "tool.finished"
	ToolError      = "tool.error"
	OutputCaptured = "output.captured"
	CommandRun     = "command.run"
)
