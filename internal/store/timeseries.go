package store

import (
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

// DefaultSeriesCapacity is 300 samples — five minutes at 1s resolution.
const DefaultSeriesCapacity = 300

// Series is a sliding window of metric values for a single (name, label-set).
type Series struct {
	Name   string
	Labels map[string]string
	Kind   model.MetricKind

	values *Ring[float64]

	mu       sync.RWMutex
	last     float64
	lastTime time.Time
}

func NewSeries(name string, kind model.MetricKind, labels map[string]string, capacity int) *Series {
	if capacity <= 0 {
		capacity = DefaultSeriesCapacity
	}
	return &Series{
		Name:   name,
		Labels: labels,
		Kind:   kind,
		values: NewRing[float64](capacity),
	}
}

func (s *Series) Push(e model.MetricEvent) {
	s.values.Push(e.Value)
	s.mu.Lock()
	s.last = e.Value
	s.lastTime = e.Timestamp
	s.mu.Unlock()
}

func (s *Series) Last() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.last
}

func (s *Series) LastTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastTime
}

func (s *Series) Len() int { return s.values.Len() }

func (s *Series) Values() []float64 { return s.values.Slice() }

func (s *Series) P50() float64 { return Percentile(s.Values(), 50) }
func (s *Series) P95() float64 { return Percentile(s.Values(), 95) }
func (s *Series) P99() float64 { return Percentile(s.Values(), 99) }

// Sparkline returns the most recent n samples (or all if fewer exist), oldest first.
func (s *Series) Sparkline(n int) []float64 {
	all := s.values.Slice()
	if n <= 0 || n >= len(all) {
		return all
	}
	return all[len(all)-n:]
}
