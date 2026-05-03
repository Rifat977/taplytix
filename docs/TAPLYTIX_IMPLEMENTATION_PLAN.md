# Taplytix — Implementation Plan
> Terminal-native developer monitoring dashboard · Go + Charm ecosystem + OpenTelemetry

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Tech Stack & Dependencies](#2-tech-stack--dependencies)
3. [Repository Structure](#3-repository-structure)
4. [Implementation Phases](#4-implementation-phases)
   - [Phase 1 — Skeleton & Config](#phase-1--skeleton--config)
   - [Phase 2 — OTLP Receiver & Data Model](#phase-2--otlp-receiver--data-model)
   - [Phase 3 — Store & Bus](#phase-3--store--bus)
   - [Phase 4 — Bubble Tea TUI Shell](#phase-4--bubble-tea-tui-shell)
   - [Phase 5 — Overview Panel](#phase-5--overview-panel)
   - [Phase 6 — Traces Panel](#phase-6--traces-panel)
   - [Phase 7 — Metrics Panel](#phase-7--metrics-panel)
   - [Phase 8 — Logs Panel](#phase-8--logs-panel)
   - [Phase 9 — Fallback Receivers](#phase-9--fallback-receivers)
   - [Phase 10 — Alert Engine](#phase-10--alert-engine)
   - [Phase 11 — Services Panel & Multi-service](#phase-11--services-panel--multi-service)
   - [Phase 12 — Remote Agent & SSH Mode](#phase-12--remote-agent--ssh-mode)
5. [Data Models](#5-data-models)
6. [Key Interfaces](#6-key-interfaces)
7. [Configuration Schema](#7-configuration-schema)
8. [Claude Code Prompts — Phase by Phase](#8-claude-code-prompts--phase-by-phase)
9. [Testing Strategy](#9-testing-strategy)
10. [Done Checklist](#10-done-checklist)
11. [TUI Screen Examples](#11-tui-screen-examples)

---

## 1. Project Overview

**Taplytix** is a single-binary, terminal-native developer monitoring dashboard.
It ingests telemetry from *any* application stack via OpenTelemetry (OTLP),
Prometheus, StatsD, or plain log files — and renders traces, metrics, and logs
in a rich Bubble Tea TUI, with no cloud account, no Docker, and no browser required.

### Target user

A developer running an app locally who wants to see traces, latency percentiles,
memory usage, and structured logs in a split terminal pane — without switching
context to a browser.

### Design goals

| Goal | Constraint |
|---|---|
| Zero-infra local dev | Single static binary, no Docker, no DB |
| Any language / stack | OTLP standard — no per-language agent |
| Terminal-first | Bubble Tea TUI, Lip Gloss styling |
| Trace-level bottleneck insight | Span waterfall with P50/P95/P99 |
| Extensible to production | Optional remote agent + SSH access via Wish |

---

## 2. Tech Stack & Dependencies

```toml
# go.mod
module github.com/rifat977/taplytix

go 1.22

require (
  # Charm ecosystem
  github.com/charmbracelet/bubbletea       v1.2.4
  github.com/charmbracelet/lipgloss        v1.0.0
  github.com/charmbracelet/bubbles         v0.20.0
  github.com/charmbracelet/huh             v0.6.0
  github.com/charmbracelet/log             v0.4.0
  github.com/charmbracelet/wish            v1.4.3   # optional SSH server

  # OpenTelemetry — receiver side only
  go.opentelemetry.io/collector/pdata      v1.16.0
  go.opentelemetry.io/collector/receiver/otlpreceiver v0.110.0

  # Prometheus text parser (no server — read-only)
  github.com/prometheus/common             v0.60.0

  # Config
  github.com/BurntSushi/toml               v1.4.0

  # CLI
  github.com/spf13/cobra                   v1.8.1
)
```

> **Rule:** Never add a dependency that requires a running daemon, a database,
> or a cloud endpoint. The binary must work fully offline.

---

## 3. Repository Structure

```
taplytix/
│
├── cmd/
│   └── taplytix/
│       └── main.go                  # cobra root + subcommands
│
├── internal/
│   │
│   ├── config/
│   │   ├── config.go                # Config struct, Load(), defaults
│   │   └── config_test.go
│   │
│   ├── model/
│   │   ├── metric.go                # MetricEvent, MetricKind
│   │   ├── span.go                  # SpanEvent, SpanStatus
│   │   └── log.go                   # LogEvent
│   │
│   ├── receiver/
│   │   ├── receiver.go              # Receiver interface
│   │   ├── otlp.go                  # OTLP gRPC :4317 + HTTP :4318
│   │   ├── prometheus.go            # HTTP scrape poller
│   │   ├── statsd.go                # UDP :8125 listener
│   │   ├── logtail.go               # file / stdin / TCP log reader
│   │   └── os.go                    # /proc, ps — OS sidecar
│   │
│   ├── store/
│   │   ├── ring.go                  # generic ring buffer [T any]
│   │   ├── timeseries.go            # per-metric series + windowed aggregates
│   │   ├── percentile.go            # P50 / P95 / P99 over sliding window
│   │   ├── tracemap.go              # assembles spans into trace trees
│   │   └── store.go                 # Store: unified access point
│   │
│   ├── bus/
│   │   └── bus.go                   # MetricsBus: Publish / Subscribe / Dispatch
│   │
│   ├── alert/
│   │   ├── rule.go                  # Rule, Op, Notifier interface
│   │   ├── engine.go                # evaluates rules on each tick
│   │   └── notifier/
│   │       ├── bell.go              # terminal bell \a
│   │       ├── webhook.go           # HTTP POST JSON payload
│   │       └── logfile.go           # append to file
│   │
│   ├── tui/
│   │   ├── app.go                   # root tea.Model, tab routing
│   │   ├── keymap.go                # key bindings (Tab, /, q, e, ?)
│   │   ├── statusbar.go             # bottom status bar component
│   │   └── panels/
│   │       ├── panel.go             # Panel interface
│   │       ├── overview.go          # vitals + sparklines
│   │       ├── traces.go            # span waterfall
│   │       ├── metrics.go           # histograms + P-tile table
│   │       ├── logs.go              # viewport + /filter textinput
│   │       └── services.go          # multi-service selector
│   │
│   └── render/
│       ├── sparkline.go             # Lip Gloss sparkline string builder
│       ├── barchart.go              # horizontal bar chart
│       ├── histogram.go             # bucketed histogram
│       ├── waterfall.go             # trace span waterfall (unicode blocks)
│       └── theme.go                 # Lip Gloss style definitions + colors
│
├── agent/
│   └── main.go                      # remote sidecar binary (VPS)
│
├── pulseboard.toml                  # renamed → taplytix.toml example
├── taplytix.toml                    # default config example
├── Makefile
├── README.md
└── go.mod
```

---

## 4. Implementation Phases

Each phase is a self-contained Claude Code session.
Complete each phase and verify it runs before moving to the next.

---

### Phase 1 — Skeleton & Config

**Goal:** Working CLI that reads config and prints a startup message.

#### Tasks

- [ ] Init Go module `github.com/rifat977/taplytix`
- [ ] Add `go.mod` with all dependencies listed in §2
- [ ] `cmd/taplytix/main.go` — cobra root command with subcommands:
  - `taplytix start` — main entry (default)
  - `taplytix init` — interactive setup wizard (Huh)
  - `taplytix version`
- [ ] `internal/config/config.go`
  - `Config` struct matching §7 schema
  - `Load(path string) (*Config, error)` using BurntSushi/toml
  - `Default() *Config` — sensible defaults (OTLP :4317, refresh 500ms)
- [ ] `internal/config/config_test.go` — one test: load a toml string, assert source name matches
- [ ] `taplytix.toml` example file
- [ ] `Makefile` with targets: `build`, `test`, `run`, `lint`

#### Acceptance criteria

```bash
go build ./...          # compiles with zero errors
taplytix version        # prints "taplytix v0.1.0"
taplytix start          # prints "starting taplytix — config loaded" and exits cleanly
```

---

### Phase 2 — OTLP Receiver & Data Model

**Goal:** Receive real OTLP data from any OTel SDK and convert to internal types.

#### Tasks

- [ ] `internal/model/metric.go`
  ```go
  type MetricKind int
  const (Counter MetricKind = iota; Gauge; Histogram)
  type MetricEvent struct {
      Name      string
      Value     float64
      Labels    map[string]string
      Kind      MetricKind
      Timestamp time.Time
      Source    string
  }
  ```
- [ ] `internal/model/span.go`
  ```go
  type SpanStatus int
  const (StatusUnset SpanStatus = iota; StatusOK; StatusError)
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
  ```
- [ ] `internal/model/log.go`
  ```go
  type LogEvent struct {
      Timestamp time.Time
      Level     string
      Body      string
      Service   string
      Attrs     map[string]string
  }
  ```
- [ ] `internal/receiver/receiver.go` — `Receiver` interface:
  ```go
  type Receiver interface {
      Start(ctx context.Context, out chan<- any) error
      Stop() error
      Name() string
  }
  ```
- [ ] `internal/receiver/otlp.go`
  - Start gRPC server on `:4317`, HTTP on `:4318`
  - Implement `pdata.TracesConsumer`, `MetricsConsumer`, `LogsConsumer`
  - Normalise `pdata.Span` → `model.SpanEvent`
  - Normalise `pdata.NumberDataPoint` → `model.MetricEvent`
  - Normalise `pdata.LogRecord` → `model.LogEvent`
  - Send all three to `out chan<- any`

#### Acceptance criteria

```bash
# In terminal 1: run a Node.js app with OTLP exporter → localhost:4317
# In terminal 2:
go test ./internal/receiver/... -v
# must print received spans/metrics/logs without errors
```

---

### Phase 3 — Store & Bus

**Goal:** Time-series store with percentile math and fan-out event bus.

#### Tasks

- [ ] `internal/store/ring.go`
  - `Ring[T any]` — fixed-capacity circular buffer
  - `Push(v T)`, `Slice() []T`, `Len() int`
  - Thread-safe with `sync.RWMutex`

- [ ] `internal/store/percentile.go`
  - `Percentile(values []float64, p float64) float64`
  - Uses copy + sort (never modifies the ring's slice in place)

- [ ] `internal/store/timeseries.go`
  - `Series` — holds a `Ring[MetricEvent]` per metric name + label set
  - `Push(e MetricEvent)`
  - `P50() / P95() / P99() float64`
  - `Sparkline(n int) []float64` — last n values for sparkline rendering

- [ ] `internal/store/tracemap.go`
  - `TraceMap` — assembles `SpanEvent`s into `Trace` trees by TraceID
  - `Add(span SpanEvent)`
  - `Get(traceID string) (*Trace, bool)`
  - `Recent(n int) []*Trace` — last n completed traces
  - Evict traces older than configurable TTL

- [ ] `internal/store/store.go`
  - `Store` — top-level unified store
  - `PushMetric(e MetricEvent)`
  - `PushSpan(e SpanEvent)`
  - `PushLog(e LogEvent)`
  - `Metrics(name string) *Series`
  - `Traces() *TraceMap`
  - `Logs() []LogEvent`

- [ ] `internal/bus/bus.go`
  - `Bus` — fan-out channel
  - `Publish(event any)`
  - `Subscribe() <-chan any`
  - `Dispatch(msg tea.Msg)` — sends to Bubble Tea program
  - Drops events if subscriber buffer is full (never blocks publisher)

#### Acceptance criteria

```bash
go test ./internal/store/... -v
go test ./internal/bus/... -v
# Ring eviction, percentile math, and trace assembly all tested
```

---

### Phase 4 — Bubble Tea TUI Shell

**Goal:** Running TUI with tab bar, keyboard routing, empty panels, and status bar.

#### Tasks

- [ ] `internal/render/theme.go`
  - Lip Gloss style definitions:
    ```go
    var (
        ColorPrimary   = lipgloss.Color("#58A6FF")
        ColorSuccess   = lipgloss.Color("#3FB950")
        ColorWarning   = lipgloss.Color("#E3B341")
        ColorDanger    = lipgloss.Color("#F85149")
        ColorMuted     = lipgloss.Color("#8B949E")
        ColorBg        = lipgloss.Color("#161B22")
        BorderStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
        TabActive       = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
        TabInactive     = lipgloss.NewStyle().Foreground(ColorMuted)
    )
    ```

- [ ] `internal/tui/panels/panel.go`
  ```go
  type Panel interface {
      tea.Model
      SetSize(width, height int)
      Title() string
  }
  ```

- [ ] `internal/tui/keymap.go`
  ```go
  type KeyMap struct {
      NextTab  key.Binding  // Tab
      PrevTab  key.Binding  // Shift+Tab
      Filter   key.Binding  // /
      Export   key.Binding  // e
      Help     key.Binding  // ?
      Quit     key.Binding  // q / ctrl+c
  }
  ```

- [ ] `internal/tui/statusbar.go`
  - Bottom bar: alert count · events/s · uptime · connection status

- [ ] `internal/tui/app.go`
  - Root `AppModel` implementing `tea.Model`
  - Holds `[]Panel`, `activeTab int`
  - `tea.WindowSizeMsg` → calls `panel.SetSize()` on all panels
  - `tea.KeyMsg` → routes Tab / panel-specific keys
  - `tickMsg` every `config.RefreshMs` → triggers store read + panel update
  - Renders: tab bar + active panel + status bar

- [ ] Wire `taplytix start` to launch `tea.NewProgram(app)`

#### Acceptance criteria

```bash
taplytix start
# TUI opens, Tab switches between 5 empty panels
# q quits cleanly
# Resize terminal → panels reflow without crash
```

---

### Phase 5 — Overview Panel

**Goal:** Live vital signs with sparklines and the top slowest operations table.

#### Tasks

- [ ] `internal/render/sparkline.go`
  - `Sparkline(values []float64, width int, color lipgloss.Color) string`
  - Uses unicode block chars: `▁▂▃▄▅▆▇█`
  - Normalises values to 0–8 range for block selection

- [ ] `internal/tui/panels/overview.go`
  - Four metric cards (Lip Gloss styled boxes):
    - P99 latency (red if > threshold)
    - P50 latency
    - Heap in use (from OS sidecar or OTel metric)
    - Active spans
  - Each card: label + current value + sparkline (last 60 samples)
  - Slowest operations table (bubbles/table):
    - Columns: span name · P95 duration · request count · error rate
    - Sorted by P95 desc
    - Updates every tick

- [ ] Connect panel to `Store` via tick message

#### Acceptance criteria

```bash
# Send OTLP traces from any app
taplytix start
# Overview tab shows live sparklines and slowest operations
```

---

### Phase 6 — Traces Panel

**Goal:** Span waterfall showing trace trees with duration bars.

#### Tasks

- [ ] `internal/render/waterfall.go`
  - `WaterfallRow(span SpanEvent, totalDuration time.Duration, indent int, termWidth int) string`
  - Renders: `indent + span name + unicode bar (█) + duration`
  - Bar width proportional to `span.Duration / totalDuration * availableWidth`
  - Color: green (OK), red (Error), amber (> 200ms)
  - Uses `▏▎▍▌▋▊▉█` for sub-character precision

- [ ] `internal/tui/panels/traces.go`
  - Top section: list of recent traces (service · root span · total duration)
  - Bottom section: selected trace waterfall
  - Navigation: `↑↓` to select trace, `Enter` to expand, `Esc` to collapse
  - Uses `bubbles/viewport` for scrollable waterfall
  - Highlights the slowest span in each trace with `ColorDanger`

#### Acceptance criteria

```bash
# Send multi-span traces (e.g. HTTP → DB → cache)
# Traces panel shows waterfall with correct indentation and bar widths
# Selecting a trace and pressing Enter expands the full span tree
```

---

### Phase 7 — Metrics Panel

**Goal:** Per-metric histogram bars and percentile breakdown table.

#### Tasks

- [ ] `internal/render/histogram.go`
  - `Histogram(buckets []Bucket, maxWidth int) string`
  - `Bucket{Label string; Count int}`
  - Horizontal bars using `█` characters, label left-aligned

- [ ] `internal/render/barchart.go`
  - `HBar(label string, value, maxValue float64, width int, color lipgloss.Color) string`
  - Used for per-route latency bars in metrics panel

- [ ] `internal/tui/panels/metrics.go`
  - Left column: list of all metric names (filterable with `/`)
  - Right column (on metric select):
    - Current value
    - P50 / P95 / P99 over last 60s / 5min / 15min windows
    - Histogram of value distribution
    - Sparkline over time
  - `bubbles/textinput` for metric name filter

#### Acceptance criteria

```bash
# Metrics panel lists all received OTLP metrics
# Selecting http.server.duration shows P50/P95/P99 and histogram
```

---

### Phase 8 — Logs Panel

**Goal:** Scrollable, filterable, level-coloured log tail.

#### Tasks

- [ ] `internal/tui/panels/logs.go`
  - `bubbles/viewport` for log lines (scrollable)
  - `bubbles/textinput` for live filter (activated with `/`)
  - Log line format: `timestamp  [LEVEL]  body  attrs...`
  - Level colors:
    - `DEBUG` → `ColorMuted`
    - `INFO`  → `ColorSuccess`
    - `WARN`  → `ColorWarning`
    - `ERROR` → `ColorDanger` + background highlight
  - Filter applies to body + attrs (case-insensitive substring)
  - Auto-scroll to bottom on new logs; pauses on manual scroll up

- [ ] Log ingestion from OTLP logs signal
- [ ] Log ingestion from `logtail.go` (file / stdin)
  - Parse plain text, logfmt, and JSON log formats
  - Extract `level`, `msg`/`message`, `time`/`timestamp` fields

#### Acceptance criteria

```bash
# taplytix start --logs ./app.log
# Logs panel shows coloured, scrollable log tail
# Typing /error filters to error lines only
# New logs auto-append; scrolling up pauses; pressing G jumps to bottom
```

---

### Phase 9 — Fallback Receivers

**Goal:** Support apps that don't use OTel SDK — Prometheus scrape, StatsD, OS sidecar.

#### Tasks

- [ ] `internal/receiver/prometheus.go`
  - Poll `endpoint` every `interval`
  - Parse Prometheus text exposition format using `prometheus/common/expfmt`
  - Convert to `[]MetricEvent` and publish to bus
  - Handle 404 / connection refused gracefully (log warning, retry)

- [ ] `internal/receiver/statsd.go`
  - UDP listener on `:8125`
  - Parse `metric.name:value|type` datagrams (counter `c`, gauge `g`, timer `ms`)
  - Convert to `MetricEvent` and publish

- [ ] `internal/receiver/os.go`
  - Poll every 2s:
    - Linux: parse `/proc/<pid>/stat` for CPU, `/proc/<pid>/status` for RSS
    - macOS: use `ps -o pid,pcpu,rss -p <pid>` subprocess
  - Publish as `MetricEvent` with source `"os"`
  - Auto-discover PID by process name if `pid = 0` in config

- [ ] `internal/receiver/logtail.go`
  - File tail: read new lines since last offset (like `tail -f`)
  - stdin: read lines from `os.Stdin`
  - TCP: listen on configurable port, accept line-delimited log streams
  - Detect format: JSON (`{`), logfmt (`key=val`), plain text

#### Acceptance criteria

```bash
# taplytix start --prom http://localhost:9090/metrics
# Overview shows metrics from Prometheus endpoint
```

---

### Phase 10 — Alert Engine

**Goal:** Threshold-based alerting with pluggable notifiers.

#### Tasks

- [ ] `internal/alert/rule.go`
  ```go
  type Op string
  const (OpGt Op = ">"; OpLt = "<"; OpGte = ">="; OpLte = "<=")

  type Rule struct {
      Name       string
      Metric     string
      Percentile int           // 0 = raw value, 50/95/99
      Op         Op
      Threshold  float64
      For        time.Duration // must be true for this long before firing
      Notify     []string      // notifier names
  }

  type Alert struct {
      Rule      Rule
      Value     float64
      FiredAt   time.Time
      ResolvedAt *time.Time
  }
  ```

- [ ] `internal/alert/notifier/bell.go`
  - Write `\a` to stderr (terminal bell)

- [ ] `internal/alert/notifier/webhook.go`
  - HTTP POST JSON `{"alert": name, "value": v, "threshold": t, "fired_at": ts}`
  - Configurable URL, 5s timeout, 3 retries

- [ ] `internal/alert/notifier/logfile.go`
  - Append alert line to `~/.taplytix/alerts.log`

- [ ] `internal/alert/engine.go`
  - Subscribes to `Bus`
  - On each tick: evaluates all rules against current store values
  - Tracks firing duration with a `map[ruleName]time.Time`
  - Fires alert only after `rule.For` duration is exceeded
  - Auto-resolves when condition clears
  - Sends `AlertMsg` to Bubble Tea program (shown in status bar)

#### Acceptance criteria

```bash
# Add rule: P99 > 500ms for 10s
# Generate slow requests
# Within 10s of P99 exceeding 500ms: terminal bell fires, status bar shows alert
```

---

### Phase 11 — Services Panel & Multi-service

**Goal:** Monitor multiple services simultaneously, with a per-service selector.

#### Tasks

- [ ] Update `Config` to support multiple `[[source]]` blocks (already in schema)

- [ ] Update `Store` to namespace all events by `source.name`
  - `store.MetricsFor(service string) map[string]*Series`
  - `store.TracesFor(service string) *TraceMap`
  - `store.LogsFor(service string) []LogEvent`

- [ ] `internal/tui/panels/services.go`
  - Left column: service list with connection status indicator
    - `● green` = connected + receiving data
    - `● yellow` = connected, no data in last 10s
    - `● red` = connection error
  - Right column: per-service mini overview (P99, heap, error rate)
  - `Enter` on a service → switches all other panels to show that service's data

- [ ] Update Overview / Traces / Metrics / Logs panels to be service-aware
  - Accept `activeService string` in their update messages
  - Render data filtered to the active service

#### Acceptance criteria

```bash
# taplytix.toml with two [[source]] blocks (two different apps)
# Services panel shows both, with status indicators
# Switching service updates all other panels
```

---

### Phase 12 — Remote Agent & SSH Mode

**Goal:** Extend Taplytix to VPS/production without changing the TUI code.

#### Tasks

- [ ] `agent/main.go` — `taplytix-agent` binary
  - Runs on the remote host
  - Starts a local OTLP receiver (same as Phase 2)
  - Forwards all events over a WebSocket connection to the dashboard
  - mTLS for secure transport
  - `taplytix-agent --listen :7777 --cert agent.crt --key agent.key`

- [ ] `internal/receiver/remote.go`
  - Connects to a `taplytix-agent` WebSocket endpoint
  - Reconnects on disconnect with exponential backoff
  - Decodes forwarded events and publishes to local bus

- [ ] Update config schema:
  ```toml
  [[source]]
  name = "prod-api"
  type = "remote"
  endpoint = "wss://prod-host:7777"
  cert = "~/.taplytix/client.crt"
  ```

- [ ] SSH mode via `charmbracelet/wish`
  - `taplytix serve --ssh :2222`
  - Wraps the full Bubble Tea program in a Wish SSH server
  - Each SSH connection gets its own `tea.Program` instance
  - `ssh dev@prod-host -p 2222` opens the full dashboard remotely

#### Acceptance criteria

```bash
# On remote VPS: taplytix-agent --listen :7777
# Locally: taplytix start (config has remote source)
# Services panel shows remote service with live data
# ssh localhost -p 2222 opens the TUI over SSH
```

---

## 5. Data Models

```go
// internal/model/metric.go
package model

import "time"

type MetricKind int
const (
    Counter   MetricKind = iota
    Gauge
    Histogram
)

type MetricEvent struct {
    Name      string
    Value     float64
    Labels    map[string]string
    Kind      MetricKind
    Timestamp time.Time
    Source    string // receiver name
}

// internal/model/span.go
type SpanStatus int
const (
    StatusUnset SpanStatus = iota
    StatusOK
    StatusError
)

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
    Children map[string][]*SpanEvent // parentID → child spans
    Duration time.Duration
}

// internal/model/log.go
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
    TraceID   string // correlate with spans if present
    Attrs     map[string]string
}
```

---

## 6. Key Interfaces

```go
// Receiver — anything that produces events
type Receiver interface {
    Start(ctx context.Context, out chan<- any) error
    Stop() error
    Name() string
}

// Panel — any Bubble Tea TUI panel
type Panel interface {
    tea.Model
    SetSize(width, height int)
    Title() string
}

// Notifier — anything that delivers an alert
type Notifier interface {
    Notify(ctx context.Context, alert Alert) error
    Name() string
}

// Store — read access for panels
type Store interface {
    MetricsFor(service string) map[string]*timeseries.Series
    TracesFor(service string) *tracemap.TraceMap
    LogsFor(service string) []model.LogEvent
    Services() []string
}
```

---

## 7. Configuration Schema

```toml
# taplytix.toml

[server]
refresh_ms   = 500
log_file     = "~/.taplytix/debug.log"
default_service = "my-app"

# ── Sources (one or more) ─────────────────────────────
[[source]]
name     = "my-api"
type     = "otlp"
grpc     = ":4317"
http     = ":4318"

[[source]]
name     = "my-api"
type     = "prometheus"
endpoint = "http://localhost:9090/metrics"
interval = "2s"

[[source]]
name     = "my-api"
type     = "statsd"
listen   = ":8125"

[[source]]
name   = "my-api"
type   = "logs"
path   = "./logs/app.log"
format = "json"          # "logfmt" | "plain" | "json"

[[source]]
name    = "os"
type    = "sysstat"
pid     = 0              # 0 = auto-detect by process name
process = "my-api"

[[source]]
name     = "prod-api"
type     = "remote"
endpoint = "wss://prod-host:7777"
cert     = "~/.taplytix/client.crt"

# ── Alerts ────────────────────────────────────────────
[[alert]]
name        = "high-p99"
metric      = "http.server.duration"
percentile  = 99
op          = ">"
threshold   = 500       # ms
for         = "30s"
notify      = ["bell", "webhook"]

[[alert]]
name      = "heap-pressure"
metric    = "process.runtime.go.mem.heap_inuse"
op        = ">"
threshold = 500_000_000 # 500 MB
for       = "10s"
notify    = ["bell"]

[notifier.webhook]
url = "https://hooks.example.com/taplytix"
```

---

## 8. Claude Code Prompts — Phase by Phase

Copy these prompts directly into Claude Code to implement each phase.

---

### Prompt — Phase 1

```
Create the initial skeleton for a Go project called "taplytix".

Module path: github.com/rifat977/taplytix

Tasks:
1. Create go.mod with dependencies:
   - github.com/charmbracelet/bubbletea v1.2.4
   - github.com/charmbracelet/lipgloss v1.0.0
   - github.com/charmbracelet/bubbles v0.20.0
   - github.com/charmbracelet/huh v0.6.0
   - github.com/charmbracelet/log v0.4.0
   - github.com/BurntSushi/toml v1.4.0
   - github.com/spf13/cobra v1.8.1

2. Create cmd/taplytix/main.go with cobra subcommands:
   - "start" (default) — prints "starting taplytix" and exits
   - "version" — prints "taplytix v0.1.0"
   - "init" — prints "setup wizard coming in phase 4"

3. Create internal/config/config.go with:
   - Config struct (fields: RefreshMs int, LogFile string, Sources []SourceConfig, Alerts []AlertConfig)
   - SourceConfig struct (Name, Type, GRPC, HTTP, Endpoint, Interval, Path, Format, Listen, Process, PID)
   - AlertConfig struct (Name, Metric string, Percentile int, Op, Threshold float64, For duration, Notify []string)
   - Load(path string) (*Config, error) using BurntSushi/toml
   - Default() *Config returning sensible defaults

4. Create internal/config/config_test.go with one test: load a TOML string, assert source name matches

5. Create taplytix.toml example file with one OTLP source and one alert

6. Create Makefile with targets: build, test, run, lint

All code must compile with `go build ./...`
```

---

### Prompt — Phase 2

```
Implement the data model and OTLP receiver for taplytix.

1. Create internal/model/metric.go, span.go, log.go with the exact structs
   defined in the implementation plan §5.

2. Create internal/receiver/receiver.go with the Receiver interface:
   type Receiver interface {
       Start(ctx context.Context, out chan<- any) error
       Stop() error
       Name() string
   }

3. Create internal/receiver/otlp.go:
   - Start a gRPC server on the address from config (default :4317)
   - Start an HTTP server on the address from config (default :4318)
   - Implement OTLP trace/metrics/logs consumers using go.opentelemetry.io/collector/pdata
   - Normalise pdata.Span → model.SpanEvent
   - Normalise pdata.NumberDataPoint → model.MetricEvent (Gauge or Counter by kind)
   - Normalise pdata.LogRecord → model.LogEvent
   - Send normalised events to out chan<- any
   - Handle graceful shutdown on context cancel

4. Write a basic test in internal/receiver/otlp_test.go that starts the receiver and asserts it starts without error
```

---

### Prompt — Phase 3

```
Implement the time-series store and event bus for taplytix.

1. internal/store/ring.go
   - Generic Ring[T any] with fixed capacity
   - Push(v T), Slice() []T, Len() int, Cap() int
   - Thread-safe with sync.RWMutex

2. internal/store/percentile.go
   - Percentile(values []float64, p float64) float64
   - Works on a copy (never mutates input)
   - Returns 0 if values is empty

3. internal/store/timeseries.go
   - Series struct with Ring[float64] (capacity 300 = 5min at 1s)
   - Push(e model.MetricEvent)
   - P50(), P95(), P99() float64
   - Last() float64
   - Sparkline(n int) []float64

4. internal/store/tracemap.go
   - TraceMap with map[traceID]*Trace
   - Add(span model.SpanEvent) — assembles spans into trees
   - Get(traceID string) (*Trace, bool)
   - Recent(n int) []*model.Trace — last n by start time
   - Evict spans older than ttl duration

5. internal/store/store.go
   - Store struct as unified access point
   - Namespaced by service name
   - PushMetric, PushSpan, PushLog methods
   - MetricsFor(service), TracesFor(service), LogsFor(service), Services()

6. internal/bus/bus.go
   - Bus struct with []chan any subscribers
   - Publish(event any) — non-blocking, drops if buffer full
   - Subscribe() <-chan any — returns a buffered channel (1000 cap)
   - Dispatch(msg tea.Msg) — for Bubble Tea integration

7. Basic tests: one test each for ring eviction, percentile P99, and trace assembly
```

---

### Prompt — Phase 4

```
Implement the Bubble Tea TUI shell for taplytix.

1. internal/render/theme.go
   Define Lip Gloss styles for: tab active/inactive, card borders, 
   colors (primary blue, success green, warning amber, danger red, muted gray),
   status bar style, panel border style.

2. internal/tui/panels/panel.go
   Panel interface: tea.Model + SetSize(w, h int) + Title() string

3. internal/tui/keymap.go
   KeyMap struct with bindings for:
   Tab (next tab), Shift+Tab (prev tab), / (filter), 
   e (export), ? (help), q/ctrl+c (quit)
   Use charmbracelet/bubbles/key

4. internal/tui/statusbar.go
   StatusBar tea.Model:
   - Left: active alerts (red if any, green if none)
   - Center: events/s counter, spans in flight
   - Right: uptime, connection status (● green/red)

5. internal/tui/app.go
   AppModel tea.Model:
   - Holds panels []Panel and activeTab int
   - Holds *store.Store and *bus.Bus
   - On tea.WindowSizeMsg: call SetSize on all panels
   - On tea.KeyMsg: Tab/Shift+Tab switches panels; other keys delegate to active panel
   - On tickMsg (every config.RefreshMs): read store, build update messages for panels
   - View(): render tab bar + active panel content + status bar
   - Tab bar: show all panel titles, highlight active

6. Wire taplytix start to:
   - Load config
   - Create store and bus
   - Instantiate 5 placeholder panels (just show their title + "coming soon")
   - Launch tea.NewProgram(app, tea.WithAltScreen())

Must compile and run. Tab switching must work. q must quit cleanly.
```

---

### Prompt — Phase 5

```
Implement the Overview panel for taplytix.

1. internal/render/sparkline.go
   Sparkline(values []float64, width int, color lipgloss.Color) string
   - Use unicode blocks: ▁▂▃▄▅▆▇█
   - Normalise values to 0-7 range
   - Return Lip Gloss-coloured string of exactly `width` characters
   - Handle empty/nil values gracefully (return spaces)

2. internal/tui/panels/overview.go
   OverviewPanel implementing Panel interface:

   Metric cards (4 across, each shows):
   - Label (e.g. "P99 latency")
   - Current value with unit (e.g. "842ms")
   - Sparkline of last 40 samples
   - Card border color: red if value exceeds alert threshold, green otherwise

   Slowest operations table (below cards):
   - Use bubbles/table
   - Columns: Operation | P50 | P95 | P99 | Req/s | Errors
   - Populated from store.TracesFor(activeService)
   - Sort by P95 descending
   - Top 10 rows only

   Update(msg tea.Msg):
   - Handle tickMsg: re-read store, rebuild table rows and sparkline data
   - Handle tea.WindowSizeMsg: recompute card widths

   View(): Lip Gloss layout — cards row + table below
   
   Wire OverviewPanel to the app's panel list.
```

---

### Prompt — Phase 6

```
Implement the Traces panel for taplytix.

1. internal/render/waterfall.go
   WaterfallRow(span *model.SpanEvent, traceStart time.Time, totalDuration time.Duration, indent int, availWidth int) string
   - indent: spaces (2 per level) + tree chars (├ └ │)
   - bar: unicode blocks ▏▎▍▌▋▊▉█ for sub-char precision
   - bar offset: proportional to (span.StartTime - traceStart) / totalDuration
   - bar width: proportional to span.Duration / totalDuration
   - color: green=OK, red=Error, amber if Duration > 200ms
   - right-aligned duration label: "  142ms"

2. internal/tui/panels/traces.go
   TracesPanel implementing Panel interface:

   Top section (30% height): recent traces list
   - Each row: service · root span name · total duration · span count · status icon
   - Use bubbles/list
   - ↑↓ to navigate

   Bottom section (70% height): waterfall of selected trace
   - Use bubbles/viewport for scrolling
   - Render each span as a WaterfallRow, respecting parent-child indentation
   - Highlight slowest span with danger color background
   - Show trace summary: total duration, span count, error count

   Keys: ↑↓ navigate list, Enter expand/collapse, Esc back to list, / filter by service

   Wire TracesPanel to the app.
```

---

### Prompt — Phase 7

```
Implement the Metrics panel for taplytix.

1. internal/render/histogram.go
   Histogram(buckets []Bucket, maxWidth int) string
   type Bucket struct { Label string; Count int }
   - Horizontal bars: label right-padded + █ chars + count
   - Max bar = maxWidth chars, others proportional

2. internal/render/barchart.go  
   HBar(label string, value, maxValue float64, barWidth int, color lipgloss.Color) string
   - Returns: "label  ████░░░  42.0ms"
   - Filled portion: █, empty: ░

3. internal/tui/panels/metrics.go
   MetricsPanel implementing Panel interface:

   Left column (metric list, 35% width):
   - Searchable list of all metric names from store
   - bubbles/list with / filter via bubbles/textinput
   - Each item: metric name + current value + unit

   Right column (metric detail, 65% width):
   On metric selected, show:
   - Current value (large)
   - Window table: P50 | P95 | P99 for 1min / 5min / 15min
   - Rate of change (per second)
   - Histogram of value distribution (10 buckets)
   - Sparkline over last 5 minutes

   Wire MetricsPanel to the app.
```

---

### Prompt — Phase 8

```
Implement the Logs panel and log ingestion for taplytix.

1. internal/receiver/logtail.go
   LogTailReceiver implementing Receiver:
   - File mode: tail a file from current end, detect rotation
   - Stdin mode: read lines from os.Stdin
   - TCP mode: listen on address, read line-delimited streams
   - Format detection and parsing:
     * JSON: unmarshal, extract "level"/"severity", "msg"/"message", "time"/"timestamp"
     * logfmt: parse key=value pairs
     * plain: entire line as Body, level detected by prefix [INFO]/[ERROR] etc.
   - Publish model.LogEvent to out channel

2. internal/tui/panels/logs.go
   LogsPanel implementing Panel interface:

   Filter bar (top, shown when / pressed):
   - bubbles/textinput
   - Filters by: body substring, level prefix (level:error), service (service:api)
   - Esc clears filter

   Log viewport (main area):
   - bubbles/viewport
   - Render each LogEvent as: "timestamp  [LEVEL]  body  key=val..."
   - Colors: DEBUG=muted, INFO=green, WARN=amber, ERROR=red+bg highlight
   - TraceID shown as dim suffix if present (clickable in future)
   - Auto-scroll to bottom on new logs
   - Detect manual scroll up → pause auto-scroll
   - G key → jump to bottom, resume auto-scroll

   Status line (bottom):
   - "Showing N/total lines  |  filter: <expr>  |  auto-scroll: on/off"

   Wire LogsPanel to the app.
```

---

### Prompt — Phase 9

```
Implement fallback receivers for taplytix (Prometheus, StatsD, OS sidecar).

1. internal/receiver/prometheus.go
   PrometheusReceiver implementing Receiver:
   - Poll endpoint every config interval (default 2s)
   - Parse Prometheus text format using prometheus/common/expfmt
   - Convert each metric family to []model.MetricEvent
   - Handle gauge, counter, histogram, summary types
   - Graceful retry with exponential backoff on connection error
   - Log warning (charmbracelet/log) on non-200 HTTP responses

2. internal/receiver/statsd.go
   StatsDReceiver implementing Receiver:
   - UDP listener on configured address (default :8125)
   - Parse datagrams: "name:value|type" and "name:value|type|@sample_rate|#tag:val"
   - Types: c (counter), g (gauge), ms (timer/histogram), s (set)
   - Convert to model.MetricEvent (timer → Histogram kind)
   - Handle multi-metric datagrams (newline separated)

3. internal/receiver/os.go
   OSSidecarReceiver implementing Receiver:
   - Poll every 2 seconds
   - Linux: parse /proc/<pid>/stat (CPU jiffies), /proc/<pid>/status (VmRSS)
   - macOS: exec "ps -o pcpu,rss -p <pid>" and parse output
   - Auto-discover PID: if config.PID == 0, find by config.Process name
   - Publish: process.cpu.percent, process.memory.rss as MetricEvent with source="os"
   - Also publish host-level: host.cpu.percent, host.memory.used from /proc/stat

4. Update cmd/taplytix/main.go to start all configured receivers concurrently
   based on [[source]] type in config.

5. Unit tests for Prometheus and StatsD parsers (no network needed — test parsing only).
```

---

### Prompt — Phase 10

```
Implement the alert engine for taplytix.

1. internal/alert/rule.go
   Rule, Op, Alert structs as defined in §5 of the implementation plan.
   Include: Validate() error method on Rule.

2. internal/alert/notifier/bell.go
   BellNotifier: write \a to os.Stderr. Name() returns "bell".

3. internal/alert/notifier/webhook.go
   WebhookNotifier: HTTP POST JSON payload with 5s timeout, 3 retries.
   Payload: {"alert": name, "value": v, "threshold": t, "fired_at": "RFC3339", "service": s}
   Name() returns "webhook".

4. internal/alert/notifier/logfile.go
   LogFileNotifier: append one line per alert to configured path.
   Name() returns "logfile".

5. internal/alert/engine.go
   Engine:
   - Holds []Rule, map[string]Notifier, *store.Store
   - Run(ctx context.Context) — goroutine that ticks every 5s
   - On each tick: evaluate all rules against store P-tile values
   - Track firing state: map[ruleName]firingStart time.Time
   - Fire alert only after rule.For duration exceeded
   - Send AlertFiredMsg / AlertResolvedMsg to Bubble Tea program
   - Call notifiers asynchronously on fire

6. Update tui/app.go:
   - Handle AlertFiredMsg: add to active alerts list
   - Handle AlertResolvedMsg: remove from active alerts list
   - Status bar: show count of active alerts in red, or "no alerts" in green

7. Load rules from config in main.go, wire engine to store and bus.
```

---

### Prompt — Phase 11

```
Implement multi-service support and the Services panel for taplytix.

1. Update internal/store/store.go:
   - Namespace all data by service name (already in model.SpanEvent.Service, model.LogEvent.Service)
   - Ensure MetricsFor, TracesFor, LogsFor all accept service name
   - Add Services() []string — returns all known service names, sorted
   - Add ServiceStatus(name string) ServiceStatus struct with:
     * LastSeen time.Time
     * EventsPerSecond float64
     * ErrorRate float64
     * Connected bool

2. internal/tui/panels/services.go
   ServicesPanel implementing Panel interface:

   Left column (service list):
   - Each item: status dot (● green/amber/red) + service name + events/s
   - Green: data received in last 5s
   - Amber: no data in 5-10s
   - Red: no data >10s or receiver error
   - ↑↓ navigate, Enter to select active service

   Right column (service detail):
   - Service name (large)
   - Connected since / last seen
   - P99 latency · heap · error rate (mini cards)
   - Top 3 slowest spans (mini waterfall, 3 rows max)

3. Add activeService string to AppModel
   Broadcast ServiceChangedMsg when user selects a service in ServicesPanel.

4. Update Overview, Traces, Metrics, Logs panels to:
   - Accept ServiceChangedMsg in Update()
   - Re-query store with new service name
   - Show service name in panel header

5. Wire ServicesPanel to the app panel list.
```

---

### Prompt — Phase 12

```
Implement the remote agent and SSH server mode for taplytix.

1. agent/main.go — taplytix-agent binary:
   - cobra CLI: "taplytix-agent --listen :7777 --cert agent.crt --key agent.key"
   - Start local OTLP receiver (reuse internal/receiver/otlp.go)
   - Start WebSocket server on --listen address
   - For each connected dashboard client:
     * Forward all received events as JSON over WebSocket
     * Handle client disconnect gracefully
   - Optional mTLS: load cert/key if provided

2. internal/receiver/remote.go
   RemoteReceiver implementing Receiver:
   - Connect to wss://<endpoint> WebSocket
   - Reconnect with exponential backoff (1s, 2s, 4s, max 30s)
   - Receive JSON event frames, decode to model.SpanEvent / MetricEvent / LogEvent
   - Publish to out channel
   - Update connection status (published as StatusMsg for Services panel)

3. Update internal/receiver/otlp.go or receiver.go:
   - Extract JSON serialisation for all three event types
   - Used by both agent (encode) and remote receiver (decode)

4. SSH server mode in cmd/taplytix/main.go:
   Add subcommand "serve":
   - taplytix serve --ssh :2222
   - Uses charmbracelet/wish to create SSH server
   - Each SSH session launches a new tea.Program(app) instance
   - Sessions share the same store (read-only from each session)
   - Auth: allow all in dev mode; public key list in prod mode (config)

5. README.md:
   Document the full workflow:
   - Local dev quickstart (30 seconds)
   - Adding OTel SDK for Node / Python / Go / Java
   - VPS setup with taplytix-agent
   - SSH remote access
```

---

## 9. Testing Strategy

Keep tests minimal — one short test file per package covering the one thing
most likely to break. No integration tests, no test infra needed.

| Package | One test to write |
|---|---|
| `store/ring` | Push N+1 items, assert oldest is evicted |
| `store/percentile` | `Percentile([]float64{1,2,3,4,5}, 99)` returns 5 |
| `store/tracemap` | Add two spans with same TraceID, assert tree assembled |
| `receiver/prometheus` | Parse a known Prometheus text snippet, assert metric name + value |
| `receiver/statsd` | Parse `"req.duration:42\|ms"`, assert MetricEvent.Value == 42 |
| `alert/engine` | Rule threshold exceeded → alert fired; condition clears → resolved |
| `config` | Load `taplytix.toml`, assert source name and refresh_ms round-trip |

```bash
# Run all tests
go test ./...
```

---

## 10. Done Checklist

One key check per phase — if this works, the phase is good.

| Phase | Check |
|---|---|
| 1 — Skeleton | `go build ./...` passes, `taplytix version` prints correctly |
| 2 — OTLP Receiver | Send a test span; it appears in the out channel |
| 3 — Store & Bus | `go test ./internal/store/...` passes |
| 4 — TUI Shell | TUI opens, Tab switches panels, q quits cleanly |
| 5 — Overview | Live sparklines and slowest ops table visible |
| 6 — Traces | Waterfall renders with correct bar widths and indentation |
| 7 — Metrics | Selecting a metric shows P50/P95/P99 and histogram |
| 8 — Logs | Level colours correct, `/filter` narrows results |
| 9 — Fallback receivers | `--prom http://localhost:9090/metrics` populates Overview |
| 10 — Alerts | P99 breach triggers terminal bell within `for` duration |
| 11 — Services | Two sources in config → both listed with status dots |
| 12 — Remote agent | `taplytix-agent` running → remote service visible in Services panel |

---

## 11. TUI Screen Examples

All screens share a common chrome:
- **Top bar** — project name · connected source · status · timestamp
- **Tab bar** — Overview / Traces / Metrics / Logs / Services
- **Status bar** — alert count · events/s · uptime · key hints

Unicode drawing uses standard box-drawing and block characters available in
any modern terminal font (JetBrains Mono, Fira Code, Cascadia Code, etc.).

---

### Screen 1 — Overview Panel

The default landing screen. Shows live vital-sign cards with sparklines,
plus a table of the slowest operations ranked by P95.

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix                    my-api · OTLP :4317 · ● connected      04/28 14:32 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ [ Overview ]  Traces  Metrics  Logs  Services                                   │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  ╭──────────────────────╮  ╭──────────────────────╮  ╭──────────────────────╮  │
│  │ P99 latency          │  │ P50 latency          │  │ Heap in use          │  │
│  │                      │  │                      │  │                      │  │
│  │  842ms               │  │   48ms               │  │  142 MB              │  │
│  │                      │  │                      │  │                      │  │
│  │ ▁▂▃▄▅▇█▇▆▅▆▇█▇▅▄▃▄▅ │  │ ▂▂▃▃▂▂▂▃▃▂▂▃▂▂▂▂▃▃▂▂ │  │ ▂▃▃▄▄▅▅▅▆▆▇▇▆▆▅▅▄▄▅▅ │  │
│  ╰──────────────────────╯  ╰──────────────────────╯  ╰──────────────────────╯  │
│                                                                                 │
│  ╭──────────────────────╮                                                       │
│  │ Active spans         │                                                       │
│  │                      │                                                       │
│  │   24                 │                                                       │
│  │                      │                                                       │
│  │ ▃▃▄▄▄▅▅▄▄▄▃▄▄▅▅▄▃▃▃▄ │                                                       │
│  ╰──────────────────────╯                                                       │
│                                                                                 │
│  Slowest operations (P95)                                                       │
│  ┌──────────────────────────┬────────┬────────┬────────┬────────┬──────────┐   │
│  │ Operation                │  P50   │  P95   │  P99   │ Req/s  │  Errors  │   │
│  ├──────────────────────────┼────────┼────────┼────────┼────────┼──────────┤   │
│  │ POST /api/search         │  48ms  │ 610ms  │ 842ms  │   4.2  │   0.8%   │   │
│  │ GET  /api/feed           │  42ms  │ 290ms  │ 320ms  │  12.1  │   0.0%   │   │
│  │ GET  /api/user/:id       │  18ms  │ 120ms  │ 210ms  │   8.7  │   0.2%   │   │
│  │ POST /api/auth/token     │  12ms  │  45ms  │  89ms  │   1.1  │   0.0%   │   │
│  │ GET  /health             │   2ms  │   4ms  │   6ms  │  60.0  │   0.0%   │   │
│  └──────────────────────────┴────────┴────────┴────────┴────────┴──────────┘   │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ⚠ 1 alert: P99 > 500ms (32s)     events/s: 142 · spans: 24     Tab/? q:quit   │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

**Colour mapping for implementation:**

| Element | Lip Gloss colour |
|---|---|
| P99 card border + value | `ColorDanger` `#F85149` — threshold exceeded |
| P50 card border + value | `ColorSuccess` `#3FB950` — healthy |
| Heap card border + value | `ColorWarning` `#E3B341` — elevated |
| Active spans value | `ColorPrimary` `#58A6FF` |
| Sparklines | Match their card's colour |
| Table header | `ColorMuted` `#8B949E` |
| Slowest row (POST /api/search) | `ColorDanger` foreground |
| Status bar alert | `ColorDanger` background pill |

---

### Screen 2 — Traces Panel

Split view: recent trace list (top) and span waterfall of the selected trace (bottom).
Waterfall bars are proportional to each span's fraction of total trace duration.

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix                    my-api · OTLP :4317 · ● connected      04/28 14:32 │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Overview  [ Traces ]  Metrics  Logs  Services                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Recent traces                                                                  │
│  ┌────────────────────────────────┬──────────┬────────┬────────────────────┐   │
│  │ Root span                      │ Service  │  Total │ Spans  Status      │   │
│  ├────────────────────────────────┼──────────┼────────┼────────────────────┤   │
│  │ ▶ POST /api/search             │ my-api   │ 842ms  │   4    ✗ error     │   │
│  │   GET  /api/feed               │ my-api   │ 320ms  │   3    ✓ ok        │   │
│  │   GET  /api/user/42            │ my-api   │  98ms  │   2    ✓ ok        │   │
│  │   POST /api/auth/token         │ my-api   │  44ms  │   2    ✓ ok        │   │
│  └────────────────────────────────┴──────────┴────────┴────────────────────┘   │
│                                                                                 │
│  ─── Trace: POST /api/search · 842ms · 4 spans · 1 error ───────────────────   │
│                                                                                 │
│  Span                          0ms      200ms     400ms     600ms     800ms    │
│  ──────────────────────────────┼─────────┼─────────┼─────────┼─────────┼───   │
│                                                                                 │
│  POST /api/search              ████████████████████████████████████████  842ms │
│  ├─ auth.verify                ██  18ms                                        │
│  ├─ cache.get                  ███  42ms                                       │
│  └─ db.query                         ████████████████████████████████  610ms  │
│     └─ db.connect                    ██  28ms                                  │
│                                                                                 │
│  Slowest span: db.query (610ms · 72% of trace)                                 │
│  Error: db.query → "connection pool exhausted (pool=20/20)"                    │
│                                                                                 │
│  Attributes                                                                     │
│  db.system=postgresql   db.name=myapp   db.statement=SELECT * FROM posts...    │
│  net.peer.name=localhost   net.peer.port=5432                                  │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ↑↓ select trace   Enter expand   Esc back   / filter          Tab/? q:quit     │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

**Waterfall bar rendering logic for implementation:**

```
total_width  = terminal_width - label_col_width - duration_label_width
bar_offset   = (span.StartTime - trace.StartTime) / trace.Duration * total_width
bar_width    = span.Duration / trace.Duration * total_width  (min 1 char)

chars        = ▏ ▎ ▍ ▌ ▋ ▊ ▉ █   (1/8 increments for fractional precision)
colour       = ColorSuccess  if span.Status == OK   && span.Duration < 200ms
             = ColorWarning  if span.Status == OK   && span.Duration >= 200ms
             = ColorDanger   if span.Status == Error
```

---

### Screen 3 — Metrics Panel

Left: searchable metric list. Right: full detail view for the selected metric —
window table, histogram, and sparkline.

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix                    my-api · OTLP :4317 · ● connected      04/28 14:32 │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Overview  Traces  [ Metrics ]  Logs  Services                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  / filter metrics...          │  http.server.duration                           │
│  ─────────────────────────── │  ─────────────────────────────────────────────  │
│  http.server.duration  842ms  │                                                 │
│  http.server.requests   14/s  │  Current P99      842 ms                        │
│  process.cpu.percent   23.4%  │  Rate of change   +12 ms/s                      │
│  process.mem.heap_use  142MB  │                                                 │
│  process.mem.heap_sys  210MB  │  Window      P50      P95      P99              │
│  process.goroutines      312  │  ─────────────────────────────────────          │
│  process.gc.pause      1.2ms  │  1 min        48ms    610ms    842ms            │
│  db.query.duration     610ms  │  5 min        42ms    420ms    720ms            │
│  db.pool.active           18  │  15 min       38ms    310ms    580ms            │
│  cache.hit_rate          84%  │                                                 │
│  runtime.gc.count         42  │  Distribution (last 5 min)                      │
│  runtime.gc.pause       1.2s  │  0–50ms    ████████████████████████  68%        │
│                               │  50–200ms  ████████  22%                        │
│                               │  200–500ms ████  6%                             │
│                               │  500ms+    ██  4%                               │
│                               │                                                 │
│                               │  Trend (last 5 min)                             │
│                               │  ▂▂▃▃▃▄▄▄▄▅▅▆▆▇▇█▇▇▆▅▅▆▇█▇▆▅▄▃▃▄▅▆▇█▇▆▅▄▃     │
│                               │  0ms ─────────────────────────────── 1200ms    │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ↑↓ select metric   / filter   e export CSV               Tab/? q:quit          │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

**Histogram bar rendering logic for implementation:**

```go
// Each bucket row:
// "label  " + filled_blocks + empty_blocks + "  percentage%"
filledWidth = int(bucket.Fraction * maxBarWidth)
emptyWidth  = maxBarWidth - filledWidth
row = labelPadded + strings.Repeat("█", filledWidth) +
      muted(strings.Repeat("░", emptyWidth)) +
      fmt.Sprintf("  %.0f%%", bucket.Fraction*100)
```

---

### Screen 4 — Logs Panel

Full-width scrollable log tail. Active `/filter` bar at top.
ERROR lines get a dim red background highlight. Auto-scroll pauses when scrolling up.

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix                    my-api · OTLP :4317 · ● connected      04/28 14:32 │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Overview  Traces  Metrics  [ Logs ]  Services                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  / filter: error█                                          Showing 3/847 lines  │
│  ─────────────────────────────────────────────────────────────────────────────  │
│                                                                                 │
│  14:31:44  [ERROR]  db connection pool exhausted pool=20/20 service=db          │
│            trace_id=a3f9b2c1 span_id=d4e5f6a7                                  │
│                                                                                 │
│  14:31:52  [ERROR]  db connection pool exhausted pool=20/20 service=db          │
│            trace_id=b7c8d9e0 span_id=e5f6a7b8                                  │
│                                                                                 │
│  14:32:10  [ERROR]  db connection pool exhausted pool=20/20 service=db          │
│            trace_id=a3f9b2c1 span_id=d4e5f6a7                                  │
│                                                                                 │
│                                                                                 │
│                                                                                 │
│                                                                                 │
│                                                                                 │
│                                                                                 │
│                                                                                 │
│  ─────────────────────────────────────────────────────────────────────────────  │
│  ↑↓ PgUp PgDn scroll   G bottom   Esc clear filter   auto-scroll: paused       │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ⚠ 1 alert active     events/s: 142 · spans: 24 · uptime 00:14:22   Tab/? q    │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

**Unfiltered view (no active filter) — full log stream:**

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix                    my-api · OTLP :4317 · ● connected      04/28 14:32 │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Overview  Traces  Metrics  [ Logs ]  Services                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  / to filter                                              Showing 847 lines     │
│  ─────────────────────────────────────────────────────────────────────────────  │
│                                                                                 │
│  14:32:01  [INFO ]  server listening on :8080                                   │
│  14:32:03  [DEBUG]  incoming request method=POST path=/api/search               │
│  14:32:03  [INFO ]  cache miss key=search:golang:page1                          │
│  14:32:03  [WARN ]  slow query detected query=SELECT duration=610ms             │
│                     trace_id=a3f9b2c1                                           │
│  14:32:03  [ERROR]  db connection pool exhausted pool=20/20 service=db          │
│                     trace_id=a3f9b2c1 span_id=d4e5f6a7                         │
│  14:32:04  [INFO ]  GC pause 1.2ms heap_reclaimed=18MB                          │
│  14:32:05  [INFO ]  request completed method=POST path=/api/search              │
│                     status=500 duration=842ms trace_id=a3f9b2c1                 │
│  14:32:06  [DEBUG]  incoming request method=GET path=/api/feed                  │
│  14:32:06  [INFO ]  cache hit key=feed:user:99 age=4s                           │
│  14:32:06  [INFO ]  request completed method=GET path=/api/feed                 │
│                     status=200 duration=320ms trace_id=b7c8d9e0                 │
│  14:32:11  [INFO ]  healthcheck ok uptime=14m22s                                │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ no alerts     events/s: 142 · spans: 24 · uptime 00:14:22   ↑↓ / G Tab/? q   │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

**Log level colour mapping for implementation:**

```go
func levelStyle(level model.LogLevel) lipgloss.Style {
    switch level {
    case model.LevelDebug:
        return lipgloss.NewStyle().Foreground(ColorMuted)
    case model.LevelInfo:
        return lipgloss.NewStyle().Foreground(ColorSuccess)
    case model.LevelWarn:
        return lipgloss.NewStyle().Foreground(ColorWarning)
    case model.LevelError:
        return lipgloss.NewStyle().
            Foreground(ColorDanger).
            Background(lipgloss.Color("#2D1A1A"))
    }
    return lipgloss.NewStyle()
}
```

---

### Screen 5 — Services Panel

Left: service list with live status dots. Right: per-service mini-overview
including connection info, key vitals, and top 3 slowest spans.

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix                    all services · 3 connected     04/28 14:32          │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Overview  Traces  Metrics  Logs  [ Services ]                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Services               │  my-api                                               │
│  ─────────────────────  │  ──────────────────────────────────────────────────  │
│  ● my-api      142/s    │  Source      OTLP gRPC :4317                          │
│  ● worker      12/s     │  Connected   00:14:22 ago                             │
│  ● scheduler   3/s      │  Last event  0.2s ago                                 │
│  ○ prod-api             │                                                       │
│    (remote ·  config)   │  ╭──────────────╮  ╭──────────────╮  ╭────────────╮  │
│                         │  │ P99 latency  │  │ Heap         │  │ Error rate │  │
│                         │  │   842ms      │  │   142 MB     │  │   0.8%     │  │
│                         │  ╰──────────────╯  ╰──────────────╯  ╰────────────╯  │
│                         │                                                       │
│                         │  Top 3 slowest spans (P95)                            │
│                         │  ────────────────────────────────────────────────    │
│                         │  db.query          ████████████████████  610ms        │
│                         │  GET /api/feed     ████████████  290ms                │
│                         │  GET /api/user/:id ██████  120ms                      │
│                         │                                                       │
│                         │  Goroutines  312      GC pauses  1.2ms avg            │
│                         │  Req/s        14      Uptime     14m 22s              │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ↑↓ select service   Enter switch active   / filter        Tab/? q:quit         │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

**Service status dot logic for implementation:**

```go
func statusDot(s store.ServiceStatus) string {
    switch {
    case !s.Connected:
        return ColorMuted.Render("○")   // not configured / disconnected
    case time.Since(s.LastSeen) < 5*time.Second:
        return ColorSuccess.Render("●") // actively receiving data
    case time.Since(s.LastSeen) < 10*time.Second:
        return ColorWarning.Render("●") // stale — no data recently
    default:
        return ColorDanger.Render("●")  // dead — no data > 10s
    }
}
```

---

### Screen 6 — Alert Firing State

Any panel can show an alert banner. Here the Overview panel shows an active alert
— the P99 card border turns red and a banner appears above the status bar.

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix                    my-api · OTLP :4317 · ● connected      04/28 14:35 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ [ Overview ]  Traces  Metrics  Logs  Services                                   │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  ╭──────────────────────╮  ╭──────────────────────╮  ╭──────────────────────╮  │
│  │ P99 latency    ⚠ !!  │  │ P50 latency          │  │ Heap in use          │  │
│  │                      │  │                      │  │                      │  │
│  │  1240ms              │  │   52ms               │  │  148 MB              │  │
│  │                      │  │                      │  │                      │  │
│  │ ▃▄▅▆▇█▇▇█▇▇██▇▇▇██▇ │  │ ▂▂▃▃▂▂▂▃▃▂▂▃▂▂▂▂▃▃▂▂ │  │ ▃▄▄▅▅▅▆▆▇▇▇▇▆▆▅▅▄▄▅▆ │  │
│  ╰──────────────────────╯  ╰──────────────────────╯  ╰──────────────────────╯  │
│                                                                                 │
│  ╭──────────────────────╮                                                       │
│  │ Active spans         │                                                       │
│  │   38                 │                                                       │
│  │ ▄▄▅▅▅▆▆▅▅▅▄▅▅▆▆▅▄▄▄▅ │                                                       │
│  ╰──────────────────────╯                                                       │
│                                                                                 │
│  Slowest operations (P95)                                                       │
│  ┌──────────────────────────┬────────┬─────────┬─────────┬────────┬─────────┐  │
│  │ POST /api/search         │  52ms  │  890ms  │ 1240ms  │   6.8  │   3.2%  │  │
│  │ GET  /api/feed           │  45ms  │  340ms  │  420ms  │  11.4  │   0.0%  │  │
│  └──────────────────────────┴────────┴─────────┴─────────┴────────┴─────────┘  │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ╔══ ALERT: high-p99 — P99 1240ms > 500ms threshold (firing 3m 12s) ══╗         │
│ ╚═════════════════════════════════════════════════════════════════════╝         │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ⚠ 1 alert active     events/s: 218 · spans: 38 · uptime 00:17:34   Tab/? q   │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

---

### Screen 7 — First-run Setup Wizard (Huh)

Shown on `taplytix init`. Built with `charmbracelet/huh`.
Writes `taplytix.toml` on completion.

```
╭─────────────────────────────────────────────────────────────────────────────────╮
│ taplytix · first-run setup                                                      │
╰─────────────────────────────────────────────────────────────────────────────────╯

  Welcome to Taplytix!
  Let's get you set up in under a minute.

  ┌─ Step 1 of 3: Source protocol ──────────────────────────────────────────────┐
  │                                                                              │
  │  How does your app export telemetry?                                         │
  │                                                                              │
  │  > ● OTLP (recommended — traces + metrics + logs)                           │
  │    ○ Prometheus  (/metrics endpoint)                                         │
  │    ○ StatsD      (UDP push)                                                  │
  │    ○ Log file    (stdout / file tail)                                        │
  │    ○ OS only     (no app instrumentation needed)                             │
  │                                                                              │
  └──────────────────────────────────────────────────────────────────────────────┘

  ┌─ Step 2 of 3: Endpoint ─────────────────────────────────────────────────────┐
  │                                                                              │
  │  gRPC port (default :4317)                                                   │
  │  > :4317                                                                     │
  │                                                                              │
  │  HTTP port (default :4318)                                                   │
  │  > :4318                                                                     │
  │                                                                              │
  └──────────────────────────────────────────────────────────────────────────────┘

  ┌─ Step 3 of 3: Alerts ───────────────────────────────────────────────────────┐
  │                                                                              │
  │  Enable default alerts?                                                      │
  │  > ● Yes — alert when P99 > 500ms or heap > 500MB                           │
  │    ○ No  — I'll configure alerts manually                                    │
  │                                                                              │
  └──────────────────────────────────────────────────────────────────────────────┘

  ↑↓ navigate   Space select   Enter confirm   Ctrl+C cancel

  ✓ Config written to ./taplytix.toml
  Run `taplytix start` to launch the dashboard.
```

---

### Screen 8 — `taplytix --help` (CLI)

```
taplytix — terminal-native developer monitoring dashboard

Usage:
  taplytix [command]

Commands:
  start       Launch the TUI dashboard (default)
  init        Interactive first-run setup wizard
  serve       Start SSH server for remote TUI access
  version     Print version information

Flags for `start`:
  --config    string   Path to config file (default: ./taplytix.toml)
  --otlp      string   OTLP gRPC listen address (default: :4317)
  --prom      string   Prometheus scrape endpoint (e.g. http://localhost:9090/metrics)
  --logs      string   Log file to tail (e.g. ./logs/app.log)
  --pid       int      OS sidecar: PID to monitor
  --process   string   OS sidecar: process name to auto-discover

Flags for `serve`:
  --ssh       string   SSH listen address (default: :2222)
  --key       string   Path to SSH host key

Examples:
  # Quickstart — receive OTLP on default ports
  taplytix start

  # Watch a Prometheus app
  taplytix start --prom http://localhost:9090/metrics

  # Tail a log file + OS metrics for PID 1234
  taplytix start --logs ./app.log --pid 1234

  # Interactive setup wizard
  taplytix init

  # Serve dashboard over SSH on port 2222
  taplytix serve --ssh :2222
```

---

### TUI Layout Constants for Implementation

These values should live in `internal/render/theme.go` and be used by all panels:

```go
package render

const (
    // Minimum terminal dimensions Taplytix requires
    MinWidth  = 100
    MinHeight = 30

    // Tab bar
    TabBarHeight = 1

    // Status bar
    StatusBarHeight = 1

    // Alert banner (shown only when alerts are active)
    AlertBannerHeight = 2

    // Overview panel
    VitalCardMinWidth = 22
    VitalCardHeight   = 7
    SparklineWidth    = 20

    // Traces panel
    TraceListHeightFraction  = 0.30
    WaterfallMinLabelWidth   = 28

    // Metrics panel
    MetricListWidthFraction  = 0.35
    HistogramMaxBarWidth     = 30

    // Logs panel
    LogTimestampWidth = 8   // "HH:MM:SS"
    LogLevelWidth     = 7   // "[ERROR]"

    // Services panel
    ServiceListWidthFraction = 0.30
)

// AvailableHeight returns the rows available for panel content
// after subtracting chrome (top bar + tab bar + status bar).
func AvailableHeight(termHeight int, alertsActive bool) int {
    chrome := 1 + TabBarHeight + StatusBarHeight
    if alertsActive {
        chrome += AlertBannerHeight
    }
    return termHeight - chrome
}
```

---

*Taplytix — see every trace, every metric, every log. Right in your terminal.*
