package render

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders values as a coloured unicode block string of exactly
// `width` runes. Values are normalised across their own min/max range. When
// fewer values than width are supplied the line is left-padded with spaces;
// when more are supplied the most recent `width` are used.
func Sparkline(values []float64, width int, color lipgloss.Color) string {
	if width <= 0 {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(color)
	if len(values) == 0 {
		return style.Render(strings.Repeat(" ", width))
	}

	if len(values) > width {
		values = values[len(values)-width:]
	}
	pad := width - len(values)

	min, max := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rangeV := max - min
	if rangeV == 0 {
		rangeV = 1 // flat line — render as the lowest block
	}

	var b strings.Builder
	for i := 0; i < pad; i++ {
		b.WriteByte(' ')
	}
	for _, v := range values {
		idx := int(math.Round(((v - min) / rangeV) * float64(len(sparkBlocks)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		b.WriteRune(sparkBlocks[idx])
	}
	return style.Render(b.String())
}
