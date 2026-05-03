package receiver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"

	"github.com/rifat977/taplytix/internal/model"
)

// PrometheusReceiver polls a Prometheus text-format endpoint at a fixed
// interval and converts each metric family into model.MetricEvent.
type PrometheusReceiver struct {
	name     string
	endpoint string
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	out    chan<- any
	client *http.Client
}

func NewPrometheus(name, endpoint string, interval time.Duration) *PrometheusReceiver {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &PrometheusReceiver{
		name:     name,
		endpoint: endpoint,
		interval: interval,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (r *PrometheusReceiver) Name() string { return r.name }

func (r *PrometheusReceiver) Start(ctx context.Context, out chan<- any) error {
	if r.endpoint == "" {
		return errors.New("prometheus receiver: endpoint required")
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return errors.New("prometheus receiver: already started")
	}
	subCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.out = out
	r.mu.Unlock()
	go r.run(subCtx)
	return nil
}

func (r *PrometheusReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	return nil
}

func (r *PrometheusReceiver) emit(ev any) {
	r.mu.Lock()
	out := r.out
	r.mu.Unlock()
	if out == nil {
		return
	}
	select {
	case out <- ev:
	default:
	}
}

func (r *PrometheusReceiver) run(ctx context.Context) {
	backoff := time.Second
	for {
		// Initial poll happens immediately, subsequent polls wait `interval`.
		err := r.scrapeOnce(ctx)
		if err != nil {
			// Exponential backoff up to 30s on errors.
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.interval):
		}
	}
}

func (r *PrometheusReceiver) scrapeOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus scrape: %s returned %d", r.endpoint, resp.StatusCode)
	}
	parser := expfmt.NewTextParser(prommodel.UTF8Validation)
	families, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return fmt.Errorf("prometheus parse: %w", err)
	}
	for _, fam := range families {
		convertFamily(fam, r.name, r.emit)
	}
	return nil
}

// convertFamily walks one Prometheus MetricFamily and emits one MetricEvent
// per data point. Histograms and summaries are decomposed into _count, _sum,
// and _bucket / quantile child series. Exposed to package tests via the
// `emit` callback.
func convertFamily(fam *dto.MetricFamily, source string, emit func(any)) {
	name := fam.GetName()
	now := time.Now()
	switch fam.GetType() {
	case dto.MetricType_COUNTER:
		for _, m := range fam.GetMetric() {
			emit(model.MetricEvent{
				Name: name, Value: m.GetCounter().GetValue(),
				Labels: labelsToMap(m.GetLabel()), Kind: model.Counter,
				Timestamp: now, Source: source,
			})
		}
	case dto.MetricType_GAUGE:
		for _, m := range fam.GetMetric() {
			emit(model.MetricEvent{
				Name: name, Value: m.GetGauge().GetValue(),
				Labels: labelsToMap(m.GetLabel()), Kind: model.Gauge,
				Timestamp: now, Source: source,
			})
		}
	case dto.MetricType_UNTYPED:
		for _, m := range fam.GetMetric() {
			emit(model.MetricEvent{
				Name: name, Value: m.GetUntyped().GetValue(),
				Labels: labelsToMap(m.GetLabel()), Kind: model.Gauge,
				Timestamp: now, Source: source,
			})
		}
	case dto.MetricType_HISTOGRAM:
		for _, m := range fam.GetMetric() {
			h := m.GetHistogram()
			labels := labelsToMap(m.GetLabel())
			emit(model.MetricEvent{
				Name: name + "_count", Value: float64(h.GetSampleCount()),
				Labels: labels, Kind: model.Counter, Timestamp: now, Source: source,
			})
			emit(model.MetricEvent{
				Name: name + "_sum", Value: h.GetSampleSum(),
				Labels: labels, Kind: model.Counter, Timestamp: now, Source: source,
			})
			for _, bk := range h.GetBucket() {
				bl := copyMap(labels)
				bl["le"] = formatBound(bk.GetUpperBound())
				emit(model.MetricEvent{
					Name: name + "_bucket", Value: float64(bk.GetCumulativeCount()),
					Labels: bl, Kind: model.Histogram, Timestamp: now, Source: source,
				})
			}
		}
	case dto.MetricType_SUMMARY:
		for _, m := range fam.GetMetric() {
			s := m.GetSummary()
			labels := labelsToMap(m.GetLabel())
			emit(model.MetricEvent{
				Name: name + "_count", Value: float64(s.GetSampleCount()),
				Labels: labels, Kind: model.Counter, Timestamp: now, Source: source,
			})
			emit(model.MetricEvent{
				Name: name + "_sum", Value: s.GetSampleSum(),
				Labels: labels, Kind: model.Counter, Timestamp: now, Source: source,
			})
			for _, q := range s.GetQuantile() {
				ql := copyMap(labels)
				ql["quantile"] = formatBound(q.GetQuantile())
				emit(model.MetricEvent{
					Name: name, Value: q.GetValue(),
					Labels: ql, Kind: model.Histogram, Timestamp: now, Source: source,
				})
			}
		}
	}
}

func labelsToMap(pairs []*dto.LabelPair) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		out[p.GetName()] = p.GetValue()
	}
	return out
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func formatBound(v float64) string {
	s := fmt.Sprintf("%g", v)
	if !strings.Contains(s, ".") && !strings.ContainsAny(s, "eE") {
		// preserve "+Inf"/integer formatting
	}
	return s
}
