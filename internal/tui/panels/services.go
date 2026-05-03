package panels

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/render"
	"github.com/rifat977/taplytix/internal/store"
)

// ServicesPanel: left list of services with status dots; right detail
// (P99 latency, heap, error rate, top 3 slowest spans). Press Enter on a
// service to set it as the AppModel's active service.
type ServicesPanel struct {
	store *store.Store

	width, height int

	services []serviceEntry
	active   string

	slist list.Model
}

type serviceEntry struct {
	name   string
	status store.ServiceStatus
}

type serviceItem struct {
	entry  serviceEntry
	active bool
}

func (s serviceItem) Title() string {
	dot := dotForStatus(s.entry.status)
	prefix := "  "
	if s.active {
		prefix = "▸ "
	}
	return fmt.Sprintf("%s%s %s", prefix, dot, s.entry.name)
}

func (s serviceItem) Description() string {
	st := s.entry.status
	if !st.Connected && st.LastSeen.IsZero() {
		return render.StatusMute.Render("(no data yet)")
	}
	return fmt.Sprintf("%.1f ev/s · %.1f%% errors · last %s",
		st.EventsPerSecond,
		st.ErrorRate*100,
		humanAge(st.LastSeen),
	)
}

func (s serviceItem) FilterValue() string { return s.entry.name }

func NewServices(st *store.Store, initialActive string) *ServicesPanel {
	delegate := list.NewDefaultDelegate()
	slist := list.New(nil, delegate, 0, 0)
	slist.Title = "Services"
	slist.SetShowStatusBar(false)
	slist.SetShowHelp(false)
	slist.SetFilteringEnabled(false)
	return &ServicesPanel{store: st, slist: slist, active: initialActive}
}

func (p *ServicesPanel) Title() string { return "Services" }

func (p *ServicesPanel) SetSize(w, h int) {
	p.width, p.height = w, h
	if w <= 0 || h <= 0 {
		return
	}
	leftW := w * 40 / 100
	if leftW < 24 {
		leftW = 24
	}
	p.slist.SetSize(leftW, h-2)
}

func (p *ServicesPanel) Init() tea.Cmd { return nil }

func (p *ServicesPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case RefreshMsg:
		p.refresh()
		return p, nil
	case ServiceChangedMsg:
		p.active = m.Service
		p.refresh()
		return p, nil
	case tea.KeyMsg:
		if m.String() == "enter" {
			if idx := p.slist.Index(); idx >= 0 && idx < len(p.services) {
				next := p.services[idx].name
				if next != p.active {
					p.active = next
					return p, func() tea.Msg { return ServiceChangedMsg{Service: next} }
				}
			}
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.slist, cmd = p.slist.Update(msg)
	return p, cmd
}

func (p *ServicesPanel) refresh() {
	prevActive := p.active
	names := p.store.Services()
	entries := make([]serviceEntry, len(names))
	for i, n := range names {
		entries[i] = serviceEntry{name: n, status: p.store.ServiceStatus(n)}
	}
	p.services = entries

	items := make([]list.Item, len(entries))
	selectIdx := -1
	for i, e := range entries {
		items[i] = serviceItem{entry: e, active: e.name == prevActive}
		if e.name == prevActive {
			selectIdx = i
		}
	}
	p.slist.SetItems(items)
	if selectIdx >= 0 {
		p.slist.Select(selectIdx)
	}
}

func (p *ServicesPanel) View() string {
	if p.width == 0 || p.height == 0 {
		return ""
	}
	leftW := p.width * 40 / 100
	if leftW < 24 {
		leftW = 24
	}
	rightW := p.width - leftW - 1

	left := lipgloss.NewStyle().Width(leftW).Render(p.slist.View())
	right := lipgloss.NewStyle().Width(rightW).Render(p.renderDetail(rightW))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (p *ServicesPanel) renderDetail(width int) string {
	if len(p.services) == 0 {
		return render.StatusMute.Render("(no services — waiting for telemetry…)")
	}
	idx := p.slist.Index()
	if idx < 0 || idx >= len(p.services) {
		idx = 0
	}
	e := p.services[idx]
	st := e.status

	title := lipgloss.NewStyle().Bold(true).Foreground(render.ColorPrimary).
		Render(fmt.Sprintf("%s %s", dotForStatus(st), e.name))
	connection := render.StatusMute.Render(fmt.Sprintf(
		"connected: %v · last seen: %s · %.1f ev/s · err rate %.1f%%",
		st.Connected, humanAge(st.LastSeen), st.EventsPerSecond, st.ErrorRate*100,
	))

	cards := p.renderMiniCards(e.name, width)
	slow := p.renderSlowSpans(e.name)

	return lipgloss.JoinVertical(lipgloss.Left,
		title, connection, "",
		cards, "",
		render.StatusMute.Render("Top slowest spans"), slow,
	)
}

func (p *ServicesPanel) renderMiniCards(service string, width int) string {
	durations := []float64{}
	if tm := p.store.TracesFor(service); tm != nil {
		for _, tr := range tm.Recent(0) {
			collectSpans(tr, func(s *model.SpanEvent) {
				durations = append(durations, float64(s.Duration)/float64(time.Millisecond))
			})
		}
	}
	heap := lookupHeap(p.store.MetricsFor(service))
	st := p.store.ServiceStatus(service)
	cardWidth := (width - 4) / 3
	if cardWidth < 16 {
		cardWidth = 16
	}
	card := func(label, value string) string {
		return render.Card.Width(cardWidth).Render(lipgloss.JoinVertical(
			lipgloss.Left,
			render.StatusMute.Render(label),
			lipgloss.NewStyle().Bold(true).Render(value),
		))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		card("P99 latency", fmtMs(store.Percentile(durations, 99))),
		" ",
		card("Heap", fmtBytes(heap)),
		" ",
		card("Error rate", fmt.Sprintf("%.1f%%", st.ErrorRate*100)),
	)
}

func (p *ServicesPanel) renderSlowSpans(service string) string {
	type entry struct {
		span *model.SpanEvent
	}
	var spans []*model.SpanEvent
	if tm := p.store.TracesFor(service); tm != nil {
		for _, tr := range tm.Recent(20) {
			collectSpans(tr, func(s *model.SpanEvent) { spans = append(spans, s) })
		}
	}
	if len(spans) == 0 {
		return render.StatusMute.Render("(no spans yet)")
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Duration > spans[j].Duration })
	if len(spans) > 3 {
		spans = spans[:3]
	}
	lines := make([]string, len(spans))
	for i, s := range spans {
		dur := time.Duration(s.Duration).String()
		lines[i] = fmt.Sprintf("  %-30s  %s", truncate(s.Name, 30), dur)
	}
	return strings.Join(lines, "\n")
}

// ── helpers ────────────────────────────────────────────────────────────────

func dotForStatus(st store.ServiceStatus) string {
	switch {
	case st.LastSeen.IsZero():
		return render.StatusMute.Render("●")
	case time.Since(st.LastSeen) <= 5*time.Second:
		return render.StatusOK.Render("●")
	case time.Since(st.LastSeen) <= 10*time.Second:
		return render.StatusWarn.Render("●")
	default:
		return render.StatusError.Render("●")
	}
}

func humanAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t).Round(time.Second)
	if d < time.Second {
		return "just now"
	}
	return d.String() + " ago"
}
