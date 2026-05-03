package store

import "sync"

// Ring is a fixed-capacity circular buffer that overwrites the oldest entry
// when full. It is safe for concurrent use.
type Ring[T any] struct {
	mu    sync.RWMutex
	buf   []T
	head  int  // next write index
	count int  // number of valid entries (≤ cap)
	cap   int
}

func NewRing[T any](capacity int) *Ring[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring[T]{buf: make([]T, capacity), cap: capacity}
}

func (r *Ring[T]) Push(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head] = v
	r.head = (r.head + 1) % r.cap
	if r.count < r.cap {
		r.count++
	}
}

// Slice returns a copy of the buffer contents in oldest-to-newest order.
func (r *Ring[T]) Slice() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]T, r.count)
	if r.count == 0 {
		return out
	}
	start := (r.head - r.count + r.cap) % r.cap
	for i := 0; i < r.count; i++ {
		out[i] = r.buf[(start+i)%r.cap]
	}
	return out
}

func (r *Ring[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

func (r *Ring[T]) Cap() int { return r.cap }
