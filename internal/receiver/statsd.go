package receiver

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

// StatsDReceiver listens on a UDP address and parses metrics in the
// `name:value|type[|@rate][|#tag:val,tag:val]` format. Multiple metrics may
// share one datagram separated by newlines.
type StatsDReceiver struct {
	name string
	addr string

	mu     sync.Mutex
	conn   *net.UDPConn
	out    chan<- any
	cancel context.CancelFunc
}

func NewStatsD(name, addr string) *StatsDReceiver {
	if addr == "" {
		addr = ":8125"
	}
	return &StatsDReceiver{name: name, addr: addr}
}

func (r *StatsDReceiver) Name() string { return r.name }

func (r *StatsDReceiver) Start(ctx context.Context, out chan<- any) error {
	udpAddr, err := net.ResolveUDPAddr("udp", r.addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.conn != nil {
		r.mu.Unlock()
		_ = conn.Close()
		return errors.New("statsd: already started")
	}
	subCtx, cancel := context.WithCancel(ctx)
	r.conn = conn
	r.cancel = cancel
	r.out = out
	r.mu.Unlock()

	go r.run(subCtx)
	return nil
}

func (r *StatsDReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
	}
	return nil
}

func (r *StatsDReceiver) emit(ev any) {
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

func (r *StatsDReceiver) run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		r.mu.Lock()
		if r.conn != nil {
			_ = r.conn.Close()
		}
		r.mu.Unlock()
	}()
	buf := make([]byte, 65535)
	for {
		r.mu.Lock()
		conn := r.conn
		r.mu.Unlock()
		if conn == nil {
			return
		}
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(buf[:n]), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if ev, ok := parseStatsDLine(line, r.name); ok {
				r.emit(ev)
			}
		}
	}
}

// parseStatsDLine parses one StatsD line. The trailing |@rate and |#tag,tag
// fields are optional and may appear in either order.
func parseStatsDLine(line, source string) (model.MetricEvent, bool) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return model.MetricEvent{}, false
	}
	name := line[:colon]
	rest := line[colon+1:]
	parts := strings.Split(rest, "|")
	if len(parts) < 2 {
		return model.MetricEvent{}, false
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return model.MetricEvent{}, false
	}
	typ := parts[1]
	kind, ok := statsdKind(typ)
	if !ok {
		return model.MetricEvent{}, false
	}
	ev := model.MetricEvent{
		Name:      name,
		Value:     val,
		Kind:      kind,
		Timestamp: time.Now(),
		Source:    source,
	}
	for _, suffix := range parts[2:] {
		switch {
		case strings.HasPrefix(suffix, "@"):
			if rate, err := strconv.ParseFloat(suffix[1:], 64); err == nil && rate > 0 && rate < 1 {
				ev.Value /= rate
			}
		case strings.HasPrefix(suffix, "#"):
			ev.Labels = parseStatsDTags(suffix[1:])
		}
	}
	return ev, true
}

func statsdKind(typ string) (model.MetricKind, bool) {
	switch typ {
	case "c":
		return model.Counter, true
	case "g":
		return model.Gauge, true
	case "ms", "h":
		return model.Histogram, true
	case "s":
		return model.Counter, true // sets — count of unique values; treat as counter
	default:
		return 0, false
	}
}

func parseStatsDTags(s string) map[string]string {
	out := map[string]string{}
	for _, tag := range strings.Split(s, ",") {
		if i := strings.IndexByte(tag, ':'); i >= 0 {
			out[tag[:i]] = tag[i+1:]
		} else if tag != "" {
			out[tag] = ""
		}
	}
	return out
}
