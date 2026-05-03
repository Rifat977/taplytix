package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rifat977/taplytix/internal/bus"
	"github.com/rifat977/taplytix/internal/config"
	"github.com/rifat977/taplytix/internal/store"
	"github.com/rifat977/taplytix/internal/tui/panels"
)

func newTestApp() *AppModel {
	cfg := config.Default()
	ps := []panels.Panel{
		panels.NewPlaceholder("Overview"),
		panels.NewPlaceholder("Traces"),
		panels.NewPlaceholder("Metrics"),
		panels.NewPlaceholder("Logs"),
		panels.NewPlaceholder("Services"),
	}
	return NewApp(cfg, store.New(), bus.New(), ps)
}

func TestAppTabSwitching(t *testing.T) {
	app := newTestApp()
	if app.ActiveIndex() != 0 {
		t.Fatalf("active = %d, want 0", app.ActiveIndex())
	}
	tab := tea.KeyMsg{Type: tea.KeyTab}
	for i := 1; i < 5; i++ {
		updated, _ := app.Update(tab)
		app = updated.(*AppModel)
		if app.ActiveIndex() != i {
			t.Errorf("after %d tabs active = %d, want %d", i, app.ActiveIndex(), i)
		}
	}
	updated, _ := app.Update(tab) // wrap around
	app = updated.(*AppModel)
	if app.ActiveIndex() != 0 {
		t.Errorf("wrap-around active = %d, want 0", app.ActiveIndex())
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	app = updated.(*AppModel)
	if app.ActiveIndex() != 4 {
		t.Errorf("shift+tab active = %d, want 4", app.ActiveIndex())
	}
}

func TestAppQuitKey(t *testing.T) {
	app := newTestApp()
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
	} {
		_, cmd := app.Update(msg)
		if cmd == nil {
			t.Errorf("expected quit cmd for %+v, got nil", msg)
			continue
		}
		if got := cmd(); got == nil {
			t.Errorf("quit cmd returned nil msg")
		} else if _, ok := got.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", got)
		}
	}
}

func TestAppResizeUpdatesPanels(t *testing.T) {
	app := newTestApp()
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app = updated.(*AppModel)
	if app.width != 120 || app.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", app.width, app.height)
	}
	view := app.View()
	if view == "" {
		t.Errorf("view empty after resize")
	}
}
