package panels

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/config"
	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/render"
	"github.com/rifat977/taplytix/internal/store"
)

// RefreshMsg tells panels to re-read the store. The TUI app forwards its
// internal tick under this name so the message type lives in the panels
// package and stays import-friendly.
type RefreshMsg struct{ Time time.Time }

const (
	sparkSamples   = 40
	tableTopN      = 10
	heapAlertBytes = 500_000_000
	p99AlertMs     = 500
)

// OverviewPanel is the default landing tab: four vital-sign cards plus a
// table of the slowest operations by P95.
type OverviewPanel struct {
	store   *store.Store
	cfg     *config.Config
	service string

	width, height int

	p99Hist   *store.Ring[float64]
	p50Hist   *store.Ring[float64]
	heapHist  *store.Ring[float64]
	spansHist *store.Ring[float64]

	p99      float64
	p50      float64
	heap     float64
	spans    int
	hasData  bool

	tbl table.Model
}

func NewOverview(cfg *config.Config, st *store.Store) *OverviewPanel {
	service := cfg.Server.DefaultService
	if service == "" && len(cfg.Sources) > 0 {
		service = cfg.Sources[0].Name
	}
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "Operation", Width: 28},
			{Title: "P50", Width: 8},
			{Title: "P95", Width: 8},
			{Title: "P99", Width: 8},
			{Title: "Req/s", Width: 8},
			{Title: "Errors", Width: 8},
		}),
		table.WithHeight(tableTopN+1),
	)
	return &OverviewPanel{
		store:     st,
		cfg:       cfg,
		service:   service,
		p99Hist:   store.NewRing[float64](sparkSamples),
		p50Hist:   store.NewRing[float64](sparkSamples),
		heapHist:  store.NewRing[float64](sparkSamples),
		spansHist: store.NewRing[float64](sparkSamples),
		tbl:       tbl,
	}
}

func (p *OverviewPanel) Title() string { return "Overview" }

func (p *OverviewPanel) SetSize(w, h int) {
	p.width, p.height = w, h
	if h > 12 {
		p.tbl.SetHeight(h - 10)
	}
	if w > 0 {
		p.tbl.SetWidth(w)
	}
}

func (p *OverviewPanel) Init() tea.Cmd { return nil }

func (p *OverviewPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case RefreshMsg:
		p.refresh()
	case ServiceChangedMsg:
		if m.Service != "" {
			p.service = m.Service
		}
		p.refresh()
	}
	return p, nil
}

func (p *OverviewPanel) refresh() {
	traces := p.store.TracesFor(p.service)
	durations := []float64{}
	type bucket struct {
		durs   []float64
		errors int
	}
	byOp := map[string]*bucket{}
	var totalSpans int
	var oldest time.Time

	if traces != nil {
		for _, tr := range traces.Recent(0) {
			collectSpans(tr, func(s *model.SpanEvent) {
				ms := float64(s.Duration) / float64(time.Millisecond)
				durations = append(durations, ms)
				b, ok := byOp[s.Name]
				if !ok {
					b = &bucket{}
					byOp[s.Name] = b
				}
				b.durs = append(b.durs, ms)
				if s.Status == model.StatusError {
					b.errors++
				}
				totalSpans++
				if oldest.IsZero() || s.StartTime.Before(oldest) {
					oldest = s.StartTime
				}
			})
		}
	}

	p.p99 = store.Percentile(durations, 99)
	p.p50 = store.Percentile(durations, 50)
	p.spans = totalSpans
	p.heap = lookupHeap(p.store.MetricsFor(p.service))
	p.hasData = len(durations) > 0 || p.heap > 0

	p.p99Hist.Push(p.p99)
	p.p50Hist.Push(p.p50)
	p.heapHist.Push(p.heap)
	p.spansHist.Push(float64(p.spans))

	windowSec := time.Since(oldest).Seconds()
	if windowSec < 1 {
		windowSec = 1
	}

	type opRow struct {
		name           string
		p50, p95, p99  float64
		count, errors int
	}
	rows := make([]opRow, 0, len(byOp))
	for name, b := range byOp {
		rows = append(rows, opRow{
			name:   name,
			p50:    store.Percentile(b.durs, 50),
			p95:    store.Percentile(b.durs, 95),
			p99:    store.Percentile(b.durs, 99),
			count:  len(b.durs),
			errors: b.errors,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].p95 > rows[j].p95 })
	if len(rows) > tableTopN {
		rows = rows[:tableTopN]
	}
	tableRows := make([]table.Row, len(rows))
	for i, r := range rows {
		errPct := 0.0
		if r.count > 0 {
			errPct = float64(r.errors) / float64(r.count) * 100
		}
		tableRows[i] = table.Row{
			truncate(r.name, 28),
			fmtMs(r.p50),
			fmtMs(r.p95),
			fmtMs(r.p99),
			fmt.Sprintf("%.1f", float64(r.count)/windowSec),
			fmt.Sprintf("%.1f%%", errPct),
		}
	}
	p.tbl.SetRows(tableRows)
}

