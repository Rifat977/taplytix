package panels

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/render"
	"github.com/rifat977/taplytix/internal/store"
)

const recentTraceLimit = 50

// TracesPanel shows recent traces (top section) and a span waterfall for the
// currently selected trace (bottom section). Enter expands the waterfall to
// full panel height; Esc returns to the split view.
type TracesPanel struct {
	store *store.Store

	width, height int

	traces   []traceRef
	selected int
	expanded bool

	tlist list.Model
	vp    viewport.Model
}

type traceRef struct {
	service string
	trace   *model.Trace
}

type traceItem struct {
	ref traceRef
}

func (t traceItem) Title() string {
	rootName := "(no root)"
	if t.ref.trace.Root != nil {
		rootName = t.ref.trace.Root.Name
	}
	return fmt.Sprintf("%s · %s", t.ref.service, rootName)
}

func (t traceItem) Description() string {
	count := traceSpanCount(t.ref.trace)
	errs := traceErrorCount(t.ref.trace)
	icon := "●"
	if errs > 0 {
		icon = "✗"
	}
	return fmt.Sprintf("%s %s · %d span(s) · %d err",
		icon,
		render.StatusMute.Render(durationLabel(t.ref.trace.Duration)),
		count, errs,
	)
}

func (t traceItem) FilterValue() string { return t.Title() }

func NewTraces(st *store.Store) *TracesPanel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	tlist := list.New(nil, delegate, 0, 0)
	tlist.Title = "Recent traces"
	tlist.SetShowStatusBar(false)
	tlist.SetShowHelp(false)
	tlist.SetFilteringEnabled(false)

	return &TracesPanel{
		store: st,
		tlist: tlist,
		vp:    viewport.New(0, 0),
	}
}

func (p *TracesPanel) Title() string { return "Traces" }

func (p *TracesPanel) SetSize(w, h int) {
	p.width, p.height = w, h
	p.relayout()
}

func (p *TracesPanel) relayout() {
	if p.width <= 0 || p.height <= 0 {
		return
	}
	if p.expanded {
		p.vp.Width = p.width
		p.vp.Height = p.height
		p.tlist.SetSize(p.width, 0)
		return
	}
	listHeight := p.height * 30 / 100
	if listHeight < 5 {
		listHeight = 5
	}
	if listHeight > p.height-5 {
		listHeight = p.height - 5
	}
	p.tlist.SetSize(p.width, listHeight)
	p.vp.Width = p.width
	p.vp.Height = p.height - listHeight - 2
	if p.vp.Height < 1 {
		p.vp.Height = 1
	}
}

func (p *TracesPanel) Init() tea.Cmd { return nil }

func (p *TracesPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case RefreshMsg:
		p.refresh()
		return p, nil
	case tea.KeyMsg:
		switch m.String() {
		case "enter":
			if len(p.traces) > 0 {
				p.expanded = !p.expanded
				p.relayout()
				p.renderWaterfall()
			}
			return p, nil
		case "esc":
			if p.expanded {
				p.expanded = false
				p.relayout()
				p.renderWaterfall()
			}
			return p, nil
		}
	}
	var cmd tea.Cmd
	if p.expanded {
		p.vp, cmd = p.vp.Update(msg)
	} else {
		var listCmd tea.Cmd
		p.tlist, listCmd = p.tlist.Update(msg)
		if idx := p.tlist.Index(); idx != p.selected {
			p.selected = idx
			p.renderWaterfall()
		}
		cmd = listCmd
	}
	return p, cmd
}

func (p *TracesPanel) refresh() {
	prevID := ""
	if p.selected >= 0 && p.selected < len(p.traces) {
		prevID = p.traces[p.selected].trace.TraceID
	}

	var refs []traceRef
	for _, svc := range p.store.Services() {
		tm := p.store.TracesFor(svc)
		if tm == nil {
			continue
		}
		for _, tr := range tm.Recent(0) {
			refs = append(refs, traceRef{service: svc, trace: tr})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		return traceStartTime(refs[i].trace).After(traceStartTime(refs[j].trace))
	})
	if len(refs) > recentTraceLimit {
		refs = refs[:recentTraceLimit]
	}
	p.traces = refs

	items := make([]list.Item, len(refs))
	for i, r := range refs {
		items[i] = traceItem{ref: r}
	}
	p.tlist.SetItems(items)

	// Restore selection by traceID if possible.
	p.selected = 0
	if prevID != "" {
		for i, r := range refs {
			if r.trace.TraceID == prevID {
				p.selected = i
				p.tlist.Select(i)
				break
			}
		}
	}
	p.renderWaterfall()
}

