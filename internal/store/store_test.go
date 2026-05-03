package store

import (
	"sort"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

func TestStoreNamespacedByService(t *testing.T) {
	s := New()
	s.PushMetric(model.MetricEvent{Name: "cpu", Value: 1, Source: "svc-a", Timestamp: time.Now()})
	s.PushMetric(model.MetricEvent{Name: "cpu", Value: 2, Source: "svc-b", Timestamp: time.Now()})
	s.PushSpan(model.SpanEvent{TraceID: "t1", SpanID: "r", Service: "svc-a", StartTime: time.Now()})
	s.PushLog(model.LogEvent{Body: "hi", Service: "svc-b", Timestamp: time.Now()})

	svcs := s.Services()
	sort.Strings(svcs)
	if len(svcs) != 2 || svcs[0] != "svc-a" || svcs[1] != "svc-b" {
		t.Fatalf("services = %v", svcs)
	}

	if m := s.MetricsFor("svc-a"); len(m) != 1 || m["cpu"].Last() != 1 {
		t.Errorf("svc-a metrics = %+v", m)
	}
	if m := s.MetricsFor("svc-b"); len(m) != 1 || m["cpu"].Last() != 2 {
		t.Errorf("svc-b metrics = %+v", m)
	}
	if tr := s.TracesFor("svc-a"); tr == nil || tr.Len() != 1 {
		t.Errorf("svc-a trace count wrong")
	}
	if logs := s.LogsFor("svc-b"); len(logs) != 1 || logs[0].Body != "hi" {
		t.Errorf("svc-b logs = %+v", logs)
	}
	if logs := s.LogsFor("svc-a"); len(logs) != 0 {
		t.Errorf("svc-a should have no logs, got %+v", logs)
	}
}

func TestStoreMetricKeyDistinguishesLabels(t *testing.T) {
	s := New()
	s.PushMetric(model.MetricEvent{Name: "req", Value: 1, Source: "svc", Labels: map[string]string{"path": "/a"}})
	s.PushMetric(model.MetricEvent{Name: "req", Value: 2, Source: "svc", Labels: map[string]string{"path": "/b"}})
	s.PushMetric(model.MetricEvent{Name: "req", Value: 3, Source: "svc", Labels: map[string]string{"path": "/a"}})

	m := s.MetricsFor("svc")
	if len(m) != 2 {
		t.Fatalf("expected 2 distinct series, got %d: %+v", len(m), m)
	}
	if got := m["req{path=/a}"].Last(); got != 3 {
		t.Errorf("req{/a} last = %v, want 3", got)
	}
	if got := m["req{path=/b}"].Last(); got != 2 {
		t.Errorf("req{/b} last = %v, want 2", got)
	}
}
