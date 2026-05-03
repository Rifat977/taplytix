package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case inEsc:
			// drop
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func TestSparklineWidthExact(t *testing.T) {
	got := stripANSI(Sparkline([]float64{1, 2, 3, 4, 5}, 5, lipgloss.Color("1")))
	if width := []rune(got); len(width) != 5 {
		t.Errorf("rune count = %d, want 5: %q", len(width), got)
	}
}

func TestSparklineEmptyIsSpaces(t *testing.T) {
	got := stripANSI(Sparkline(nil, 4, lipgloss.Color("1")))
	if got != "    " {
		t.Errorf("empty sparkline = %q, want 4 spaces", got)
	}
}

func TestSparklinePadsLeftWhenFewerValues(t *testing.T) {
	got := stripANSI(Sparkline([]float64{1, 5}, 5, lipgloss.Color("1")))
	if !strings.HasPrefix(got, "   ") {
		t.Errorf("expected 3-space left pad in %q", got)
	}
}

func TestSparklineMonotonicAscending(t *testing.T) {
	got := stripANSI(Sparkline([]float64{1, 2, 3, 4, 5, 6, 7, 8}, 8, lipgloss.Color("1")))
	first := []rune(got)[0]
	last := []rune(got)[7]
	if first == last {
		t.Errorf("first and last rune equal in ascending series: %q", got)
	}
}