func (p *TracesPanel) renderWaterfall() {
	if len(p.traces) == 0 || p.selected < 0 || p.selected >= len(p.traces) {
		p.vp.SetContent(render.StatusMute.Render("(no trace selected — waiting for telemetry…)"))
		return
	}
	tr := p.traces[p.selected].trace
	if tr.Root == nil {
		p.vp.SetContent(render.StatusMute.Render("(trace has no root span yet)"))
		return
	}

	width := p.vp.Width
	if width <= 0 {
		width = 80
	}
	traceStart := tr.Root.StartTime
	total := tr.Duration
	if total <= 0 {
		total = time.Millisecond
	}

	slowest := slowestSpan(tr)
	rows := []row{}
	walkTraceDFS(tr, tr.Root, []bool{}, &rows)

	header := traceSummary(p.traces[p.selected].service, tr)
	body := make([]string, 0, len(rows)+2)
	body = append(body, header, "")
	for _, r := range rows {
		line := render.WaterfallRow(r.span, traceStart, total, r.prefix, width)
		if r.span == slowest && slowest != nil {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		body = append(body, line)
	}
	p.vp.SetContent(strings.Join(body, "\n"))
}

type row struct {
	span   *model.SpanEvent
	prefix string
}

func walkTraceDFS(tr *model.Trace, span *model.SpanEvent, ancestorsHasNext []bool, out *[]row) {
	if span == nil {
		return
	}
	prefix := buildTreePrefix(ancestorsHasNext)
	*out = append(*out, row{span: span, prefix: prefix})
	kids := tr.Children[span.SpanID]
	sort.Slice(kids, func(i, j int) bool { return kids[i].StartTime.Before(kids[j].StartTime) })
	for i, c := range kids {
		isLast := i == len(kids)-1
		nextAncestors := append(append([]bool{}, ancestorsHasNext...), !isLast)
		walkTraceDFS(tr, c, nextAncestors, out)
	}
}

func buildTreePrefix(ancestorsHasNext []bool) string {
	if len(ancestorsHasNext) == 0 {
		return ""
	}
	var b strings.Builder
	for i, hasNext := range ancestorsHasNext {
		last := i == len(ancestorsHasNext)-1
		switch {
		case last && hasNext:
			b.WriteString("├─ ")
		case last && !hasNext:
			b.WriteString("└─ ")
		case hasNext:
			b.WriteString("│  ")
		default:
			b.WriteString("   ")
		}
	}
	return b.String()
}

func (p *TracesPanel) View() string {
	if p.width == 0 || p.height == 0 {
		return ""
	}
	if p.expanded {
		return p.vp.View()
	}
	listView := p.tlist.View()
	wfHeader := render.StatusMute.Render("Waterfall (Enter to expand · Esc to collapse · ↑↓ to navigate)")
	return lipgloss.JoinVertical(lipgloss.Left, listView, wfHeader, p.vp.View())
}

// ── helpers ────────────────────────────────────────────────────────────────

func traceSpanCount(t *model.Trace) int {
	if t == nil {
		return 0
	}
	n := 0
	if t.Root != nil {
		n++
	}
	for _, kids := range t.Children {
		n += len(kids)
	}
	return n
}

func traceErrorCount(t *model.Trace) int {
	if t == nil {
		return 0
	}
	n := 0
	if t.Root != nil && t.Root.Status == model.StatusError {
		n++
	}
	for _, kids := range t.Children {
		for _, c := range kids {
			if c.Status == model.StatusError {
				n++
			}
		}
	}
	return n
}

func traceStartTime(t *model.Trace) time.Time {
	if t == nil || t.Root == nil {
		if t != nil {
			for _, kids := range t.Children {
				for _, c := range kids {
					return c.StartTime
				}
			}
		}
		return time.Time{}
	}
	return t.Root.StartTime
}

func slowestSpan(t *model.Trace) *model.SpanEvent {
	var slow *model.SpanEvent
	visit := func(s *model.SpanEvent) {
		if slow == nil || s.Duration > slow.Duration {
			slow = s
		}
	}
	if t.Root != nil {
		visit(t.Root)
	}
	for _, kids := range t.Children {
		for _, c := range kids {
			visit(c)
		}
	}
	return slow
}

func traceSummary(service string, t *model.Trace) string {
	count := traceSpanCount(t)
	errs := traceErrorCount(t)
	rootName := ""
	if t.Root != nil {
		rootName = t.Root.Name
	}
	return fmt.Sprintf("%s · %s · %s · %d span(s) · %d error(s)",
		service,
		rootName,
		durationLabel(t.Duration),
		count,
		errs,
	)
}

func durationLabel(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return d.String()
	}
}
