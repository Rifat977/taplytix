# taplytix

> Terminal-native developer monitoring dashboard. Single binary, no daemon, no database, no cloud.

Ingests OpenTelemetry, Prometheus, StatsD, plain log files, and process-level
host stats; renders traces, metrics, and logs in a Bubble Tea TUI. Optional
sidecar agent forwards telemetry from a remote VPS over WebSocket. Dashboard
itself can be served over SSH.

## Table of Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [First Run](#first-run)
- [Configuration](#configuration)
- [Sending Telemetry](#sending-telemetry)
  - [OpenTelemetry SDKs](#opentelemetry-sdks)
  - [Prometheus Endpoint](#prometheus-endpoint)
  - [StatsD](#statsd)
  - [Tailing Log Files](#tailing-log-files)
  - [Process / Host Stats](#process--host-stats)
- [Alerts](#alerts)
- [Remote VPS Setup](#remote-vps-setup)
- [SSH Mode](#ssh-mode)
- [Keybindings](#keybindings)
- [CLI Reference](#cli-reference)
- [Architecture](#architecture)
- [Development](#development)

## Requirements

- **Go 1.22 or newer** — required to build from source.
- **A modern terminal** — JetBrains Mono, Fira Code, Cascadia Code, or anything
  with Unicode block-drawing support.
- **No runtime dependencies.** No databases, no Docker, no daemons. The binary
  is fully self-contained and works offline.

Verify your toolchain:

```sh
go version    # must print 1.22+
```

## Installation

### Option 1 — clone and build (recommended)

```sh
git clone <this-repo> taplytix
cd taplytix
make build           # produces ./bin/taplytix
# Optionally also build the remote sidecar:
go build -o bin/taplytix-agent ./agent
```

### Option 2 — `go install`

```sh
go install github.com/rifat977/taplytix/cmd/taplytix@latest
go install github.com/rifat977/taplytix/agent@latest    # optional sidecar
```

The binaries land in `$(go env GOPATH)/bin`. Make sure that directory is on
your `$PATH`.

### Option 3 — install to /usr/local/bin

```sh
make build
sudo install -m 0755 bin/taplytix /usr/local/bin/taplytix
sudo install -m 0755 bin/taplytix-agent /usr/local/bin/taplytix-agent  # optional
```

Verify the install:

```sh
taplytix version          # → taplytix v0.1.0
taplytix-agent version    # → taplytix-agent v0.1.0
```

## First Run

The fastest way to see something on screen:

```sh
taplytix start
```

With no config file present, taplytix uses sensible defaults: an OTLP
receiver on `:4317` (gRPC) and `:4318` (HTTP), a 500 ms refresh tick, and
the default service name `my-app`. The TUI opens immediately; tabs are
empty until telemetry arrives.

Now in a second terminal, send some data — see [Sending
Telemetry](#sending-telemetry) below for a copy-pasteable example.

## Configuration

Configuration is a TOML file. By default taplytix looks for `./taplytix.toml`
in the current working directory; override with `--config /path/to/file.toml`.

A working starter config (also shipped as `taplytix.toml` in this repo):

```toml
[server]
refresh_ms      = 500
log_file        = "~/.taplytix/debug.log"
default_service = "my-app"

# ── Sources ───────────────────────────────────────────
[[source]]
name = "my-api"
type = "otlp"
grpc = ":4317"
http = ":4318"

# Tail an application log file
[[source]]
name   = "app-logs"
type   = "logs"
path   = "/var/log/my-app/app.log"
format = "auto"           # "json" | "logfmt" | "plain" | "auto"

# Scrape a Prometheus endpoint every 2s
[[source]]
name     = "node-exporter"
type     = "prometheus"
endpoint = "http://localhost:9100/metrics"
interval = "2s"

# StatsD UDP listener
[[source]]
name   = "statsd"
type   = "statsd"
listen = ":8125"

# Process-level CPU/RSS for an external app (auto-discover by name)
[[source]]
name    = "host"
type    = "sysstat"
process = "my-api"
pid     = 0               # 0 = auto-discover by process name

# ── Alerts ────────────────────────────────────────────
[[alert]]
name       = "high-p99"
metric     = "http.server.duration"
percentile = 99
op         = ">"
threshold  = 500          # ms
for        = "30s"
notify     = ["bell", "logfile"]

[notifier.webhook]
url = "https://hooks.example.com/taplytix"

[notifier.logfile]
path = "~/.taplytix/alerts.log"
```

Every section is optional. Define only the sources you actually have. Many
flags also exist as command-line shortcuts so you can experiment without
editing the config file.

## Sending Telemetry

### OpenTelemetry SDKs

The default OTLP receiver listens on the standard ports, so any compliant SDK
works out of the box:

**Node.js**

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
OTEL_SERVICE_NAME=my-api \
node --require @opentelemetry/auto-instrumentations-node/register app.js
```

**Python**

```sh
pip install opentelemetry-distro opentelemetry-exporter-otlp
opentelemetry-bootstrap -a install
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
OTEL_SERVICE_NAME=my-api \
opentelemetry-instrument python app.py
```

**Go** (programmatic, gRPC):

```go
exp, _ := otlptracegrpc.New(ctx,
    otlptracegrpc.WithEndpoint("localhost:4317"),
    otlptracegrpc.WithInsecure(),
)
```

**Java agent**

```sh
java \
  -javaagent:opentelemetry-javaagent.jar \
  -Dotel.exporter.otlp.endpoint=http://localhost:4317 \
  -Dotel.service.name=my-api \
  -jar app.jar
```

### Prometheus Endpoint

Scrape any Prometheus text-format endpoint without editing the config:

```sh
taplytix start --prom http://localhost:9100/metrics
```

Or via config (`type = "prometheus"`, `endpoint = …`, `interval = "2s"`).

### StatsD

```sh
taplytix start --statsd :8125

# Send a counter:
echo -n 'demo.requests:1|c' | nc -u -w0 127.0.0.1 8125

# Send a timer with tags:
echo -n 'req.duration:42|ms|#path:/api,code:200' | nc -u -w0 127.0.0.1 8125
```

### Tailing Log Files

```sh
# Tail a file:
taplytix start --logs /var/log/my-app/app.log

# Read from stdin:
my-app 2>&1 | taplytix start --logs -
```

Format auto-detection: starts with `{` ⇒ JSON, contains `key=value` ⇒ logfmt,
otherwise plain text (with `[INFO]` / `[ERROR]` etc. prefix detection).

### Process / Host Stats

Linux uses `/proc/<pid>/stat` and `/proc/<pid>/status`; macOS shells out to
`ps`. Configure via `taplytix.toml`:

```toml
[[source]]
name    = "host"
type    = "sysstat"
process = "my-api"     # auto-discover PID by name
```

Emits `process.cpu.percent` and `process.memory.rss` every 2 s.

## Alerts

Define rules in `taplytix.toml` under `[[alert]]`. Each rule:

- evaluates one metric (raw value or a percentile of recent samples);
- fires only after `For` duration of continuous breach;
- auto-resolves when the condition clears;
- fans out to every notifier listed in `notify`.

Built-in notifiers:

| Name | What it does |
|---|---|
| `bell` | Writes ASCII BEL to stderr (terminal beeps) |
| `webhook` | POSTs JSON `{alert,value,threshold,fired_at,service}` to `[notifier.webhook].url` |
| `logfile` | Appends a line per fire/resolve to `[notifier.logfile].path` (defaults to `~/.taplytix/alerts.log`) |

The status bar shows `⚠ N alert(s)` in red while any alert is active.

## Remote VPS Setup

When the application runs on a remote box, run the agent there and forward
telemetry over WebSocket to your laptop:

**On the VPS:**

```sh
# Build (or scp the binary)
go build -o /usr/local/bin/taplytix-agent ./agent

# Run
taplytix-agent --listen :7777 \
  --otlp-grpc :4317 \
  --otlp-http :4318
```

For TLS (recommended for any non-LAN deployment):

```sh
taplytix-agent --listen :7777 --cert /etc/ssl/agent.crt --key /etc/ssl/agent.key
```

**On the laptop**, add a remote source to `taplytix.toml`:

```toml
[[source]]
name     = "prod-api"
type     = "remote"
endpoint = "ws://prod-host:7777"        # or "wss://prod-host:7777" with TLS
```

Then:

```sh
taplytix start
```

The Services tab shows `prod-api` once data is flowing. Reconnects with
exponential backoff (1 s → 30 s) if the connection drops.

You can also do an SSH tunnel instead of opening port 7777 to the world:

```sh
ssh -L 7777:localhost:7777 prod-host
# In the config: endpoint = "ws://localhost:7777"
```

## SSH Mode

Serve the dashboard itself over SSH so multiple developers can view live
telemetry without copying it off the host:

```sh
# Dev mode — allow any SSH key
taplytix serve --ssh :2222

# Prod mode — restrict to listed keys
taplytix serve --ssh :2222 \
  --authorized-keys /etc/taplytix/authorized_keys \
  --host-key /etc/taplytix/ssh_host_key
```

Connect from a laptop:

```sh
ssh -p 2222 dev@your-host
```

Each SSH session gets its own Bubble Tea program reading from the shared
store, so multiple viewers see the same live data without interfering with
each other. Sessions without a real PTY (e.g. `ssh host -p 2222 cmd`) are
politely refused.

## Keybindings

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Cycle panels (Overview · Traces · Metrics · Logs · Services) |
| `q` / `Ctrl+C` | Quit |
| `/` | Open filter input (Metrics, Logs panels) |
| `Esc` | Close filter / collapse expanded waterfall |
| `Enter` | Expand selected trace · select service · confirm filter |
| `↑` `↓` `k` `j` `PgUp` `PgDn` | Navigate lists, scroll viewport |
| `G` | Logs panel: jump to bottom and resume auto-scroll |
| `?` | Help (placeholder) |

Filter syntax in the Logs panel: tokens prefixed with `level:` and
`service:` filter exact matches; everything else is a body+attrs substring.
Example: `level:ERROR service:api timeout`.

## CLI Reference

### `taplytix start` — main dashboard

```sh
taplytix start [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--config, -c` | `taplytix.toml` | Path to TOML config |
| `--logs` | — | Tail a log file (or `-` for stdin) |
| `--prom` | — | Scrape this Prometheus endpoint |
| `--statsd` | — | Listen for StatsD UDP on this address |

CLI-flag sources are *added* to whatever the config file already defines.

### `taplytix serve` — SSH dashboard

```sh
taplytix serve [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--ssh` | `:2222` | SSH listen address |
| `--host-key` | random | Path to PEM-encoded host key |
| `--authorized-keys` | (allow all) | Path to OpenSSH `authorized_keys` file |

### `taplytix init` — interactive config wizard

Placeholder; runs an interactive setup wizard (work in progress).

### `taplytix-agent` — remote sidecar

```sh
taplytix-agent [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--listen` | `:7777` | WebSocket listen address |
| `--otlp-grpc` | `:4317` | OTLP gRPC listen address |
| `--otlp-http` | `:4318` | OTLP HTTP listen address |
| `--cert` | — | TLS certificate (enables `wss://`) |
| `--key` | — | TLS private key |

The agent runs no TUI; it just bridges OTLP → WebSocket. Logs to stderr.

## Architecture

```
                ┌─────────────────────┐
                │      Receivers      │   OTLP · Prometheus · StatsD
                │                     │   logtail · sysstat · remote
                └──────────┬──────────┘
                           │ events
                           ▼
                ┌─────────────────────┐
                │         Bus         │   non-blocking pub/sub
                └────┬────────────┬───┘
                     │            │
              ┌──────▼─────┐ ┌────▼──────────┐
              │   Store    │ │ Alert engine  │
              │ (in-mem    │ │ (5s tick)     │
              │  rings)    │ └──┬────────────┘
              └─────┬──────┘    │
                    │           ▼
                    │     Notifiers
                    │     bell · webhook · logfile
                    ▼
              ┌──────────┐
              │   TUI    │   Bubble Tea
              └──────────┘
```

- **Receivers** (`internal/receiver/`) normalise everything into the canonical
  `model.MetricEvent` / `SpanEvent` / `LogEvent` types.
- **Store** (`internal/store/`) namespaces by service, holds ring buffers,
  computes percentiles, assembles trace trees, tracks activity.
- **Alert engine** (`internal/alert/`) evaluates rules every 5 s; fires after
  `For` duration; auto-resolves; fans out to notifiers.
- **TUI** (`internal/tui/`) is the Bubble Tea root with five panels; receives
  alert messages via `bus.Dispatch`.
- **Agent** (`agent/` + `internal/agent/`) wraps a local OTLP receiver and a
  WebSocket broadcaster. The remote receiver on the dashboard side reconnects
  with exponential backoff.

No daemon, no database, no broker, no cloud. Everything is in-memory.

## Development

```sh
make build         # build ./bin/taplytix
make test          # run all tests
make run           # build + run start
make lint          # go vet + gofmt
go test ./internal/store -run TestRing -v   # single test

go build -o bin/taplytix-agent ./agent      # also build the agent
```

The repo is organised as:

```
cmd/taplytix/        # main TUI binary
agent/               # taplytix-agent sidecar binary
internal/
  receiver/          # OTLP, Prometheus, StatsD, logtail, sysstat, remote
  store/             # ring buffer, percentile, time-series, trace map, store
  bus/               # pub/sub
  alert/             # rule, engine
    notifier/        # bell, webhook, logfile
  tui/               # Bubble Tea app
    panels/          # overview, traces, metrics, logs, services
  render/            # sparkline, waterfall, histogram, barchart, theme
  agent/             # WebSocket broadcaster (used by agent/main.go)
  model/             # canonical event types + wire format
  config/            # TOML config loader
docs/                # design docs (TAPLYTIX_IMPLEMENTATION_PLAN.md)
```

For the design rationale and per-phase build plan, see
`docs/TAPLYTIX_IMPLEMENTATION_PLAN.md`.
