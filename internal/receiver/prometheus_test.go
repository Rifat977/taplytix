package receiver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"

	"github.com/rifat977/taplytix/internal/model"
)

const sampleProm = `# HELP http_requests_total The total HTTP requests.
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1027
http_requests_total{method="POST",code="500"} 3

# HELP go_memstats_alloc_bytes Bytes allocated.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 1.234e+06

# HELP request_duration Histogram of request durations.
# TYPE request_duration histogram
request_duration_bucket{le="0.1"} 7
request_duration_bucket{le="0.5"} 12
request_duration_bucket{le="+Inf"} 14
request_duration_sum 6.4
request_duration_count 14
`

func TestPrometheusFamilyConversion(t *testing.T) {
	parser := expfmt.NewTextParser(prommodel.UTF8Validation)
	families, err := parser.TextToMetricFamilies(bytes.NewBufferString(sampleProm))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var events []model.MetricEvent
	emit := func(ev any) { events = append(events, ev.(model.MetricEvent)) }
	for _, fam := range families {
		convertFamily(fam, "test", emit)
	}

	counter := find(events, "http_requests_total", map[string]string{"method": "GET", "code": "200"})
	if counter == nil || counter.Value != 1027 || counter.Kind != model.Counter {
		t.Errorf("counter event = %+v", counter)
	}
	gauge := find(events, "go_memstats_alloc_bytes", nil)
	if gauge == nil || gauge.Kind != model.Gauge || gauge.Value != 1234000 {
		t.Errorf("gauge event = %+v", gauge)
	}
	bucket := find(events, "request_duration_bucket", map[string]string{"le": "0.5"})
	if bucket == nil || bucket.Value != 12 || bucket.Kind != model.Histogram {
		t.Errorf("histogram bucket = %+v", bucket)
	}
	if find(events, "request_duration_sum", nil) == nil {
		t.Errorf("missing _sum event")
	}
	if find(events, "request_duration_count", nil) == nil {
		t.Errorf("missing _count event")
	}
}

func find(events []model.MetricEvent, name string, labels map[string]string) *model.MetricEvent {
	for i := range events {
		if events[i].Name != name {
			continue
		}
		match := true
		for k, v := range labels {
			if events[i].Labels[k] != v {
				match = false
				break
			}
		}
		if match {
			return &events[i]
		}
	}
	return nil
}

func TestPrometheusReceiverScrapesEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleProm))
	}))
	defer srv.Close()

	r := NewPrometheus("test", srv.URL, 50*time.Millisecond)
	out := make(chan any, 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx, out); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	deadline := time.After(2 * time.Second)
	gotCounter := false
	for !gotCounter {
		select {
		case ev := <-out:
			if me, ok := ev.(model.MetricEvent); ok && me.Name == "http_requests_total" {
				gotCounter = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for counter event")
		}
	}
}