func (p *OverviewPanel) View() string {
	if p.width == 0 {
		return ""
	}
	cards := lipgloss.JoinHorizontal(lipgloss.Top,
		p.renderCard("P99 latency", fmtMs(p.p99), p.p99Hist.Slice(), p.p99 > p99AlertMs),
		" ",
		p.renderCard("P50 latency", fmtMs(p.p50), p.p50Hist.Slice(), false),
		" ",
		p.renderCard("Heap in use", fmtBytes(p.heap), p.heapHist.Slice(), p.heap > heapAlertBytes),
		" ",
		p.renderCard("Active spans", fmt.Sprintf("%d", p.spans), p.spansHist.Slice(), false),
	)
	heading := render.StatusMute.Render("Slowest operations (P95)")
	tableView := p.tbl.View()
	if len(p.tbl.Rows()) == 0 {
		tableView = render.StatusMute.Render("(waiting for telemetry…)")
	}
	return lipgloss.JoinVertical(lipgloss.Left, "", cards, "", heading, tableView)
}

func (p *OverviewPanel) renderCard(label, value string, history []float64, alert bool) string {
	border := render.ColorMuted
	valueStyle := lipgloss.NewStyle().Foreground(render.ColorText).Bold(true)
	sparkColor := render.ColorPrimary
	if alert {
		border = render.ColorDanger
		valueStyle = valueStyle.Foreground(render.ColorDanger)
		sparkColor = render.ColorDanger
	}
	style := render.Card.BorderForeground(border).Width(20)
	body := lipgloss.JoinVertical(lipgloss.Left,
		render.StatusMute.Render(label),
		valueStyle.Render(value),
		render.Sparkline(history, 18, sparkColor),
	)
	return style.Render(body)
}

func collectSpans(t *model.Trace, visit func(*model.SpanEvent)) {
	if t == nil {
		return
	}
	if t.Root != nil {
		visit(t.Root)
	}
	for _, kids := range t.Children {
		for _, c := range kids {
			visit(c)
		}
	}
}

func lookupHeap(metrics map[string]*store.Series) float64 {
	for name, s := range metrics {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "heap") && strings.Contains(lower, "use") {
			return s.Last()
		}
	}
	return 0
}

func fmtMs(v float64) string {
	if v == 0 {
		return "—"
	}
	if v >= 1000 {
		return fmt.Sprintf("%.2fs", v/1000)
	}
	return fmt.Sprintf("%.0fms", v)
}

func fmtBytes(v float64) string {
	if v == 0 {
		return "—"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case v >= gb:
		return fmt.Sprintf("%.1f GB", v/gb)
	case v >= mb:
		return fmt.Sprintf("%.0f MB", v/mb)
	case v >= kb:
		return fmt.Sprintf("%.0f KB", v/kb)
	default:
		return fmt.Sprintf("%.0f B", v)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
