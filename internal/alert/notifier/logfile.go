package notifier

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/alert"
)

// LogFile appends one line per alert to a configured path. The parent
// directory is created lazily.
type LogFile struct {
	Path string

	mu sync.Mutex
}

func NewLogFile(path string) *LogFile { return &LogFile{Path: path} }

func (l *LogFile) Name() string { return "logfile" }

func (l *LogFile) Notify(_ context.Context, a alert.Alert) error {
	if l.Path == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	state := "FIRED"
	if a.ResolvedAt != nil {
		state = "RESOLVED"
	}
	line := fmt.Sprintf("%s  %s  %s  rule=%s  metric=%s  value=%v  threshold=%v  service=%s\n",
		time.Now().UTC().Format(time.RFC3339),
		state, a.Rule.Op, a.Rule.Name, a.Rule.Metric,
		a.Value, a.Rule.Threshold, a.Service,
	)
	_, err = f.WriteString(line)
	return err
}
