package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

type ServerConfig struct {
	RefreshMs      int    `toml:"refresh_ms"`
	LogFile        string `toml:"log_file"`
	DefaultService string `toml:"default_service"`
}

type SourceConfig struct {
	Name     string   `toml:"name"`
	Type     string   `toml:"type"`
	GRPC     string   `toml:"grpc"`
	HTTP     string   `toml:"http"`
	Endpoint string   `toml:"endpoint"`
	Interval Duration `toml:"interval"`
	Listen   string   `toml:"listen"`
	Path     string   `toml:"path"`
	Format   string   `toml:"format"`
	Process  string   `toml:"process"`
	PID      int      `toml:"pid"`
	Cert     string   `toml:"cert"`
}

type AlertConfig struct {
	Name       string   `toml:"name"`
	Metric     string   `toml:"metric"`
	Percentile int      `toml:"percentile"`
	Op         string   `toml:"op"`
	Threshold  float64  `toml:"threshold"`
	For        Duration `toml:"for"`
	Notify     []string `toml:"notify"`
}

type WebhookNotifier struct {
	URL string `toml:"url"`
}

type LogfileNotifier struct {
	Path string `toml:"path"`
}

type NotifierConfig struct {
	Webhook WebhookNotifier `toml:"webhook"`
	Logfile LogfileNotifier `toml:"logfile"`
}

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Sources  []SourceConfig `toml:"source"`
	Alerts   []AlertConfig  `toml:"alert"`
	Notifier NotifierConfig `toml:"notifier"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			RefreshMs:      500,
			LogFile:        "~/.taplytix/debug.log",
			DefaultService: "my-app",
		},
		Sources: []SourceConfig{{
			Name: "my-app",
			Type: "otlp",
			GRPC: ":4317",
			HTTP: ":4318",
		}},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := Default()
	cfg.Sources = nil
	if _, err := toml.Decode(string(data), cfg); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}
	return cfg, nil
}
