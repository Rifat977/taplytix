package model

import "time"

type SpanStatus int

const (
	StatusUnset SpanStatus = iota
	StatusOK
	StatusError
)

func (s SpanStatus) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusError:
		return "error"
	default:
		return "unset"
	}
}

type SpanEvent struct {
	TraceID   string
	SpanID    string
	ParentID  string
	Name      string
	Service   string
	StartTime time.Time
	Duration  time.Duration
	Status    SpanStatus
	Attrs     map[string]string
}

type Trace struct {
	TraceID  string
	Root     *SpanEvent
	Children map[string][]*SpanEvent
	Duration time.Duration
}
