package receiver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

func TestParseJSONLog(t *testing.T) {
	line := `{"level":"error","msg":"db down","time":"2025-01-02T15:04:05Z","trace_id":"abc","user":"alice"}`
	ev, ok := parseJSON([]byte(line), "svc")
	if !ok {
		t.Fatal("parseJSON failed")
	}
	if ev.Level != model.LevelError {
		t.Errorf("level = %s, want ERROR", ev.Level)
	}
	if ev.Body != "db down" {
		t.Errorf("body = %q, want %q", ev.Body, "db down")
	}
	if ev.TraceID != "abc" {
		t.Errorf("traceID = %q, want abc", ev.TraceID)
	}
	if ev.Attrs["user"] != "alice" {
		t.Errorf("attrs.user = %q, want alice", ev.Attrs["user"])
	}
	if ev.Timestamp.Year() != 2025 {
		t.Errorf("timestamp = %v, want 2025", ev.Timestamp)
	}
}

func TestParseLogfmt(t *testing.T) {
	line := `level=warn msg="disk almost full" service=api free_pct=12`
	ev, ok := parseLogfmt(line, "fallback")
	if !ok {
		t.Fatal("parseLogfmt failed")
	}
	if ev.Level != model.LevelWarn {
		t.Errorf("level = %s, want WARN", ev.Level)
	}
	if ev.Body != "disk almost full" {
		t.Errorf("body = %q", ev.Body)
	}
	if ev.Service != "api" {
		t.Errorf("service = %q, want api", ev.Service)
	}
	if ev.Attrs["free_pct"] != "12" {
		t.Errorf("attrs.free_pct = %q, want 12", ev.Attrs["free_pct"])
	}
}

func TestParsePlainWithLevelPrefix(t *testing.T) {
	ev := parsePlain("[ERROR] something exploded", "svc")
	if ev.Level != model.LevelError {
		t.Errorf("level = %s, want ERROR", ev.Level)
	}
	if ev.Body != "[ERROR] something exploded" {
		t.Errorf("body = %q", ev.Body)
	}
}

func TestDetectFormat(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                    "json",
		`level=info msg=hi`:          "logfmt",
		`hello world`:                "plain",
		`2025-01-02 plain log line`:  "plain",
	}
	for in, want := range cases {
		if got := detectFormat(in); got != want {
			t.Errorf("detect(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestLogTailFileEmitsAppendedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("ignore-this\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r := NewLogTail("test", LogTailOptions{Path: path, Format: "plain", Service: "svc"})
	out := make(chan any, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx, out); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	// The file is opened and seeked-to-end before we append, so the existing
	// "ignore-this" line should not be re-emitted.
	time.Sleep(150 * time.Millisecond)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("append open: %v", err)
	}
	if _, err := f.WriteString("[ERROR] boom\n[INFO] alive\n"); err != nil {
		t.Fatalf("append write: %v", err)
	}
	_ = f.Close()

	deadline := time.After(2 * time.Second)
	got := []model.LogEvent{}
	for len(got) < 2 {
		select {
		case ev := <-out:
			le, ok := ev.(model.LogEvent)
			if !ok {
				t.Fatalf("unexpected type %T", ev)
			}
			got = append(got, le)
		case <-deadline:
			t.Fatalf("timed out — got %d events: %+v", len(got), got)
		}
	}
	if got[0].Level != model.LevelError || got[1].Level != model.LevelInfo {
		t.Errorf("levels = %s / %s, want ERROR / INFO", got[0].Level, got[1].Level)
	}
}
