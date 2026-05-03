package agent

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/receiver"
)

func TestBroadcasterEndToEnd(t *testing.T) {
	b := NewBroadcaster()
	srv := httptest.NewServer(b)
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http", "ws", 1)

	out := make(chan any, 16)
	r := receiver.NewRemote("remote", wsURL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx, out); err != nil {
		t.Fatalf("remote start: %v", err)
	}
	defer r.Stop()

	// Wait for client to connect.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && b.ClientCount() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if b.ClientCount() != 1 {
		t.Fatalf("client count = %d, want 1", b.ClientCount())
	}

	// Drive the broadcaster's pump from a synthetic input channel.
	in := make(chan any, 4)
	go b.Pump(ctx, in)

	now := time.Now().UTC().Round(time.Microsecond)
	in <- model.MetricEvent{Name: "cpu", Value: 42, Source: "agent", Kind: model.Gauge, Timestamp: now}
	in <- model.LogEvent{Body: "ok", Service: "agent", Level: model.LevelInfo, Timestamp: now}

	got := []any{}
	timeout := time.After(3 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-out:
			got = append(got, ev)
		case <-timeout:
			t.Fatalf("timed out — got %d events: %#v", len(got), got)
		}
	}

	if me, ok := got[0].(model.MetricEvent); !ok || me.Name != "cpu" || me.Value != 42 {
		t.Errorf("first event = %+v, want MetricEvent cpu=42", got[0])
	}
	if le, ok := got[1].(model.LogEvent); !ok || le.Body != "ok" {
		t.Errorf("second event = %+v, want LogEvent body=ok", got[1])
	}
}
