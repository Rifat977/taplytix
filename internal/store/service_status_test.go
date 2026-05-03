package store

import (
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

func TestServiceStatusReflectsRecentActivity(t *testing.T) {
	s := New()
	now := time.Now()
	for i := 0; i < 10; i++ {
		s.PushMetric(model.MetricEvent{Name: "x", Value: 1, Source: "svc", Timestamp: now})
	}
	s.PushSpan(model.SpanEvent{TraceID: "t", SpanID: "r", Service: "svc", StartTime: now, Duration: time.Millisecond, Status: model.StatusError})

	st := s.ServiceStatus("svc")
	if !st.Connected {
		t.Errorf("expected Connected = true")
	}
	if st.EventsPerSecond <= 0 {
		t.Errorf("EventsPerSecond = %v, want > 0", st.EventsPerSecond)
	}
	if st.ErrorRate == 0 {
		t.Errorf("ErrorRate = %v, want > 0", st.ErrorRate)
	}
}

func TestServiceStatusUnknownService(t *testing.T) {
	s := New()
	st := s.ServiceStatus("nope")
	if st.Connected || !st.LastSeen.IsZero() {
		t.Errorf("status for unknown service = %+v, want zero", st)
	}
}

func TestServiceStatusStaleAfterTimeout(t *testing.T) {
	s := New()
	old := time.Now().Add(-30 * time.Second)
	s.PushLog(model.LogEvent{Body: "old", Service: "svc", Timestamp: old, Level: model.LevelInfo})
	st := s.ServiceStatus("svc")
	if st.Connected {
		t.Errorf("expected Connected = false for stale activity")
	}
}
