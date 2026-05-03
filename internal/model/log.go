package model

import "time"

type LogLevel string

const (
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

type LogEvent struct {
	Timestamp time.Time
	Level     LogLevel
	Body      string
	Service   string
	TraceID   string
	Attrs     map[string]string
}
