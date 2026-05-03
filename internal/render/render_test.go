package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestHistogramProportionalBars(t *testing.T) {
	got := Histogram([]Bucket{
		{Label: "0-10", Count: 5},
		{Label: "10-20", Count: 10},
		{Label: "20-30", Count: 1},
	}, 10)
	plain := stripANSI(got)
	lines := strings.Split(plain, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	count10 := strings.Count(lines[1], "█")
	count5 := strings.Count(lines[0], "█")
	count1 := strings.Count(lines[2], "█")
	if !(count10 > count5 && count5 > count1) {
		t.Errorf("expected bar widths to scale by count, got %d, %d, %d", count5, count10, count1)
	}
}

func TestHistogramEmpty(t *testing.T) {
	if got := Histogram(nil, 10); got != "" {
		t.Errorf("empty histogram = %q, want empty", got)
	}
}

func TestMakeBucketsEqualWidth(t *testing.T) {
	values := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	buckets := MakeBuckets(values, 5, nil)
	if len(buckets) != 5 {
		t.Fatalf("buckets = %d, want 5", len(buckets))
	}
	total := 0
	for _, b := range buckets {
		total += b.Count
	}
	if total != len(values) {
		t.Errorf("bucket total = %d, want %d", total, len(values))
	}
}

func TestHBarFilledProportion(t *testing.T) {
	got := stripANSI(HBar("p99", 5, 10, 10, lipgloss.Color("1"), "5ms"))
	if strings.Count(got, "█") != 5 {
		t.Errorf("filled cells = %d, want 5: %q", strings.Count(got, "█"), got)
	}
	if strings.Count(got, "░") != 5 {
		t.Errorf("empty cells = %d, want 5: %q", strings.Count(got, "░"), got)
	}
	if !strings.Contains(got, "p99") || !strings.HasSuffix(got, "5ms") {
		t.Errorf("missing label/trailing in %q", got)
	}
}
