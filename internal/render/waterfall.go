package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/model"
)

// Sub-cell block characters in 1/8th increments.
var partialBlocks = []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

// WaterfallRow renders a single span as one row of the trace waterfall.
// `prefix` is the already-rendered indent (spaces + tree chars like ├ └ │)
// supplied by the caller so that sibling/ancestor context can be preserved.
// `availWidth` is the total width budget for the row.
func WaterfallRow(span *model.SpanEvent, traceStart time.Time, total time.Duration, prefix string, availWidth int) string {
	if span == nil || availWidth <= 0 {
		return ""
	}

	name := truncateName(span.Name, 24)
	durLabel := fmt.Sprintf("%6s", formatDuration(span.Duration))

	// Reserve room for prefix, name (with one trailing space) and duration label.
	nameCell := prefix + name + " "
	consumed := lipgloss.Width(nameCell) + lipgloss.Width(durLabel) + 1
	barBudget := availWidth - consumed
	if barBudget < 4 {
		barBudget = 4
	}

	bar := buildBar(span, traceStart, total, barBudget)
	return nameCell + bar + " " + StatusMute.Render(durLabel)
}

func buildBar(span *model.SpanEvent, traceStart time.Time, total time.Duration, width int) string {
	if total <= 0 || width <= 0 {
		return strings.Repeat(" ", maxInt(width, 0))
	}

	totalUnits := width * 8 // sub-cell units (1/8 each)
	offsetUnits := int(float64(span.StartTime.Sub(traceStart)) / float64(total) * float64(totalUnits))
	durUnits := int(float64(span.Duration) / float64(total) * float64(totalUnits))
	if offsetUnits < 0 {
		offsetUnits = 0
	}
	if offsetUnits > totalUnits {
		offsetUnits = totalUnits
	}
	if durUnits < 1 {
		durUnits = 1
	}
	if offsetUnits+durUnits > totalUnits {
		durUnits = totalUnits - offsetUnits
	}

	leadCells := offsetUnits / 8
	leadFrac := offsetUnits % 8

	// Whole cells of bar; the leading partial-cell offset is rendered as
	// spaces, and any trailing partial-cell tail uses a partial block.
	fullCells := durUnits / 8
	tailFrac := durUnits % 8

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", leadCells))
	if leadFrac > 0 {
		// nudge the bar by drawing the partial char at the start
		b.WriteRune(partialBlocks[leadFrac])
		fullCells--
		if fullCells < 0 {
			fullCells = 0
		}
	}
	if fullCells > 0 {
		b.WriteString(strings.Repeat("█", fullCells))
	}
	if tailFrac > 0 {
		b.WriteRune(partialBlocks[tailFrac])
	}

	rendered := b.String()
	// Pad to width
	pad := width - lipgloss.Width(rendered)
	if pad > 0 {
		rendered += strings.Repeat(" ", pad)
	} else if pad < 0 {
		// Should not happen, but truncate defensively.
		rendered = string([]rune(rendered)[:width])
	}

	color := spanColor(span)
	return lipgloss.NewStyle().Foreground(color).Render(rendered)
}

func spanColor(s *model.SpanEvent) lipgloss.Color {
	switch {
	case s.Status == model.StatusError:
		return ColorDanger
	case s.Duration > 200*time.Millisecond:
		return ColorWarning
	default:
		return ColorSuccess
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d >= time.Microsecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	default:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
}

func truncateName(s string, n int) string {
	if len([]rune(s)) <= n {
		return fmt.Sprintf("%-*s", n, s)
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
