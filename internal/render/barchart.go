package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HBar renders a horizontal bar of the form "label  ████░░░  trailing".
// `value/maxValue` controls the filled portion; barWidth is the cell budget
// for the bar itself. The trailing string is rendered verbatim after the
// bar (e.g. "42.0ms" or formatted unit).
func HBar(label string, value, maxValue float64, barWidth int, color lipgloss.Color, trailing string) string {
	if barWidth < 1 {
		barWidth = 1
	}
	if maxValue <= 0 {
		maxValue = 1
	}
	filled := int(value / maxValue * float64(barWidth))
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("░", barWidth-filled))
	if label == "" {
		return bar + "  " + trailing
	}
	return label + "  " + bar + "  " + trailing
}
