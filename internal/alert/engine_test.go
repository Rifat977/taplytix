package alert

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/bus"
	"github.com/rifat977/taplytix/internal/model"
	"github.com/rifat977/taplytix/internal/store"
)

type recordingNotifier struct {
	name     string
	count    int32
	last     Alert
	mu       sync.Mutex
	fired    chan struct{}
	signaled int32
}

func (r *recordingNotifier) Name() string { return r.name }
func (r *recordingNotifier) Notify(_ context.Context, a Alert) error {
	atomic.AddInt32(&r.count, 1)
	r.mu.Lock()
	r.last = a
	r.mu.Unlock()
	if r.fired != nil && atomic.CompareAndSwapInt32(&r.signaled, 0, 1) {
		close(r.fired)
	}
	return nil
}

func TestEngineFiresAfterForDuration(t *testing.T) {
	st := store.New()
	b := bus.New()
	rule := Rule{
		Name: "high-p99", Metric: "latency", Percentile: 99,
		Op: OpGt, Threshold: 50, For: 60 * time.Millisecond,
		Notify: []string{"bell"},
	}
	bell := &recordingNotifier{name: "bell", fired: make(chan struct{})}
	e := New(st, b, []Rule{rule}, []Notifier{bell})
	e.SetTick(20 * time.Millisecond)

	for i := 0; i < 5; i++ {
		st.PushMetric(model.MetricEvent{
			Name: "latency", Value: 100, Source: "svc", Kind: model.Histogram, Timestamp: time.Now(),
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)

	select {
	case <-bell.fired:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected alert to fire within 2s, got %d notifications", atomic.LoadInt32(&bell.count))
	}
	if active := e.Active(); len(active) != 1 || active[0].Rule.Name != "high-p99" {
		t.Errorf("active = %+v, want one entry", active)
	}
}

func TestEngineDoesNotFireBelowThreshold(t *testing.T) {
	st := store.New()
	b := bus.New()
	rule := Rule{
		Name: "high-p99", Metric: "latency", Percentile: 99,
		Op: OpGt, Threshold: 1000, For: 30 * time.Millisecond,
	}
	bell := &recordingNotifier{name: "bell"}
	e := New(st, b, []Rule{rule}, []Notifier{bell})
	e.SetTick(15 * time.Millisecond)

	for i := 0; i < 5; i++ {
		st.PushMetric(model.MetricEvent{
			Name: "latency", Value: 100, Source: "svc", Kind: model.Histogram,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	e.Run(ctx)

	if got := atomic.LoadInt32(&bell.count); got != 0 {
		t.Errorf("notifier fired %d time(s), want 0", got)
	}
	if len(e.Active()) != 0 {
		t.Errorf("active alerts = %+v, want none", e.Active())
	}
}

func TestEngineResolvesWhenConditionClears(t *testing.T) {
	st := store.New()
	b := bus.New()
	rule := Rule{
		Name: "burst", Metric: "x", Op: OpGt, Threshold: 10,
		For: 30 * time.Millisecond, Notify: []string{"bell"},
	}
	bell := &recordingNotifier{name: "bell", fired: make(chan struct{})}
	e := New(st, b, []Rule{rule}, []Notifier{bell})
	e.SetTick(10 * time.Millisecond)

	st.PushMetric(model.MetricEvent{Name: "x", Value: 100, Source: "svc", Kind: model.Gauge})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)

	select {
	case <-bell.fired:
	case <-time.After(2 * time.Second):
		t.Fatalf("first fire never happened")
	}

	// Simulate condition clearing: push a low value, displacing the high one.
	for i := 0; i < 600; i++ {
		st.PushMetric(model.MetricEvent{Name: "x", Value: 0, Source: "svc", Kind: model.Gauge})
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(e.Active()) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("alert never resolved; active = %+v", e.Active())
}

func TestRuleValidate(t *testing.T) {
	cases := []struct {
		r       Rule
		wantErr bool
	}{
		{Rule{Name: "n", Metric: "m", Op: OpGt, Threshold: 1}, false},
		{Rule{Metric: "m", Op: OpGt}, true},                       // missing name
		{Rule{Name: "n", Op: OpGt}, true},                          // missing metric
		{Rule{Name: "n", Metric: "m", Op: "??"}, true},             // bad op
		{Rule{Name: "n", Metric: "m", Op: OpGt, Percentile: 200}, true}, // bad pct
	}
	for i, tc := range cases {
		err := tc.r.Validate()
		if (err != nil) != tc.wantErr {
			t.Errorf("case %d: err=%v, wantErr=%v", i, err, tc.wantErr)
		}
	}
}

func TestCompareAllOps(t *testing.T) {
	cases := []struct {
		v, t float64
		op   Op
		want bool
	}{
		{5, 3, OpGt, true},
		{3, 3, OpGte, true},
		{2, 3, OpLt, true},
		{3, 3, OpLte, true},
		{3, 3, OpEq, true},
		{3, 4, OpNeq, true},
		{3, 3, OpGt, false},
	}
	for _, tc := range cases {
		if got := Compare(tc.v, tc.t, tc.op); got != tc.want {
			t.Errorf("Compare(%v %s %v) = %v, want %v", tc.v, tc.op, tc.t, got, tc.want)
		}
	}
}
