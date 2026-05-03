package receiver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"

	"github.com/rifat977/taplytix/internal/model"
)

const serviceNameAttr = "service.name"

// OTLPReceiver accepts OTLP traces, metrics, and logs over gRPC and HTTP and
// converts them into the unified model.* event types.
type OTLPReceiver struct {
	name     string
	grpcAddr string
	httpAddr string

	mu      sync.Mutex
	out     chan<- any
	grpcSrv *grpc.Server
	httpSrv *http.Server
	stopped chan struct{}
}

// NewOTLP returns a receiver that listens on the given gRPC and HTTP addresses.
// Either address may be empty to disable that transport.
func NewOTLP(name, grpcAddr, httpAddr string) *OTLPReceiver {
	return &OTLPReceiver{name: name, grpcAddr: grpcAddr, httpAddr: httpAddr}
}

func (r *OTLPReceiver) Name() string { return r.name }

func (r *OTLPReceiver) Start(ctx context.Context, out chan<- any) error {
	r.mu.Lock()
	if r.out != nil {
		r.mu.Unlock()
		return errors.New("otlp receiver: already started")
	}
	r.out = out
	r.stopped = make(chan struct{})
	r.mu.Unlock()

	if err := r.startGRPC(); err != nil {
		_ = r.Stop()
		return fmt.Errorf("otlp gRPC: %w", err)
	}
	if err := r.startHTTP(); err != nil {
		_ = r.Stop()
		return fmt.Errorf("otlp HTTP: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = r.Stop()
	}()

	return nil
}

func (r *OTLPReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.stopped:
		return nil
	default:
	}
	if r.stopped != nil {
		close(r.stopped)
	}
	if r.grpcSrv != nil {
		r.grpcSrv.GracefulStop()
		r.grpcSrv = nil
	}
	if r.httpSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = r.httpSrv.Shutdown(shutdownCtx)
		cancel()
		r.httpSrv = nil
	}
	r.out = nil
	return nil
}

func (r *OTLPReceiver) startGRPC() error {
	if r.grpcAddr == "" {
		return nil
	}
	lis, err := net.Listen("tcp", r.grpcAddr)
	if err != nil {
		return err
	}
	srv := grpc.NewServer()
	ptraceotlp.RegisterGRPCServer(srv, &grpcTraceServer{r: r})
	pmetricotlp.RegisterGRPCServer(srv, &grpcMetricServer{r: r})
	plogotlp.RegisterGRPCServer(srv, &grpcLogServer{r: r})
	r.mu.Lock()
	r.grpcSrv = srv
	r.mu.Unlock()
	go func() {
		_ = srv.Serve(lis)
	}()
	return nil
}

func (r *OTLPReceiver) startHTTP() error {
	if r.httpAddr == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.handleTraces)
	mux.HandleFunc("/v1/metrics", r.handleMetrics)
	mux.HandleFunc("/v1/logs", r.handleLogs)
	srv := &http.Server{Addr: r.httpAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	lis, err := net.Listen("tcp", r.httpAddr)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.httpSrv = srv
	r.mu.Unlock()
	go func() {
		_ = srv.Serve(lis)
	}()
	return nil
}

func (r *OTLPReceiver) emit(ev any) {
	r.mu.Lock()
	out := r.out
	r.mu.Unlock()
	if out == nil {
		return
	}
	// Non-blocking send: if the consumer is overwhelmed, drop. The pipeline
	// must never stall ingestion (see design note in CLAUDE.md).
	select {
	case out <- ev:
	default:
	}
}

// ── gRPC service adapters ──────────────────────────────────────────────────

type grpcTraceServer struct {
	ptraceotlp.UnimplementedGRPCServer
	r *OTLPReceiver
}

func (s *grpcTraceServer) Export(_ context.Context, req ptraceotlp.ExportRequest) (ptraceotlp.ExportResponse, error) {
	s.r.consumeTraces(req.Traces())
	return ptraceotlp.NewExportResponse(), nil
}

