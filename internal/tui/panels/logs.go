package panels

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/render"
	"github.com/rifat977/taplytix/internal/store"
)

// LogsPanel: scrollable, filterable, level-coloured log tail across all
// services. Auto-scrolls to the bottom on new logs unless the user has
// scrolled up; press G to jump to bottom and resume auto-scroll.
type LogsPanel struct {
	store *store.Store

	width, height int

	service string // empty = show all services

	vp        viewport.Model
	filter    textinput.Model
	filtering bool

	autoScroll bool
	totalCount int
	shownCount int
}

func NewLogs(st *store.Store) *LogsPanel {
	ti := textinput.New()
	ti.Placeholder = "filter (text · level:error · service:api)"
	ti.Prompt = "/"
	ti.CharLimit = 128

	return &LogsPanel{
		store:      st,
		vp:         viewport.New(0, 0),
		filter:     ti,
		autoScroll: true,
	}
}

func (p *LogsPanel) Title() string { return "Logs" }

func (p *LogsPanel) SetSize(w, h int) {
	p.width, p.height = w, h
	if w <= 0 || h <= 0 {
		return
	}
	p.vp.Width = w
	p.vp.Height = h - 2 // reserve filter input + status line
	if p.vp.Height < 1 {
		p.vp.Height = 1
	}
}

func (p *LogsPanel) Init() tea.Cmd { return nil }

func (p *LogsPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case RefreshMsg:
		p.refresh()
		return p, nil
	case ServiceChangedMsg:
		p.service = m.Service
		p.refresh()
		return p, nil
	case tea.KeyMsg:
		if p.filtering {
			switch m.String() {
			case "esc":
				p.filtering = false
				p.filter.SetValue("")
				p.filter.Blur()
				p.refresh()
				return p, nil
			case "enter":
				p.filtering = false
				p.filter.Blur()
				return p, nil
			}
			var cmd tea.Cmd
			p.filter, cmd = p.filter.Update(msg)
			p.refresh()
			return p, cmd
		}
		switch m.String() {
		case "/":
			p.filtering = true
			p.filter.Focus()
			return p, textinput.Blink
		case "G":
			p.autoScroll = true
			p.vp.GotoBottom()
			return p, nil
		case "up", "k", "pgup":
			p.autoScroll = false
		}
	}
	var cmd tea.Cmd
	p.vp, cmd = p.vp.Update(msg)
	// If user scrolled up beyond the bottom, pause autoscroll. If they scrolled
	// back to the bottom, resume.
	if p.vp.AtBottom() {
		p.autoScroll = true
	} else {
		p.autoScroll = false
	}
	return p, cmd
}

func (p *LogsPanel) refresh() {
	services := p.store.Services()
	if p.service != "" {
		services = []string{p.service}
	}
	var all []model.LogEvent
	for _, svc := range services {
		all = append(all, p.store.LogsFor(svc)...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.Before(all[j].Timestamp)
	})
	p.totalCount = len(all)

	body, level, service := parseFilterExpr(p.filter.Value())
	filtered := make([]model.LogEvent, 0, len(all))
	for _, ev := range all {
		if level != "" && string(ev.Level) != level {
			continue
		}
		if service != "" && !strings.EqualFold(ev.Service, service) {
			continue
		}
		if body != "" && !matchesBody(ev, body) {
			continue
		}
		filtered = append(filtered, ev)
	}
	p.shownCount = len(filtered)

	lines := make([]string, len(filtered))
	for i, ev := range filtered {
		lines[i] = renderLogLine(ev)
	}
	p.vp.SetContent(strings.Join(lines, "\n"))
	if p.autoScroll {
		p.vp.GotoBottom()
	}
}

func (p *LogsPanel) View() string {
	if p.width == 0 || p.height == 0 {
		return ""
	}
	header := render.StatusMute.Render("/ to filter · ↑/k/PgUp pause · G jump to bottom")
	if p.filtering || p.filter.Value() != "" {
		header = p.filter.View()
	}
	status := p.renderStatusLine()
	return lipgloss.JoinVertical(lipgloss.Left, header, p.vp.View(), status)
}

func (p *LogsPanel) renderStatusLine() string {
	scroll := "auto-scroll: on"
	if !p.autoScroll {
		scroll = "auto-scroll: paused"
	}
	flt := strings.TrimSpace(p.filter.Value())
	if flt == "" {
		flt = "—"
	}
	return render.StatusMute.Render(fmt.Sprintf(
		"showing %d/%d  ·  filter: %s  ·  %s",
		p.shownCount, p.totalCount, flt, scroll,
	))
}

// ── helpers ────────────────────────────────────────────────────────────────

// parseFilterExpr extracts level:X and service:Y prefixes from the filter
// string. Remaining text becomes the body substring.
func parseFilterExpr(s string) (body, level, service string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", ""
	}
	tokens := strings.Fields(s)
	var rest []string
	for _, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, "level:"):
			level = strings.ToUpper(strings.TrimPrefix(tok, "level:"))
		case strings.HasPrefix(tok, "service:"):
			service = strings.TrimPrefix(tok, "service:")
		default:
			rest = append(rest, tok)
		}
	}
	body = strings.ToLower(strings.Join(rest, " "))
	return
}

func matchesBody(ev model.LogEvent, q string) bool {
	if strings.Contains(strings.ToLower(ev.Body), q) {
		return true
	}
	for k, v := range ev.Attrs {
		if strings.Contains(strings.ToLower(k+"="+v), q) {
			return true
		}
	}
	return false
}

func renderLogLine(ev model.LogEvent) string {
	ts := ev.Timestamp.Format("15:04:05.000")
	levelTag := fmt.Sprintf("[%s]", ev.Level)
	body := ev.Body
	attrs := formatAttrs(ev.Attrs)

	var levelStyle lipgloss.Style
	switch ev.Level {
	case model.LevelDebug:
		levelStyle = lipgloss.NewStyle().Foreground(render.ColorMuted)
	case model.LevelWarn:
		levelStyle = lipgloss.NewStyle().Foreground(render.ColorWarning)
	case model.LevelError:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(render.ColorDanger).Bold(true)
	default:
		levelStyle = lipgloss.NewStyle().Foreground(render.ColorSuccess)
	}

	parts := []string{
		render.StatusMute.Render(ts),
		levelStyle.Render(fmt.Sprintf("%-7s", levelTag)),
		body,
	}
	if attrs != "" {
		parts = append(parts, render.StatusMute.Render(attrs))
	}
	if ev.TraceID != "" {
		parts = append(parts, render.StatusMute.Render("trace="+ev.TraceID))
	}
	return strings.Join(parts, "  ")
}

func formatAttrs(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
	}
	return b.String()
}
