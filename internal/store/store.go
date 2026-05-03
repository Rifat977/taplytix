package store

import (
	"sort"
	"sync"

	"github.com/rifat977/taplytix/internal/model"
)

// DefaultLogCapacity is the per-service log ring buffer size.
const DefaultLogCapacity = 1000

// Store is the unified read/write entry point used by the TUI. It is keyed
// by service name; each service has its own metric series, trace map, and
// log ring.
type Store struct {
	mu       sync.RWMutex
	services map[string]*serviceState

	seriesCap int
	logCap    int
}

type serviceState struct {
	mu      sync.RWMutex
	metrics map[string]*Series // key: metric name (with labels folded in)
	traces  *TraceMap
	logs    *Ring[model.LogEvent]
}

func New() *Store {
	return &Store{
		services:  make(map[string]*serviceState),
		seriesCap: DefaultSeriesCapacity,
		logCap:    DefaultLogCapacity,
	}
}

func (s *Store) PushMetric(e model.MetricEvent) {
	st := s.serviceFor(e.Source)
	key := metricKey(e.Name, e.Labels)
	st.mu.Lock()
	series, ok := st.metrics[key]
	if !ok {
		series = NewSeries(e.Name, e.Kind, e.Labels, s.seriesCap)
		st.metrics[key] = series
	}
	st.mu.Unlock()
	series.Push(e)
}

func (s *Store) PushSpan(e model.SpanEvent) {
	st := s.serviceFor(e.Service)
	st.traces.Add(e)
}

func (s *Store) PushLog(e model.LogEvent) {
	st := s.serviceFor(e.Service)
	st.logs.Push(e)
}

func (s *Store) MetricsFor(service string) map[string]*Series {
	st, ok := s.lookup(service)
	if !ok {
		return nil
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make(map[string]*Series, len(st.metrics))
	for k, v := range st.metrics {
		out[k] = v
	}
	return out
}

func (s *Store) TracesFor(service string) *TraceMap {
	st, ok := s.lookup(service)
	if !ok {
		return nil
	}
	return st.traces
}

func (s *Store) LogsFor(service string) []model.LogEvent {
	st, ok := s.lookup(service)
	if !ok {
		return nil
	}
	return st.logs.Slice()
}

func (s *Store) Services() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.services))
	for name := range s.services {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (s *Store) serviceFor(name string) *serviceState {
	if name == "" {
		name = "unknown"
	}
	s.mu.RLock()
	st, ok := s.services[name]
	s.mu.RUnlock()
	if ok {
		return st
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok = s.services[name]; ok {
		return st
	}
	st = &serviceState{
		metrics: make(map[string]*Series),
		traces:  NewTraceMap(),
		logs:    NewRing[model.LogEvent](s.logCap),
	}
	s.services[name] = st
	return st
}

func (s *Store) lookup(name string) (*serviceState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.services[name]
	return st, ok
}

// metricKey folds metric name + sorted labels into a stable string key.
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := name
	for _, k := range keys {
		out += "{" + k + "=" + labels[k] + "}"
	}
	return out
}
