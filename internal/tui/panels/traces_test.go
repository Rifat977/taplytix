package panels

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/store"
)

func makeTrace(t *testing.T, st *store.Store, service string) string {
	t.Helper()
	now := time.Now()
	root := model.SpanEvent{
		TraceID: "trace-x", SpanID: "root", ParentID: "",
		Name: "GET /api", Service: service, StartTime: now, Duration: 100 * time.Millisecond,
	}
	child := model.SpanEvent{
		TraceID: "trace-x", SpanID: "child-a", ParentID: "root",
		Name: "db.query", Service: service, StartTime: now.Add(10 * time.Millisecond),
		Duration: 250 * time.Millisecond, Status: model.StatusError,
	}
	grand := model.SpanEvent{
		TraceID: "trace-x", SpanID: "grand", ParentID: "child-a",
		Name: "redis.get", Service: service, StartTime: now.Add(15 * time.Millisecond),
		Duration: 5 * time.Millisecond,
	}
	for _, s := range []model.SpanEvent{root, child, grand} {
		st.PushSpan(s)
	}
	return root.TraceID
}

func TestTracesPanelRefreshPopulatesList(t *testing.T) {
	st := store.New()
	makeTrace(t, st, "svc-a")

	p := NewTraces(st)
	p.SetSize(120, 40)
	p.refresh()

	if len(p.traces) != 1 {
		t.Fatalf("traces collected = %d, want 1", len(p.traces))
	}
	if p.traces[0].service != "svc-a" {
		t.Errorf("service = %q, want svc-a", p.traces[0].service)
	}
	view := p.View()
	if !strings.Contains(view, "GET /api") {
		t.Errorf("view missing root span name: %q", view)
	}
	if !strings.Contains(view, "db.query") {
		t.Errorf("view missing child span name: %q", view)
	}
}

func TestTracesPanelEnterTogglesExpanded(t *testing.T) {
	st := store.New()
	makeTrace(t, st, "svc-a")

	p := NewTraces(st)
	p.SetSize(120, 40)
	p.refresh()

	if p.expanded {
		t.Fatal("expected initial state to be collapsed")
	}
	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	p = updated.(*TracesPanel)
	if !p.expanded {
		t.Errorf("Enter did not expand panel")
	}
	updated, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	p = updated.(*TracesPanel)
	if p.expanded {
		t.Errorf("Esc did not collapse panel")
	}
}

func TestTracesPanelEmptyState(t *testing.T) {
	p := NewTraces(store.New())
	p.SetSize(80, 24)
	p.refresh()
	if got := p.View(); got == "" {
		t.Errorf("expected non-empty view in empty state")
	}
}
