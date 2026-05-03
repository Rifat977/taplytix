package store

import (
	"sort"
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

// DefaultLogCapacity is the per-service log ring buffer size.
const DefaultLogCapacity = 1000

// DefaultEventTrackerCapacity is the size of the per-service activity ring
// used to compute events/s and error rate.
const DefaultEventTrackerCapacity = 4096

// ServiceStatus is a snapshot of a service's recent activity, used by the
// Services panel and status bar.
type ServiceStatus struct {
	LastSeen        time.Time
	EventsPerSecond float64
	ErrorRate       float64
	Connected       bool
}

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
	mu       sync.RWMutex
	metrics  map[string]*Series // key: metric name (with labels folded in)
	traces   *TraceMap
	logs     *Ring[model.LogEvent]
	events   *Ring[time.Time]
	errors   *Ring[time.Time]
	lastSeen time.Time
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
	st.recordEvent(timeOrNow(e.Timestamp), false)
}

func (s *Store) PushSpan(e model.SpanEvent) {
	st := s.serviceFor(e.Service)
	st.traces.Add(e)
	st.recordEvent(timeOrNow(e.StartTime.Add(e.Duration)), e.Status == model.StatusError)
}

func (s *Store) PushLog(e model.LogEvent) {
	st := s.serviceFor(e.Service)
	st.logs.Push(e)
	st.recordEvent(timeOrNow(e.Timestamp), e.Level == model.LevelError)
}

func (st *serviceState) recordEvent(ts time.Time, isErr bool) {
	st.mu.Lock()
	if ts.After(st.lastSeen) {
		st.lastSeen = ts
	}
	st.mu.Unlock()
	st.events.Push(ts)
	if isErr {
		st.errors.Push(ts)
	}
}

func timeOrNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
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
		events:  NewRing[time.Time](DefaultEventTrackerCapacity),
		errors:  NewRing[time.Time](DefaultEventTrackerCapacity),
	}
	s.services[name] = st
	return st
}

// ServiceStatus returns a snapshot of the service's activity. Connected is
// true when the most recent event arrived within 10 seconds. Counts are
// computed over the last 5 seconds.
func (s *Store) ServiceStatus(name string) ServiceStatus {
	st, ok := s.lookup(name)
	if !ok {
		return ServiceStatus{}
	}
	now := time.Now()
	cutoff := now.Add(-5 * time.Second)
	var ev, er int
	for _, t := range st.events.Slice() {
		if t.After(cutoff) {
			ev++
		}
	}
	for _, t := range st.errors.Slice() {
		if t.After(cutoff) {
			er++
		}
	}
	st.mu.RLock()
	last := st.lastSeen
	st.mu.RUnlock()
	rate := 0.0
	if ev > 0 {
		rate = float64(er) / float64(ev)
	}
	return ServiceStatus{
		LastSeen:        last,
		EventsPerSecond: float64(ev) / 5.0,
		ErrorRate:       rate,
		Connected:       !last.IsZero() && now.Sub(last) <= 10*time.Second,
	}
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
