package panels

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/render"
	"github.com/rifat977/taplytix/internal/store"
)

// MetricsPanel: searchable list of metrics on the left, selected metric's
// percentile windows + histogram + sparkline on the right.
type MetricsPanel struct {
	store *store.Store

	width, height int

	allKeys  []metricRef // all known (service, key) — full set, used to filter
	visKeys  []metricRef // currently visible after filter
	selected int

	mlist   list.Model
	filter  textinput.Model
	filtering bool
}

type metricRef struct {
	service string
	key     string
	series  *store.Series
}

type metricItem struct {
	ref metricRef
}

func (m metricItem) Title() string {
	return fmt.Sprintf("%s · %s", m.ref.service, m.ref.key)
}
func (m metricItem) Description() string {
	return fmt.Sprintf("%s · %s",
		m.ref.series.Kind.String(),
		formatMetricValue(m.ref.series.Last()),
	)
}
func (m metricItem) FilterValue() string { return m.Title() }

func NewMetrics(st *store.Store) *MetricsPanel {
	delegate := list.NewDefaultDelegate()
	mlist := list.New(nil, delegate, 0, 0)
	mlist.Title = "Metrics"
	mlist.SetShowStatusBar(false)
	mlist.SetShowHelp(false)
	mlist.SetFilteringEnabled(false)

	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.Prompt = "/"
	ti.CharLimit = 64

	return &MetricsPanel{store: st, mlist: mlist, filter: ti}
}

func (p *MetricsPanel) Title() string { return "Metrics" }

func (p *MetricsPanel) SetSize(w, h int) {
	p.width, p.height = w, h
	if w <= 0 || h <= 0 {
		return
	}
	leftW := w * 35 / 100
	if leftW < 24 {
		leftW = 24
	}
	listH := h - 3 // reserve filter input + heading
	if listH < 3 {
		listH = 3
	}
	p.mlist.SetSize(leftW, listH)
}

func (p *MetricsPanel) Init() tea.Cmd { return nil }

func (p *MetricsPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case RefreshMsg:
		p.refresh()
		return p, nil
	case tea.KeyMsg:
		if p.filtering {
			switch m.String() {
			case "esc":
				p.filtering = false
				p.filter.SetValue("")
				p.filter.Blur()
				p.applyFilter()
				return p, nil
			case "enter":
				p.filtering = false
				p.filter.Blur()
				return p, nil
			}
			var cmd tea.Cmd
			p.filter, cmd = p.filter.Update(msg)
			p.applyFilter()
			return p, cmd
		}
		if m.String() == "/" {
			p.filtering = true
			p.filter.Focus()
			return p, textinput.Blink
		}
	}
	var cmd tea.Cmd
	p.mlist, cmd = p.mlist.Update(msg)
	if idx := p.mlist.Index(); idx != p.selected {
		p.selected = idx
	}
	return p, cmd
}

func (p *MetricsPanel) refresh() {
	prevKey := ""
	if p.selected >= 0 && p.selected < len(p.visKeys) {
		prevKey = p.visKeys[p.selected].service + "::" + p.visKeys[p.selected].key
	}

	var refs []metricRef
	for _, svc := range p.store.Services() {
		ms := p.store.MetricsFor(svc)
		for k, s := range ms {
			refs = append(refs, metricRef{service: svc, key: k, series: s})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].service != refs[j].service {
			return refs[i].service < refs[j].service
		}
		return refs[i].key < refs[j].key
	})
	p.allKeys = refs
	p.applyFilter()

	if prevKey != "" {
		for i, r := range p.visKeys {
			if r.service+"::"+r.key == prevKey {
				p.selected = i
				p.mlist.Select(i)
				break
			}
		}
	}
}

func (p *MetricsPanel) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	p.visKeys = make([]metricRef, 0, len(p.allKeys))
	for _, r := range p.allKeys {
		if q == "" || strings.Contains(strings.ToLower(r.service+" "+r.key), q) {
			p.visKeys = append(p.visKeys, r)
		}
	}
	items := make([]list.Item, len(p.visKeys))
	for i, r := range p.visKeys {
		items[i] = metricItem{ref: r}
	}
	p.mlist.SetItems(items)
}