type grpcMetricServer struct {
	pmetricotlp.UnimplementedGRPCServer
	r *OTLPReceiver
}

func (s *grpcMetricServer) Export(_ context.Context, req pmetricotlp.ExportRequest) (pmetricotlp.ExportResponse, error) {
	s.r.consumeMetrics(req.Metrics())
	return pmetricotlp.NewExportResponse(), nil
}

type grpcLogServer struct {
	plogotlp.UnimplementedGRPCServer
	r *OTLPReceiver
}

func (s *grpcLogServer) Export(_ context.Context, req plogotlp.ExportRequest) (plogotlp.ExportResponse, error) {
	s.r.consumeLogs(req.Logs())
	return plogotlp.NewExportResponse(), nil
}

// ── HTTP handlers ──────────────────────────────────────────────────────────

func (r *OTLPReceiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	exp := ptraceotlp.NewExportRequest()
	if err := unmarshalRequest(req, body, exp.UnmarshalProto, exp.UnmarshalJSON); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.consumeTraces(exp.Traces())
	writeOTLPResponse(w, req, ptraceotlp.NewExportResponse().MarshalProto, ptraceotlp.NewExportResponse().MarshalJSON)
}

func (r *OTLPReceiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	exp := pmetricotlp.NewExportRequest()
	if err := unmarshalRequest(req, body, exp.UnmarshalProto, exp.UnmarshalJSON); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.consumeMetrics(exp.Metrics())
	writeOTLPResponse(w, req, pmetricotlp.NewExportResponse().MarshalProto, pmetricotlp.NewExportResponse().MarshalJSON)
}

func (r *OTLPReceiver) handleLogs(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	exp := plogotlp.NewExportRequest()
	if err := unmarshalRequest(req, body, exp.UnmarshalProto, exp.UnmarshalJSON); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.consumeLogs(exp.Logs())
	writeOTLPResponse(w, req, plogotlp.NewExportResponse().MarshalProto, plogotlp.NewExportResponse().MarshalJSON)
}

func unmarshalRequest(req *http.Request, body []byte, fromProto, fromJSON func([]byte) error) error {
	switch req.Header.Get("Content-Type") {
	case "application/json":
		return fromJSON(body)
	default:
		return fromProto(body)
	}
}

func writeOTLPResponse(w http.ResponseWriter, req *http.Request, marshalProto, marshalJSON func() ([]byte, error)) {
	var (
		data []byte
		err  error
	)
	if req.Header.Get("Content-Type") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		data, err = marshalJSON()
	} else {
		w.Header().Set("Content-Type", "application/x-protobuf")
		data, err = marshalProto()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}

// ── pdata → model conversion ───────────────────────────────────────────────

func (r *OTLPReceiver) consumeTraces(td ptrace.Traces) {
	rs := td.ResourceSpans()
	for i := 0; i < rs.Len(); i++ {
		resource := rs.At(i).Resource()
		service := stringAttr(resource.Attributes(), serviceNameAttr, r.name)
		ss := rs.At(i).ScopeSpans()
		for j := 0; j < ss.Len(); j++ {
			spans := ss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				r.emit(spanToModel(spans.At(k), service))
			}
		}
	}
}

func (r *OTLPReceiver) consumeMetrics(md pmetric.Metrics) {
	rm := md.ResourceMetrics()
	for i := 0; i < rm.Len(); i++ {
		resource := rm.At(i).Resource()
		service := stringAttr(resource.Attributes(), serviceNameAttr, r.name)
		sm := rm.At(i).ScopeMetrics()
		for j := 0; j < sm.Len(); j++ {
			metrics := sm.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				m := metrics.At(k)
				r.emitMetric(m, service)
			}
		}
	}
}

