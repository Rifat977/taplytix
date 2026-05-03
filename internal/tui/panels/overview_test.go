package panels

import (
	"strings"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/config"
	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/store"
)

func TestOverviewRefreshAggregatesSpans(t *testing.T) {
	st := store.New()
	now := time.Now()
	st.PushSpan(model.SpanEvent{
		TraceID: "t1", SpanID: "r", Service: "svc",
		Name: "GET /a", StartTime: now, Duration: 100 * time.Millisecond,
	})
	st.PushSpan(model.SpanEvent{
		TraceID: "t2", SpanID: "r", Service: "svc",
		Name: "GET /a", StartTime: now, Duration: 200 * time.Millisecond, Status: model.StatusError,
	})
	st.PushSpan(model.SpanEvent{
		TraceID: "t3", SpanID: "r", Service: "svc",
		Name: "GET /b", StartTime: now, Duration: 10 * time.Millisecond,
	})

	cfg := config.Default()
	cfg.Server.DefaultService = "svc"
	p := NewOverview(cfg, st)
	p.SetSize(120, 40)
	p.refresh()

	if p.spans != 3 {
		t.Errorf("spans = %d, want 3", p.spans)
	}
	if p.p99 == 0 {
		t.Errorf("expected P99 > 0, got %v", p.p99)
	}
	if p.p50 == 0 {
		t.Errorf("expected P50 > 0, got %v", p.p50)
	}
	rows := p.tbl.Rows()
	if len(rows) != 2 {
		t.Fatalf("table rows = %d, want 2", len(rows))
	}
	// Sorted by P95 descending: GET /a (slower) should come first.
	if !strings.Contains(rows[0][0], "/a") {
		t.Errorf("top row = %v, expected /a first", rows[0])
	}
}

func TestOverviewRefreshHeapMetric(t *testing.T) {
	st := store.New()
	now := time.Now()
	st.PushMetric(model.MetricEvent{
		Name: "process.runtime.go.mem.heap_inuse",
		Value: 250_000_000, Source: "svc", Kind: model.Gauge, Timestamp: now,
	})
	cfg := config.Default()
	cfg.Server.DefaultService = "svc"
	p := NewOverview(cfg, st)
	p.SetSize(120, 40)
	p.refresh()
	if p.heap != 250_000_000 {
		t.Errorf("heap = %v, want 250000000", p.heap)
	}
}

func TestOverviewEmptyStateRenders(t *testing.T) {
	st := store.New()
	cfg := config.Default()
	cfg.Server.DefaultService = "svc"
	p := NewOverview(cfg, st)
	p.SetSize(120, 40)
	p.refresh()
	if got := p.View(); got == "" {
		t.Errorf("expected non-empty view in empty state")
	}
}
