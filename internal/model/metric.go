package model

import "time"

type MetricKind int

const (
	Counter MetricKind = iota
	Gauge
	Histogram
)

func (k MetricKind) String() string {
	switch k {
	case Counter:
		return "counter"
	case Gauge:
		return "gauge"
	case Histogram:
		return "histogram"
	default:
		return "unknown"
	}
}

type MetricEvent struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Kind      MetricKind
	Timestamp time.Time
	Source    string
}
