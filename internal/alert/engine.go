package alert

import (
	"context"
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/bus"
	"github.com/rifat977/taplytix/internal/store"
)

// DefaultTick is the engine's evaluation interval.
const DefaultTick = 5 * time.Second

// Engine evaluates alert rules against the store on a fixed interval. When
// a rule's condition has held for at least Rule.For, an alert fires and the
// configured notifiers run asynchronously.
type Engine struct {
	store     *store.Store
	bus       *bus.Bus
	rules     []Rule
	notifiers map[string]Notifier
	tick      time.Duration

	mu     sync.Mutex
	firing map[string]*pending
}

type pending struct {
	rule        Rule
	service     string
	firingSince time.Time
	fired       bool
	lastValue   float64
	firedAlert  *Alert
}

func New(st *store.Store, b *bus.Bus, rules []Rule, notifiers []Notifier) *Engine {
	nm := make(map[string]Notifier, len(notifiers))
	for _, n := range notifiers {
		nm[n.Name()] = n
	}
	return &Engine{
		store:     st,
		bus:       b,
		rules:     rules,
		notifiers: nm,
		tick:      DefaultTick,
		firing:    make(map[string]*pending),
	}
}

// SetTick overrides the evaluation interval. Useful for tests.
func (e *Engine) SetTick(d time.Duration) {
	if d > 0 {
		e.tick = d
	}
}

// Active returns a snapshot of the currently fired (unresolved) alerts.
func (e *Engine) Active() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Alert, 0, len(e.firing))
	for _, p := range e.firing {
		if p.fired && p.firedAlert != nil {
			out = append(out, *p.firedAlert)
		}
	}
	return out
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.tick)
	defer ticker.Stop()
	// One immediate evaluation so newly-started engines respond before the
	// first tick interval elapses.
	e.evaluateOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluateOnce()
		}
	}
}

func (e *Engine) evaluateOnce() {
	now := time.Now()
	services := e.store.Services()
	seen := map[string]struct{}{}

	for _, rule := range e.rules {
		for _, svc := range services {
			metrics := e.store.MetricsFor(svc)
			series := findSeries(metrics, rule.Metric)
			if series == nil {
				continue
			}
			value, ok := computeValue(series, rule.Percentile)
			if !ok {
				continue
			}
			key := rule.Name + "::" + svc
			if Compare(value, rule.Threshold, rule.Op) {
				seen[key] = struct{}{}
				e.onBreach(rule, svc, key, value, now)
			} else {
				e.onClear(key, value, now)
			}
		}
	}

	// Resolve any firing entries whose metric disappeared this tick.
	e.mu.Lock()
	stale := make([]string, 0)
	for key, p := range e.firing {
		if _, ok := seen[key]; !ok && p.fired {
			stale = append(stale, key)
		}
	}
	e.mu.Unlock()
	for _, key := range stale {
		e.onClear(key, 0, now)
	}
}

func (e *Engine) onBreach(rule Rule, svc, key string, value float64, now time.Time) {
	e.mu.Lock()
	p, ok := e.firing[key]
	if !ok {
		p = &pending{rule: rule, service: svc, firingSince: now}
		e.firing[key] = p
	}
	p.lastValue = value
	already := p.fired
	shouldFire := !already && now.Sub(p.firingSince) >= rule.For
	if shouldFire {
		alert := Alert{
			Rule: rule, Service: svc, Value: value, FiredAt: now,
		}
		p.fired = true
		p.firedAlert = &alert
	}
	var firedAlert *Alert
	if shouldFire {
		firedAlert = p.firedAlert
	}
	e.mu.Unlock()

	if firedAlert != nil {
		e.dispatch(FiredMsg{Alert: *firedAlert})
		e.fanoutNotify(rule.Notify, *firedAlert)
	}
}

func (e *Engine) onClear(key string, value float64, now time.Time) {
	e.mu.Lock()
	p, ok := e.firing[key]
	if !ok {
		e.mu.Unlock()
		return
	}
	wasFired := p.fired
	var resolved Alert
	if wasFired && p.firedAlert != nil {
		resolved = *p.firedAlert
		t := now
		resolved.ResolvedAt = &t
		resolved.Value = value
	}
	delete(e.firing, key)
	e.mu.Unlock()

	if wasFired {
		e.dispatch(ResolvedMsg{Alert: resolved})
		e.fanoutNotify(resolved.Rule.Notify, resolved)
	}
}

func (e *Engine) dispatch(msg any) {
	if e.bus != nil {
		e.bus.Dispatch(msg)
	}
}

func (e *Engine) fanoutNotify(names []string, a Alert) {
	for _, name := range names {
		n, ok := e.notifiers[name]
		if !ok {
			continue
		}
		go func(n Notifier) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = n.Notify(ctx, a)
		}(n)
	}
}

// computeValue extracts the numeric value the rule should compare against.
// Percentile == 0 → raw last sample; otherwise the percentile of the series.
// Returns ok=false when the series is empty.
func computeValue(s *store.Series, p int) (float64, bool) {
	if s.Len() == 0 {
		return 0, false
	}
	if p == 0 {
		return s.Last(), true
	}
	return store.Percentile(s.Values(), float64(p)), true
}

// findSeries locates a series in the per-service metrics map by raw metric
// name, ignoring labels in the key.
func findSeries(metrics map[string]*store.Series, metricName string) *store.Series {
	if s, ok := metrics[metricName]; ok {
		return s
	}
	for _, s := range metrics {
		if s.Name == metricName {
			return s
		}
	}
	return nil
}
