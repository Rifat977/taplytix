package panels

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/store"
)

func seedMetrics(st *store.Store) {
	now := time.Now()
	for i := 0; i < 30; i++ {
		st.PushMetric(model.MetricEvent{
			Name: "http.server.duration", Value: float64(i * 10), Source: "svc",
			Kind: model.Histogram, Timestamp: now,
		})
	}
	st.PushMetric(model.MetricEvent{
		Name: "process.runtime.go.mem.heap_inuse", Value: 250_000_000,
		Source: "svc", Kind: model.Gauge, Timestamp: now,
	})
}

func TestMetricsPanelRefreshListsMetrics(t *testing.T) {
	st := store.New()
	seedMetrics(st)

	p := NewMetrics(st)
	p.SetSize(120, 30)
	p.refresh()

	if len(p.allKeys) != 2 {
		t.Fatalf("metrics = %d, want 2: %+v", len(p.allKeys), p.allKeys)
	}
	view := p.View()
	if !strings.Contains(view, "http.server.duration") {
		t.Errorf("view missing http.server.duration: %q", view)
	}
}

func TestMetricsPanelFilter(t *testing.T) {
	st := store.New()
	seedMetrics(st)

	p := NewMetrics(st)
	p.SetSize(120, 30)
	p.refresh()

	// Press / then type "heap"
	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p = updated.(*MetricsPanel)
	for _, r := range "heap" {
		updated, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		p = updated.(*MetricsPanel)
	}

	if len(p.visKeys) != 1 || !strings.Contains(p.visKeys[0].key, "heap") {
		t.Errorf("after filter visKeys = %+v, want only heap entry", p.visKeys)
	}

	// Esc clears filter
	updated, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	p = updated.(*MetricsPanel)
	if len(p.visKeys) != 2 {
		t.Errorf("after esc visKeys = %d, want 2", len(p.visKeys))
	}
}

func TestMetricsPanelEmptyState(t *testing.T) {
	p := NewMetrics(store.New())
	p.SetSize(120, 30)
	p.refresh()
	if p.View() == "" {
		t.Errorf("expected non-empty view in empty state")
	}
}