func (p *MetricsPanel) View() string {
	if p.width == 0 || p.height == 0 {
		return ""
	}
	header := render.StatusMute.Render("/ to filter · ↑↓ to select")
	if p.filtering {
		header = p.filter.View()
	}
	leftW := p.width * 35 / 100
	if leftW < 24 {
		leftW = 24
	}
	rightW := p.width - leftW - 1

	left := lipgloss.JoinVertical(lipgloss.Left, header, p.mlist.View())
	right := p.renderDetail(rightW)
	leftCol := lipgloss.NewStyle().Width(leftW).Render(left)
	rightCol := lipgloss.NewStyle().Width(rightW).Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, " ", rightCol)
}

func (p *MetricsPanel) renderDetail(width int) string {
	if len(p.visKeys) == 0 {
		return render.StatusMute.Render("(no metrics — waiting for telemetry…)")
	}
	if p.selected < 0 || p.selected >= len(p.visKeys) {
		return ""
	}
	ref := p.visKeys[p.selected]
	s := ref.series
	values := s.Values()

	title := lipgloss.NewStyle().Bold(true).Foreground(render.ColorPrimary).Render(ref.key)
	subtitle := render.StatusMute.Render(fmt.Sprintf("%s · %s · %d sample(s)", ref.service, s.Kind.String(), s.Len()))
	current := lipgloss.NewStyle().Bold(true).Foreground(render.ColorText).Render(formatMetricValue(s.Last()))

	windows := renderWindowTable(values)
	rate := renderRate(values)
	hist := renderHistogramSection(values, width)
	spark := render.Sparkline(s.Sparkline(width-2), width-2, render.ColorPrimary)

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		current,
		"",
		windows,
		"",
		rate,
		"",
		render.StatusMute.Render("Distribution"),
		hist,
		"",
		render.StatusMute.Render("Last "+fmt.Sprint(s.Len())+" sample(s)"),
		spark,
	)
}

func renderWindowTable(values []float64) string {
	rows := []struct {
		label string
		n     int
	}{
		{"1 min", 60},
		{"5 min", 300},
		{"15 min", 900},
	}
	lines := []string{
		fmt.Sprintf("%-7s  %-10s %-10s %-10s", "window", "P50", "P95", "P99"),
	}
	for _, r := range rows {
		win := tail(values, r.n)
		lines = append(lines, fmt.Sprintf("%-7s  %-10s %-10s %-10s",
			r.label,
			formatMetricValue(store.Percentile(win, 50)),
			formatMetricValue(store.Percentile(win, 95)),
			formatMetricValue(store.Percentile(win, 99)),
		))
	}
	return strings.Join(lines, "\n")
}

func renderRate(values []float64) string {
	if len(values) < 2 {
		return render.StatusMute.Render("rate: —")
	}
	delta := values[len(values)-1] - values[0]
	rate := delta / float64(len(values))
	return fmt.Sprintf("rate of change: %+.3f / sample", rate)
}

func renderHistogramSection(values []float64, width int) string {
	if len(values) == 0 {
		return render.StatusMute.Render("(no samples)")
	}
	buckets := render.MakeBuckets(values, 10, func(v float64) string {
		return fmt.Sprintf("%g", v)
	})
	barWidth := width - 24
	if barWidth < 8 {
		barWidth = 8
	}
	return render.Histogram(buckets, barWidth)
}

func tail(values []float64, n int) []float64 {
	if n <= 0 || n >= len(values) {
		return values
	}
	return values[len(values)-n:]
}

func formatMetricValue(v float64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.2fG", v/1_000_000_000)
	case v >= 1_000_000:
		return fmt.Sprintf("%.2fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.2fk", v/1_000)
	case v >= 1:
		return fmt.Sprintf("%.2f", v)
	case v == 0:
		return "0"
	default:
		return fmt.Sprintf("%.4f", v)
	}
}

// _ unused-tear: prevents the import-cycle linter from complaining when
// model is only referenced via store.Series.Kind during refactors.
var _ = model.Counter
