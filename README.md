# taplytix

> Terminal-native developer monitoring dashboard. Single binary, no daemon, no database, no cloud.

Ingests OpenTelemetry, Prometheus, StatsD, and plain log files; renders traces, metrics, and logs in a Bubble Tea TUI.

## Quick start (local dev)

```sh
go build -o bin/taplytix ./cmd/taplytix
./bin/taplytix start                                   # uses ./taplytix.toml or sane defaults
./bin/taplytix start --logs ./app.log                  # tail a file
./bin/taplytix start --prom http://localhost:9100/metrics
./bin/taplytix start --statsd :8125
```

The default config (or `--config taplytix.toml`) opens an OTLP receiver on
`:4317` (gRPC) and `:4318` (HTTP). Point any OpenTelemetry SDK at one of those
endpoints and traces, metrics, and logs flow into the TUI.

Tabs: `Tab` / `Shift+Tab` cycle through Overview · Traces · Metrics · Logs · Services. `q` quits.

## Wiring an app

Examples for the standard OTel exporters (defaults pick up `:4317` automatically):

| Language | Snippet |
|---|---|
| Node.js | `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 node app.js` (HTTP) |
| Python  | `opentelemetry-bootstrap -a install && OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 opentelemetry-instrument python app.py` |
| Go      | `otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint("localhost:4317"), otlptracegrpc.WithInsecure())` |
| Java    | `-Dotel.exporter.otlp.endpoint=http://localhost:4317 -javaagent:opentelemetry-javaagent.jar` |

For non-OTel apps, see `--prom` (scrape Prometheus text), `--statsd` (UDP datagrams), or the `taplytix.toml`
`[[source]]` entries (`type = "logs"`, `"sysstat"`, etc.).

## Remote (VPS) setup

`taplytix-agent` runs near the application and forwards events over WebSocket to a remote dashboard.

On the VPS:

```sh
go build -o bin/taplytix-agent ./agent
./bin/taplytix-agent --listen :7777
# Optional TLS:
./bin/taplytix-agent --listen :7777 --cert agent.crt --key agent.key
```

On the laptop, add a remote source to `taplytix.toml`:

```toml
[[source]]
name     = "prod-api"
type     = "remote"
endpoint = "ws://prod-host:7777"   # or wss:// with TLS
```

Then `./bin/taplytix start`. The Services panel will show `prod-api` once the agent has data flowing.

## SSH access

Serve the dashboard over SSH so anyone with a key can run the TUI without copying telemetry off-host:

```sh
./bin/taplytix serve --ssh :2222 --authorized-keys ~/.ssh/authorized_keys
ssh -p 2222 dev@your-host
```

If `--authorized-keys` is omitted, the server allows all connections (useful in dev). Each SSH session
runs its own `tea.Program` against the shared store/bus.

## Configuration

See the full schema in `taplytix.toml`. All fields are optional except where noted.

```toml
[server]
refresh_ms      = 500
default_service = "my-app"

[[source]]
name = "my-api"
type = "otlp"
grpc = ":4317"
http = ":4318"

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
```

## Architecture

```
Receivers → Bus → Store → TUI
              ↓
           Alert engine → Notifiers (bell · webhook · logfile)
```

- **Receivers** (`internal/receiver/`): OTLP gRPC+HTTP, Prometheus scrape, StatsD UDP, logtail (file/stdin/TCP), OS sidecar (Linux /proc, macOS ps), and remote (WebSocket from `taplytix-agent`).
- **Store** (`internal/store/`): in-memory ring buffers per service. Computes percentiles, assembles trace trees, tracks per-service activity.
- **TUI** (`internal/tui/`): Bubble Tea root with five panels; tick-driven refresh; alert messages via `bus.Dispatch`.
- **Alert engine** (`internal/alert/`): evaluates threshold rules every 5s; fires after `For` duration; auto-resolves when condition clears.

The binary is fully offline: no databases, no message brokers, no cloud endpoints.
