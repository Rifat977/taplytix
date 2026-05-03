package store

import (
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

func TestSeriesPushAndPercentiles(t *testing.T) {
	s := NewSeries("latency", model.Histogram, nil, 0)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		s.Push(model.MetricEvent{Name: "latency", Value: v, Timestamp: time.Now()})
	}
	if got := s.P50(); got != 3 {
		t.Errorf("P50 = %v, want 3", got)
	}
	if got := s.P99(); got != 5 {
		t.Errorf("P99 = %v, want 5", got)
	}
	if got := s.Last(); got != 5 {
		t.Errorf("Last = %v, want 5", got)
	}
}

func TestSeriesSparkline(t *testing.T) {
	s := NewSeries("x", model.Gauge, nil, 0)
	for i := 0; i < 10; i++ {
		s.Push(model.MetricEvent{Value: float64(i)})
	}
	got := s.Sparkline(3)
	want := []float64{7, 8, 9}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("sparkline(3) = %v, want %v", got, want)
	}
}
