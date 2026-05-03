# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

This repo currently contains **only a design document** at `docs/TAPLYTIX_IMPLEMENTATION_PLAN.md` (~1900 lines). No Go source, `go.mod`, or `Makefile` exists yet — all code in the plan is to be written. Treat that document as the spec for any implementation work.

When asked to implement something, locate the relevant phase (1–12) in the plan and follow its task list, acceptance criteria, and the data models / interfaces in §5–§7. The plan is intended to be executed phase-by-phase, each phase verified before the next begins.

## Project: Taplytix

Single-binary, terminal-native developer monitoring dashboard. Ingests OTLP/Prometheus/StatsD/log telemetry and renders traces, metrics, and logs in a Bubble Tea TUI. **Hard constraint:** no daemon, no database, no cloud endpoint, no Docker — must run fully offline as one static binary.

Module path: `github.com/rifat977/taplytix` · Go 1.22 · Charm ecosystem (bubbletea, lipgloss, bubbles, huh, log, wish) · `go.opentelemetry.io/collector/{pdata,receiver/otlpreceiver}` · `prometheus/common` · `BurntSushi/toml` · `spf13/cobra`.

## Commands (once Phase 1 lands)

```bash
go build ./...                       # compile everything
go test ./...                        # run all tests
go test ./internal/store/... -v      # single package
go test ./internal/store -run TestRing  # single test
make build | test | run | lint       # Makefile wrappers (Phase 1 deliverable)
```

Per-phase smoke checks live in §10 of the plan (e.g. Phase 2: send a test span, see it on the out channel; Phase 4: TUI opens, Tab switches panels, q quits).

## Architecture (big picture)

Pipeline is **Receiver → Bus → Store → TUI**, with an Alert engine subscribed to the bus in parallel:

- **`internal/receiver/`** — pluggable input sources behind a `Receiver` interface: `otlp.go` (gRPC :4317 + HTTP :4318, primary), plus fallbacks `prometheus.go` (HTTP scrape), `statsd.go` (UDP :8125), `logtail.go` (file/stdin/TCP), `os.go` (/proc, ps sidecar). Each receiver normalizes input into the shared event types.
- **`internal/model/`** — canonical event types (`MetricEvent`, `SpanEvent`, `LogEvent`) that flow through the rest of the system. All receivers convert to these; nothing downstream knows about OTLP/Prom/StatsD specifics.
- **`internal/bus/`** — `MetricsBus` pub/sub fan-out. Receivers publish; Store and Alert engine subscribe.
- **`internal/store/`** — in-memory state: generic `ring.go` ring buffer, `timeseries.go` windowed aggregates, `percentile.go` (P50/P95/P99 sliding window), `tracemap.go` (assembles spans by TraceID into trees). `store.go` is the single read API the TUI uses.
- **`internal/alert/`** — rule engine ticks against the store; notifiers (`bell`, `webhook`, `logfile`) implement a `Notifier` interface.
- **`internal/tui/`** — root `app.go` is the Bubble Tea `tea.Model` doing tab routing across `panels/` (overview, traces, metrics, logs, services). Each panel implements a shared `Panel` interface and pulls from the Store on tick.
- **`internal/render/`** — pure string-builders (sparkline, barchart, histogram, waterfall) using Lip Gloss. Keep these free of TUI state so they're unit-testable.
- **`cmd/taplytix/main.go`** — Cobra root with subcommands `start` (default), `init` (Huh wizard), `version`.
- **`agent/main.go`** — separate sidecar binary for Phase 12 remote/SSH mode (Wish).

Config is TOML (`taplytix.toml`), schema in §7 of the plan; loaded via `internal/config/config.go` with `Load()` and `Default()`.

## Conventions specific to this project

- **No daemons or external state.** Reject any dependency that requires a running database, broker, or cloud account. Everything is in-memory ring buffers.
- **Receivers must not block.** They feed the bus via channels; backpressure is handled by dropping (ring buffer semantics), not blocking ingestion.
- **TUI never reads receivers directly** — always via Store. Renderers never read Store directly — they take plain data.
- **Unicode drawing** uses standard box/block chars compatible with JetBrains Mono / Fira Code / Cascadia Code.
- Phase ordering matters: store + bus (Phase 3) must exist before any panel work; the OTLP receiver (Phase 2) is the reference receiver — fallback receivers (Phase 9) follow its shape.
