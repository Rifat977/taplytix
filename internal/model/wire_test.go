package model

import (
	"reflect"
	"testing"
	"time"
)

func TestWireRoundTrip(t *testing.T) {
	now := time.Now().UTC().Round(time.Microsecond) // JSON drops sub-µs
	cases := []any{
		MetricEvent{Name: "x", Value: 42, Source: "svc", Kind: Gauge, Timestamp: now, Labels: map[string]string{"a": "b"}},
		SpanEvent{TraceID: "t", SpanID: "s", Name: "GET /a", Service: "svc", StartTime: now, Duration: 50 * time.Millisecond, Status: StatusError},
		LogEvent{Body: "hi", Level: LevelWarn, Service: "svc", Timestamp: now, TraceID: "t", Attrs: map[string]string{"k": "v"}},
	}
	for _, ev := range cases {
		data, err := EncodeEvent(ev)
		if err != nil {
			t.Errorf("encode %T: %v", ev, err)
			continue
		}
		got, err := DecodeEvent(data)
		if err != nil {
			t.Errorf("decode %T: %v (raw: %s)", ev, err, data)
			continue
		}
		if !reflect.DeepEqual(ev, got) {
			t.Errorf("round-trip mismatch:\n got: %+v\nwant: %+v", got, ev)
		}
	}
}

func TestEncodeUnknownTypeFails(t *testing.T) {
	if _, err := EncodeEvent(42); err == nil {
		t.Errorf("expected error encoding int, got nil")
	}
}

func TestDecodeBadJSON(t *testing.T) {
	if _, err := DecodeEvent([]byte("not json")); err == nil {
		t.Errorf("expected error decoding bad JSON, got nil")
	}
}
