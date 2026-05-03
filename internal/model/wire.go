package model

import (
	"encoding/json"
	"fmt"
)

// WireKind tags a serialised event so the decoder knows which struct to
// unmarshal into. Strings (rather than ints) so the format is human-readable.
type WireKind string

const (
	WireMetric WireKind = "metric"
	WireSpan   WireKind = "span"
	WireLog    WireKind = "log"
)

// WireEvent is the JSON envelope used for forwarding events between an
// agent and a dashboard. Exactly one of Metric/Span/Log is populated.
type WireEvent struct {
	Kind   WireKind     `json:"kind"`
	Metric *MetricEvent `json:"metric,omitempty"`
	Span   *SpanEvent   `json:"span,omitempty"`
	Log    *LogEvent    `json:"log,omitempty"`
}

// EncodeEvent wraps a typed event into a WireEvent and marshals it to JSON.
// Events of unknown type return an error.
func EncodeEvent(ev any) ([]byte, error) {
	we, err := WrapEvent(ev)
	if err != nil {
		return nil, err
	}
	return json.Marshal(we)
}

// WrapEvent boxes a typed event into a WireEvent. Useful when callers want
// to batch several events into one JSON array.
func WrapEvent(ev any) (WireEvent, error) {
	switch e := ev.(type) {
	case MetricEvent:
		return WireEvent{Kind: WireMetric, Metric: &e}, nil
	case SpanEvent:
		return WireEvent{Kind: WireSpan, Span: &e}, nil
	case LogEvent:
		return WireEvent{Kind: WireLog, Log: &e}, nil
	default:
		return WireEvent{}, fmt.Errorf("wire: unsupported event type %T", ev)
	}
}

// DecodeEvent parses a JSON-encoded WireEvent and returns the inner typed
// event (MetricEvent / SpanEvent / LogEvent).
func DecodeEvent(data []byte) (any, error) {
	var we WireEvent
	if err := json.Unmarshal(data, &we); err != nil {
		return nil, err
	}
	return we.Unwrap()
}

// Unwrap returns the inner typed event from a WireEvent.
func (w WireEvent) Unwrap() (any, error) {
	switch w.Kind {
	case WireMetric:
		if w.Metric == nil {
			return nil, fmt.Errorf("wire: metric kind with nil payload")
		}
		return *w.Metric, nil
	case WireSpan:
		if w.Span == nil {
			return nil, fmt.Errorf("wire: span kind with nil payload")
		}
		return *w.Span, nil
	case WireLog:
		if w.Log == nil {
			return nil, fmt.Errorf("wire: log kind with nil payload")
		}
		return *w.Log, nil
	default:
		return nil, fmt.Errorf("wire: unknown kind %q", w.Kind)
	}
}
