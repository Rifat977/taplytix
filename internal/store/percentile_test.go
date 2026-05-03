package store

import (
	"reflect"
	"testing"
)

func TestPercentileBasic(t *testing.T) {
	if got := Percentile([]float64{1, 2, 3, 4, 5}, 99); got != 5 {
		t.Errorf("P99 = %v, want 5", got)
	}
	if got := Percentile([]float64{1, 2, 3, 4, 5}, 50); got != 3 {
		t.Errorf("P50 = %v, want 3", got)
	}
	if got := Percentile([]float64{1, 2, 3, 4, 5}, 0); got != 1 {
		t.Errorf("P0 = %v, want 1", got)
	}
}

func TestPercentileEmpty(t *testing.T) {
	if got := Percentile(nil, 95); got != 0 {
		t.Errorf("empty P95 = %v, want 0", got)
	}
}

func TestPercentileDoesNotMutateInput(t *testing.T) {
	in := []float64{5, 3, 1, 4, 2}
	orig := make([]float64, len(in))
	copy(orig, in)
	_ = Percentile(in, 95)
	if !reflect.DeepEqual(in, orig) {
		t.Errorf("input mutated: got %v, want %v", in, orig)
	}
}
