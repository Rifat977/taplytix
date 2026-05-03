package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/alert"
)

func sampleAlert() alert.Alert {
	return alert.Alert{
		Rule:    alert.Rule{Name: "high-p99", Metric: "latency", Op: alert.OpGt, Threshold: 500},
		Service: "api",
		Value:   842,
		FiredAt: time.Date(2025, 5, 4, 12, 0, 0, 0, time.UTC),
	}
}

func TestBellWritesBEL(t *testing.T) {
	var buf bytes.Buffer
	b := &Bell{W: &buf}
	if err := b.Notify(context.Background(), sampleAlert()); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if buf.String() != "\x07" {
		t.Errorf("bell wrote %q, want BEL", buf.String())
	}
}

func TestWebhookSendsPayloadAndRetries(t *testing.T) {
	var attempts int32
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		lastBody = body
		if n < 2 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL)
	w.Client.Timeout = 2 * time.Second
	if err := w.Notify(context.Background(), sampleAlert()); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	var got map[string]any
	if err := json.Unmarshal(lastBody, &got); err != nil {
		t.Fatalf("payload parse: %v (raw: %s)", err, lastBody)
	}
	if got["alert"] != "high-p99" || got["service"] != "api" {
		t.Errorf("payload = %+v", got)
	}
}

func TestLogFileAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "alerts.log")
	l := NewLogFile(path)
	if err := l.Notify(context.Background(), sampleAlert()); err != nil {
		t.Fatalf("notify: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "FIRED") {
		t.Errorf("log missing FIRED marker: %q", string(data))
	}
	if !strings.Contains(string(data), "rule=high-p99") {
		t.Errorf("log missing rule name: %q", string(data))
	}
}
