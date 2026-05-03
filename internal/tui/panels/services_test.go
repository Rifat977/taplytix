package panels

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/store"
)

func seedTwoServices(st *store.Store) {
	now := time.Now()
	st.PushSpan(model.SpanEvent{
		TraceID: "t1", SpanID: "r", Service: "svc-a",
		Name: "GET /a", StartTime: now, Duration: 50 * time.Millisecond,
	})
	st.PushSpan(model.SpanEvent{
		TraceID: "t2", SpanID: "r", Service: "svc-b",
		Name: "GET /b", StartTime: now, Duration: 200 * time.Millisecond, Status: model.StatusError,
	})
	st.PushMetric(model.MetricEvent{
		Name: "process.runtime.go.mem.heap_inuse", Value: 100_000_000,
		Source: "svc-b", Kind: model.Gauge, Timestamp: now,
	})
}

func TestServicesPanelRefreshListsAllServices(t *testing.T) {
	st := store.New()
	seedTwoServices(st)

	p := NewServices(st, "svc-a")
	p.SetSize(140, 30)
	p.refresh()

	if len(p.services) != 2 {
		t.Fatalf("services = %d, want 2", len(p.services))
	}
	view := p.View()
	if !strings.Contains(view, "svc-a") || !strings.Contains(view, "svc-b") {
		t.Errorf("view missing service names: %q", view)
	}
}

func TestServicesPanelEnterEmitsServiceChangedMsg(t *testing.T) {
	st := store.New()
	seedTwoServices(st)

	p := NewServices(st, "svc-a")
	p.SetSize(140, 30)
	p.refresh()

	// Move down to svc-b (sorted alphabetically: svc-a, svc-b)
	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyDown})
	p = updated.(*ServicesPanel)

	updated, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	p = updated.(*ServicesPanel)
	if cmd == nil {
		t.Fatalf("expected cmd from Enter, got nil")
	}
	msg := cmd()
	sc, ok := msg.(ServiceChangedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want ServiceChangedMsg", msg)
	}
	if sc.Service != "svc-b" {
		t.Errorf("ServiceChangedMsg.Service = %q, want svc-b", sc.Service)
	}
}

func TestServicesPanelEmptyState(t *testing.T) {
	p := NewServices(store.New(), "")
	p.SetSize(120, 20)
	p.refresh()
	if p.View() == "" {
		t.Errorf("expected non-empty view")
	}
}
