package panels

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/store"
)

func seedLogs(st *store.Store) {
	now := time.Now()
	st.PushLog(model.LogEvent{Timestamp: now.Add(-3 * time.Second), Level: model.LevelInfo, Body: "request ok", Service: "api", Attrs: map[string]string{"path": "/health"}})
	st.PushLog(model.LogEvent{Timestamp: now.Add(-2 * time.Second), Level: model.LevelError, Body: "db timeout", Service: "api"})
	st.PushLog(model.LogEvent{Timestamp: now.Add(-1 * time.Second), Level: model.LevelWarn, Body: "slow query", Service: "worker"})
}

func TestLogsPanelRefresh(t *testing.T) {
	st := store.New()
	seedLogs(st)

	p := NewLogs(st)
	p.SetSize(120, 20)
	p.refresh()

	if p.totalCount != 3 || p.shownCount != 3 {
		t.Errorf("counts = %d/%d, want 3/3", p.shownCount, p.totalCount)
	}
	view := p.View()
	if !strings.Contains(view, "db timeout") {
		t.Errorf("view missing error line: %q", view)
	}
}

func TestLogsPanelLevelFilter(t *testing.T) {
	st := store.New()
	seedLogs(st)

	p := NewLogs(st)
	p.SetSize(120, 20)
	p.refresh()

	// Press / then type "level:ERROR"
	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p = updated.(*LogsPanel)
	for _, r := range "level:ERROR" {
		updated, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		p = updated.(*LogsPanel)
	}

	if p.shownCount != 1 {
		t.Errorf("shown after level:ERROR = %d, want 1", p.shownCount)
	}
}

func TestLogsPanelServiceFilter(t *testing.T) {
	st := store.New()
	seedLogs(st)

	p := NewLogs(st)
	p.SetSize(120, 20)
	p.refresh()

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p = updated.(*LogsPanel)
	for _, r := range "service:worker" {
		updated, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		p = updated.(*LogsPanel)
	}
	if p.shownCount != 1 {
		t.Errorf("shown after service:worker = %d, want 1", p.shownCount)
	}
}

func TestLogsPanelBodyFilter(t *testing.T) {
	st := store.New()
	seedLogs(st)

	p := NewLogs(st)
	p.SetSize(120, 20)
	p.refresh()

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p = updated.(*LogsPanel)
	for _, r := range "timeout" {
		updated, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		p = updated.(*LogsPanel)
	}
	if p.shownCount != 1 {
		t.Errorf("shown after 'timeout' = %d, want 1", p.shownCount)
	}
}

func TestLogsPanelEmptyState(t *testing.T) {
	p := NewLogs(store.New())
	p.SetSize(120, 20)
	p.refresh()
	if p.View() == "" {
		t.Errorf("expected non-empty view")
	}
}
