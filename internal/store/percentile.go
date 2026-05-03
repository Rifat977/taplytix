package store

import (
	"math"
	"sort"
)

// Percentile returns the p-th percentile of values (0 ≤ p ≤ 100) using linear
// interpolation between ranks. The input slice is never mutated.
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		p = 0
	}
	if p >= 100 {
		p = 100
	}
	cp := make([]float64, len(values))
	copy(cp, values)
	sort.Float64s(cp)
	if len(cp) == 1 {
		return cp[0]
	}
	rank := (p / 100) * float64(len(cp)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return cp[lo]
	}
	frac := rank - float64(lo)
	return cp[lo] + frac*(cp[hi]-cp[lo])
}
