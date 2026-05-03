package store

import (
	"math"
	"sort"
)

// Percentile returns the p-th percentile of values (0 ≤ p ≤ 100) using the
// nearest-rank method: index = ceil(p/100 * n) - 1, clamped. The input
// slice is never mutated.
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := make([]float64, len(values))
	copy(cp, values)
	sort.Float64s(cp)
	if p <= 0 {
		return cp[0]
	}
	if p >= 100 {
		return cp[len(cp)-1]
	}
	idx := int(math.Ceil(p/100*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}
