package bus

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultSubscriberBuffer is the per-subscriber channel capacity. When a
// subscriber falls behind, Publish drops the event for that subscriber
// instead of blocking the entire pipeline.
const DefaultSubscriberBuffer = 1000

// Bus is a non-blocking pub/sub fan-out. The receiver pipeline publishes;
// the store, alert engine, and any other consumers subscribe.
type Bus struct {
	mu          sync.RWMutex
	subscribers []chan any
	program     *tea.Program
	bufSize     int
}

func New() *Bus {
	return &Bus{bufSize: DefaultSubscriberBuffer}
}

// Publish fans out to every subscriber. Slow subscribers drop events.
func (b *Bus) Publish(event any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// Subscribe returns a buffered channel. Callers must drain it; falling
// behind means lost events for that subscriber, never a stalled pipeline.
func (b *Bus) Subscribe() <-chan any {
	ch := make(chan any, b.bufSize)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// SetProgram wires the Bus to a Bubble Tea program so Dispatch can deliver
// messages directly into the TUI's update loop.
func (b *Bus) SetProgram(p *tea.Program) {
	b.mu.Lock()
	b.program = p
	b.mu.Unlock()
}

// Dispatch sends a tea.Msg to the connected Bubble Tea program. It is a
// no-op when no program has been registered (e.g. during tests).
func (b *Bus) Dispatch(msg tea.Msg) {
	b.mu.RLock()
	p := b.program
	b.mu.RUnlock()
	if p == nil {
		return
	}
	p.Send(msg)
}

// Close shuts down all subscriber channels. Once closed the bus is unusable.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = nil
}
