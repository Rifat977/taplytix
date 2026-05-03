package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Bucket struct {
	Label string
	Count int
}

// Histogram renders horizontal bars for each bucket. The longest bar
// occupies maxWidth cells; others scale proportionally. Bucket labels are
// left-aligned; counts are appended.
func Histogram(buckets []Bucket, maxWidth int) string {
	if len(buckets) == 0 || maxWidth <= 0 {
		return ""
	}
	maxCount := 0
	maxLabel := 0
	for _, b := range buckets {
		if b.Count > maxCount {
			maxCount = b.Count
		}
		if w := lipgloss.Width(b.Label); w > maxLabel {
			maxLabel = w
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}
	style := lipgloss.NewStyle().Foreground(ColorPrimary)

	var sb strings.Builder
	for i, b := range buckets {
		bw := int(float64(b.Count) / float64(maxCount) * float64(maxWidth))
		if b.Count > 0 && bw < 1 {
			bw = 1
		}
		bar := style.Render(strings.Repeat("█", bw))
		sb.WriteString(fmt.Sprintf("%-*s  %s%s  %d",
			maxLabel, b.Label,
			bar, strings.Repeat(" ", maxWidth-bw),
			b.Count))
		if i < len(buckets)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// MakeBuckets divides [min, max] into n equal-width buckets and counts how
// many values fall into each. The returned slice has length n; labels are
// formatted as the bucket's lower bound.
func MakeBuckets(values []float64, n int, format func(float64) string) []Bucket {
	if len(values) == 0 || n <= 0 {
		return nil
	}
	if format == nil {
		format = func(v float64) string { return fmt.Sprintf("%.2f", v) }
	}
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if max == min {
		return []Bucket{{Label: format(min), Count: len(values)}}
	}
	width := (max - min) / float64(n)
	out := make([]Bucket, n)
	for i := range out {
		out[i].Label = format(min + width*float64(i))
	}
	for _, v := range values {
		idx := int((v - min) / width)
		if idx >= n {
			idx = n - 1
		}
		out[idx].Count++
	}
	return out
}
