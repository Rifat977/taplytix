package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/alert"
	"github.com/rifat977/taplytix/internal/bus"
	"github.com/rifat977/taplytix/internal/config"
	"github.com/rifat977/taplytix/internal/render"
	"github.com/rifat977/taplytix/internal/store"
	"github.com/rifat977/taplytix/internal/tui/panels"
)

// tickMsg fires every RefreshMs and triggers a panel refresh from the store.
type tickMsg time.Time

type AppModel struct {
	cfg       *config.Config
	store     *store.Store
	bus       *bus.Bus
	keys      KeyMap
	panels    []panels.Panel
	active    int
	statusBar StatusBar
	width     int
	height    int
	refresh   time.Duration
	source    string // first source name for top bar

	activeService string

	alerts map[string]alert.Alert // key = rule + "::" + service
}

func NewApp(cfg *config.Config, st *store.Store, b *bus.Bus, ps []panels.Panel) *AppModel {
	refresh := time.Duration(cfg.Server.RefreshMs) * time.Millisecond
	if refresh <= 0 {
		refresh = 500 * time.Millisecond
	}
	src := cfg.Server.DefaultService
	if src == "" && len(cfg.Sources) > 0 {
		src = cfg.Sources[0].Name
	}
	return &AppModel{
		cfg:           cfg,
		store:         st,
		bus:           b,
		keys:          DefaultKeyMap(),
		panels:        ps,
		statusBar:     NewStatusBar(),
		refresh:       refresh,
		source:        src,
		activeService: src,
		alerts:        make(map[string]alert.Alert),
	}
}

func alertKey(a alert.Alert) string { return a.Rule.Name + "::" + a.Service }

func (m *AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{tickEvery(m.refresh)}
	for _, p := range m.panels {
		if c := p.Init(); c != nil {
			cmds = append(cmds, c)
		}
	}
	if m.activeService != "" {
		cmds = append(cmds, func() tea.Msg {
			return panels.ServiceChangedMsg{Service: m.activeService}
		})
	}
	return tea.Batch(cmds...)
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.Width = msg.Width
		m.resizePanels()
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.NextTab):
			m.active = (m.active + 1) % len(m.panels)
			return m, nil
		case key.Matches(msg, m.keys.PrevTab):
			m.active = (m.active - 1 + len(m.panels)) % len(m.panels)
			return m, nil
		}
		return m.delegateToActive(msg)

	case tickMsg:
		refresh := panels.RefreshMsg{Time: time.Time(msg)}
		for i, p := range m.panels {
			updated, _ := p.Update(refresh)
			if pp, ok := updated.(panels.Panel); ok {
				m.panels[i] = pp
			}
		}
		return m, tickEvery(m.refresh)

	case alert.FiredMsg:
		m.alerts[alertKey(msg.Alert)] = msg.Alert
		m.statusBar.Alerts = len(m.alerts)
		return m, nil

	case alert.ResolvedMsg:
		delete(m.alerts, alertKey(msg.Alert))
		m.statusBar.Alerts = len(m.alerts)
		return m, nil

	case panels.ServiceChangedMsg:
		m.activeService = msg.Service
		m.source = msg.Service
		for i, p := range m.panels {
			updated, _ := p.Update(msg)
			if pp, ok := updated.(panels.Panel); ok {
				m.panels[i] = pp
			}
		}
		return m, nil
	}

	return m.delegateToActive(msg)
}

func (m *AppModel) delegateToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.panels) == 0 {
		return m, nil
	}
	updated, cmd := m.panels[m.active].Update(msg)
	if p, ok := updated.(panels.Panel); ok {
		m.panels[m.active] = p
	}
	return m, cmd
}

func (m *AppModel) resizePanels() {
	if len(m.panels) == 0 {
		return
	}
	// Reserve 1 line for top bar, 2 for tab bar (with border), 2 for status.
	contentHeight := m.height - 5
	if contentHeight < 1 {
		contentHeight = 1
	}
	for _, p := range m.panels {
		p.SetSize(m.width, contentHeight)
	}
}

func (m *AppModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading taplytix…"
	}
	top := render.TopBar.Render("taplytix · " + m.source)
	tabs := m.renderTabs()
	body := ""
	if len(m.panels) > 0 {
		body = m.panels[m.active].View()
	}
	status := m.statusBar.View()
	return lipgloss.JoinVertical(lipgloss.Left, top, tabs, body, status)
}

func (m *AppModel) renderTabs() string {
	parts := make([]string, len(m.panels))
	for i, p := range m.panels {
		title := p.Title()
		if i == m.active {
			parts[i] = render.TabActive.Render("[ " + title + " ]")
		} else {
			parts[i] = render.TabInactive.Render(title)
		}
	}
	row := strings.Join(parts, " ")
	return render.TabBar.Width(m.width).Render(row)
}

// ActiveIndex is exported for tests.
func (m *AppModel) ActiveIndex() int { return m.active }

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}
