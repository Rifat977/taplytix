package receiver

import (
	"context"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rifat977/taplytix/internal/model"
)

func TestOTLPReceiverStartsAndStops(t *testing.T) {
	r := NewOTLP("test", freePort(t), freePort(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan any, 16)
	if err := r.Start(ctx, out); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestOTLPReceiverEndToEnd(t *testing.T) {
	grpcAddr := freePort(t)
	r := NewOTLP("test", grpcAddr, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan any, 32)
	if err := r.Start(ctx, out); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop() })

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Push one span, one metric, one log.
	ts := time.Now()

	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "demo")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op")
	span.SetTraceID([16]byte{0x01})
	span.SetSpanID([8]byte{0x02})
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(ts))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(ts.Add(50 * time.Millisecond)))
	span.Status().SetCode(ptrace.StatusCodeOk)

	if _, err := ptraceotlp.NewGRPCClient(conn).Export(ctx, ptraceotlp.NewExportRequestFromTraces(traces)); err != nil {
		t.Fatalf("export traces: %v", err)
	}

	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "demo")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("requests")
	sum := m.SetEmptySum()
	dp := sum.DataPoints().AppendEmpty()
	dp.SetIntValue(7)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(ts))

	if _, err := pmetricotlp.NewGRPCClient(conn).Export(ctx, pmetricotlp.NewExportRequestFromMetrics(metrics)); err != nil {
		t.Fatalf("export metrics: %v", err)
	}

	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "demo")
	rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.Body().SetStr("hello")
	rec.SetSeverityText("INFO")
	rec.SetTimestamp(pcommon.NewTimestampFromTime(ts))

	if _, err := plogotlp.NewGRPCClient(conn).Export(ctx, plogotlp.NewExportRequestFromLogs(logs)); err != nil {
		t.Fatalf("export logs: %v", err)
	}

	got := drain(out, 3, 2*time.Second)
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d: %#v", len(got), got)
	}

	var sawSpan, sawMetric, sawLog bool
	for _, ev := range got {
		switch e := ev.(type) {
		case model.SpanEvent:
			sawSpan = true
			if e.Service != "demo" || e.Name != "op" || e.Status != model.StatusOK {
				t.Errorf("span event: %+v", e)
			}
			if e.Duration != 50*time.Millisecond {
				t.Errorf("span duration = %v, want 50ms", e.Duration)
			}
		case model.MetricEvent:
			sawMetric = true
			if e.Name != "requests" || e.Value != 7 || e.Kind != model.Counter || e.Source != "demo" {
				t.Errorf("metric event: %+v", e)
			}
		case model.LogEvent:
			sawLog = true
			if e.Body != "hello" || e.Service != "demo" || e.Level != model.LevelInfo {
				t.Errorf("log event: %+v", e)
			}
		default:
			t.Errorf("unexpected event type %T: %v", ev, ev)
		}
	}
	if !sawSpan || !sawMetric || !sawLog {
		t.Errorf("missing event(s): span=%v metric=%v log=%v", sawSpan, sawMetric, sawLog)
	}
}

func drain(ch <-chan any, n int, timeout time.Duration) []any {
	out := make([]any, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

func freePort(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	return addr
}
