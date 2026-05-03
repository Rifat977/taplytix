package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

const sampleTOML = `
[server]
refresh_ms = 500
log_file = "~/.taplytix/debug.log"
default_service = "my-api"

[[source]]
name = "my-api"
type = "otlp"
grpc = ":4317"
http = ":4318"

[[alert]]
name = "high-p99"
metric = "http.server.duration"
percentile = 99
op = ">"
threshold = 500
for = "30s"
notify = ["bell"]
`

func TestLoadFromString(t *testing.T) {
	cfg := Default()
	cfg.Sources = nil
	if _, err := toml.Decode(sampleTOML, cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(cfg.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(cfg.Sources))
	}
	if got := cfg.Sources[0].Name; got != "my-api" {
		t.Errorf("source name = %q, want %q", got, "my-api")
	}
	if len(cfg.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(cfg.Alerts))
	}
	if got := cfg.Alerts[0].For.Std(); got != 30*time.Second {
		t.Errorf("alert.for = %v, want 30s", got)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "taplytix.toml")
	if err := os.WriteFile(path, []byte(sampleTOML), 0o644); err != nil {
		t.Fatalf("write tmp config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sources[0].Name != "my-api" {
		t.Errorf("source name = %q, want %q", cfg.Sources[0].Name, "my-api")
	}
	if cfg.Server.RefreshMs != 500 {
		t.Errorf("refresh_ms = %d, want 500", cfg.Server.RefreshMs)
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Server.RefreshMs != 500 {
		t.Errorf("default refresh_ms = %d, want 500", cfg.Server.RefreshMs)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Type != "otlp" {
		t.Errorf("default sources = %+v, want one OTLP source", cfg.Sources)
	}
}
