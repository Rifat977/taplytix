package receiver

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

func TestParseStatsDLine(t *testing.T) {
	cases := []struct {
		in    string
		name  string
		val   float64
		kind  model.MetricKind
		labels map[string]string
	}{
		{"req.count:1|c", "req.count", 1, model.Counter, nil},
		{"mem.used:512|g", "mem.used", 512, model.Gauge, nil},
		{"req.duration:42|ms", "req.duration", 42, model.Histogram, nil},
		{"sampled:1|c|@0.1", "sampled", 10, model.Counter, nil}, // 1 / 0.1
		{"tagged:5|g|#region:us,env:prod", "tagged", 5, model.Gauge, map[string]string{"region": "us", "env": "prod"}},
	}
	for _, tc := range cases {
		ev, ok := parseStatsDLine(tc.in, "src")
		if !ok {
			t.Errorf("parse(%q) returned !ok", tc.in)
			continue
		}
		if ev.Name != tc.name || ev.Value != tc.val || ev.Kind != tc.kind {
			t.Errorf("parse(%q) = name=%s val=%v kind=%v, want %s/%v/%v",
				tc.in, ev.Name, ev.Value, ev.Kind, tc.name, tc.val, tc.kind)
		}
		for k, v := range tc.labels {
			if ev.Labels[k] != v {
				t.Errorf("parse(%q) label %s = %q, want %q", tc.in, k, ev.Labels[k], v)
			}
		}
	}
}

func TestParseStatsDInvalid(t *testing.T) {
	for _, in := range []string{"", "no_value", "bad:notanumber|c", "missing_type:1"} {
		if _, ok := parseStatsDLine(in, "src"); ok {
			t.Errorf("parse(%q) returned ok, want !ok", in)
		}
	}
}

func TestStatsDReceiverEndToEnd(t *testing.T) {
	r := NewStatsD("test", "127.0.0.1:0")
	// Custom listen with random port: ResolveUDPAddr will pick one.
	out := make(chan any, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx, out); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()
	addr := r.conn.LocalAddr().String()

	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping:7|c\nlatency:100|ms")); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.After(2 * time.Second)
	got := 0
	for got < 2 {
		select {
		case <-out:
			got++
		case <-deadline:
			t.Fatalf("timed out after %d events", got)
		}
	}
}