func (r *OTLPReceiver) consumeLogs(ld plog.Logs) {
	rl := ld.ResourceLogs()
	for i := 0; i < rl.Len(); i++ {
		resource := rl.At(i).Resource()
		service := stringAttr(resource.Attributes(), serviceNameAttr, r.name)
		sl := rl.At(i).ScopeLogs()
		for j := 0; j < sl.Len(); j++ {
			records := sl.At(j).LogRecords()
			for k := 0; k < records.Len(); k++ {
				r.emit(logRecordToModel(records.At(k), service))
			}
		}
	}
}

func spanToModel(s ptrace.Span, service string) model.SpanEvent {
	status := model.StatusUnset
	switch s.Status().Code() {
	case ptrace.StatusCodeOk:
		status = model.StatusOK
	case ptrace.StatusCodeError:
		status = model.StatusError
	}
	return model.SpanEvent{
		TraceID:   s.TraceID().String(),
		SpanID:    s.SpanID().String(),
		ParentID:  s.ParentSpanID().String(),
		Name:      s.Name(),
		Service:   service,
		StartTime: s.StartTimestamp().AsTime(),
		Duration:  s.EndTimestamp().AsTime().Sub(s.StartTimestamp().AsTime()),
		Status:    status,
		Attrs:     attrsToMap(s.Attributes()),
	}
}

func (r *OTLPReceiver) emitMetric(m pmetric.Metric, service string) {
	switch m.Type() {
	case pmetric.MetricTypeGauge:
		dps := m.Gauge().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			r.emit(numberDataPointToModel(dps.At(i), m.Name(), model.Gauge, service))
		}
	case pmetric.MetricTypeSum:
		dps := m.Sum().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			r.emit(numberDataPointToModel(dps.At(i), m.Name(), model.Counter, service))
		}
	case pmetric.MetricTypeHistogram:
		dps := m.Histogram().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			r.emit(model.MetricEvent{
				Name:      m.Name(),
				Value:     dp.Sum(),
				Labels:    attrsToMap(dp.Attributes()),
				Kind:      model.Histogram,
				Timestamp: dp.Timestamp().AsTime(),
				Source:    service,
			})
		}
	}
}

func numberDataPointToModel(dp pmetric.NumberDataPoint, name string, kind model.MetricKind, service string) model.MetricEvent {
	var v float64
	switch dp.ValueType() {
	case pmetric.NumberDataPointValueTypeDouble:
		v = dp.DoubleValue()
	case pmetric.NumberDataPointValueTypeInt:
		v = float64(dp.IntValue())
	}
	return model.MetricEvent{
		Name:      name,
		Value:     v,
		Labels:    attrsToMap(dp.Attributes()),
		Kind:      kind,
		Timestamp: dp.Timestamp().AsTime(),
		Source:    service,
	}
}

func logRecordToModel(rec plog.LogRecord, service string) model.LogEvent {
	level := model.LogLevel(rec.SeverityText())
	if level == "" {
		level = severityToLevel(rec.SeverityNumber())
	}
	return model.LogEvent{
		Timestamp: rec.Timestamp().AsTime(),
		Level:     level,
		Body:      rec.Body().AsString(),
		Service:   service,
		TraceID:   rec.TraceID().String(),
		Attrs:     attrsToMap(rec.Attributes()),
	}
}

func severityToLevel(n plog.SeverityNumber) model.LogLevel {
	switch {
	case n >= plog.SeverityNumberError:
		return model.LevelError
	case n >= plog.SeverityNumberWarn:
		return model.LevelWarn
	case n >= plog.SeverityNumberInfo:
		return model.LevelInfo
	case n >= plog.SeverityNumberDebug:
		return model.LevelDebug
	default:
		return model.LevelInfo
	}
}

func stringAttr(m pcommon.Map, key, fallback string) string {
	if v, ok := m.Get(key); ok {
		return v.AsString()
	}
	return fallback
}

func attrsToMap(m pcommon.Map) map[string]string {
	if m.Len() == 0 {
		return nil
	}
	out := make(map[string]string, m.Len())
	m.Range(func(k string, v pcommon.Value) bool {
		out[k] = v.AsString()
		return true
	})
	return out
}

