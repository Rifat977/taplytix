package store

import (
	"sort"
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

// TraceMap assembles incoming SpanEvents into Trace trees keyed by TraceID.
// It is safe for concurrent use.
type TraceMap struct {
	mu     sync.RWMutex
	traces map[string]*model.Trace
}

func NewTraceMap() *TraceMap {
	return &TraceMap{traces: make(map[string]*model.Trace)}
}

// Add inserts a span into its trace, creating the trace on first sight.
// A span with empty ParentID becomes the root; otherwise it is appended to
// the parent's child list. The trace's Duration tracks the latest span end.
func (m *TraceMap) Add(span model.SpanEvent) {
	if span.TraceID == "" {
		return
	}
	s := span // copy to take stable address
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.traces[s.TraceID]
	if !ok {
		t = &model.Trace{
			TraceID:  s.TraceID,
			Children: make(map[string][]*model.SpanEvent),
		}
		m.traces[s.TraceID] = t
	}
	if isRoot(s) {
		t.Root = &s
	} else {
		t.Children[s.ParentID] = append(t.Children[s.ParentID], &s)
	}
	end := s.StartTime.Add(s.Duration)
	if t.Root != nil {
		traceEnd := traceLatestEnd(t)
		t.Duration = traceEnd.Sub(t.Root.StartTime)
	} else if t.Duration < end.Sub(s.StartTime) {
		t.Duration = end.Sub(s.StartTime)
	}
}

func (m *TraceMap) Get(traceID string) (*model.Trace, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.traces[traceID]
	return t, ok
}

// Recent returns up to n traces ordered newest-first by root start time.
func (m *TraceMap) Recent(n int) []*model.Trace {
	m.mu.RLock()
	all := make([]*model.Trace, 0, len(m.traces))
	for _, t := range m.traces {
		all = append(all, t)
	}
	m.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool {
		return traceStart(all[i]).After(traceStart(all[j]))
	})
	if n > 0 && len(all) > n {
		all = all[:n]
	}
	return all
}

// Evict drops traces whose latest span ended more than ttl ago.
func (m *TraceMap) Evict(ttl time.Duration) int {
	cutoff := time.Now().Add(-ttl)
	m.mu.Lock()
	defer m.mu.Unlock()
	var removed int
	for id, t := range m.traces {
		if traceLatestEnd(t).Before(cutoff) {
			delete(m.traces, id)
			removed++
		}
	}
	return removed
}

func (m *TraceMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.traces)
}

func isRoot(s model.SpanEvent) bool {
	if s.ParentID == "" {
		return true
	}
	for _, c := range s.ParentID {
		if c != '0' {
			return false
		}
	}
	return true
}

func traceStart(t *model.Trace) time.Time {
	if t.Root != nil {
		return t.Root.StartTime
	}
	for _, kids := range t.Children {
		for _, c := range kids {
			return c.StartTime
		}
	}
	return time.Time{}
}

func traceLatestEnd(t *model.Trace) time.Time {
	var latest time.Time
	if t.Root != nil {
		latest = t.Root.StartTime.Add(t.Root.Duration)
	}
	for _, kids := range t.Children {
		for _, c := range kids {
			end := c.StartTime.Add(c.Duration)
			if end.After(latest) {
				latest = end
			}
		}
	}
	return latest
}
