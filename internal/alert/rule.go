package alert

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Op string

const (
	OpGt  Op = ">"
	OpLt  Op = "<"
	OpGte Op = ">="
	OpLte Op = "<="
	OpEq  Op = "=="
	OpNeq Op = "!="
)

// Rule defines a threshold check evaluated by the engine on every tick.
type Rule struct {
	Name       string
	Metric     string
	Percentile int           // 0 = raw last value, otherwise 50/95/99
	Op         Op
	Threshold  float64
	For        time.Duration // must be true for this long before firing
	Notify     []string      // notifier names
}

// Alert represents a single fired alert. ResolvedAt is nil while active.
type Alert struct {
	Rule       Rule
	Service    string
	Value      float64
	FiredAt    time.Time
	ResolvedAt *time.Time
}

// FiredMsg / ResolvedMsg are dispatched into the TUI program by the engine
// so the AppModel can maintain its active-alert list.
type FiredMsg struct{ Alert Alert }
type ResolvedMsg struct{ Alert Alert }

// Notifier delivers an alert through one channel (bell, webhook, file…).
type Notifier interface {
	Name() string
	Notify(ctx context.Context, a Alert) error
}

func (r Rule) Validate() error {
	if r.Name == "" {
		return errors.New("rule: name is required")
	}
	if r.Metric == "" {
		return fmt.Errorf("rule %q: metric is required", r.Name)
	}
	if r.Percentile < 0 || r.Percentile > 100 {
		return fmt.Errorf("rule %q: percentile must be in [0,100]", r.Name)
	}
	switch r.Op {
	case OpGt, OpLt, OpGte, OpLte, OpEq, OpNeq:
	default:
		return fmt.Errorf("rule %q: unknown op %q", r.Name, r.Op)
	}
	return nil
}

// Compare reports whether value op threshold holds.
func Compare(value, threshold float64, op Op) bool {
	switch op {
	case OpGt:
		return value > threshold
	case OpLt:
		return value < threshold
	case OpGte:
		return value >= threshold
	case OpLte:
		return value <= threshold
	case OpEq:
		return value == threshold
	case OpNeq:
		return value != threshold
	default:
		return false
	}
}
