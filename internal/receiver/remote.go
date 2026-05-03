package receiver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/rifat977/taplytix/internal/model"
)

// RemoteReceiver connects to a taplytix-agent WebSocket endpoint and
// publishes the decoded events to the local pipeline. Reconnects with
// exponential backoff on disconnect.
type RemoteReceiver struct {
	name     string
	endpoint string

	mu     sync.Mutex
	cancel context.CancelFunc
	out    chan<- any
}

// NewRemote returns a receiver that connects to `endpoint`. The endpoint
// may include or omit the path; if omitted, "/events" is appended (matching
// the agent's default mount).
func NewRemote(name, endpoint string) *RemoteReceiver {
	if u, err := url.Parse(endpoint); err == nil && (u.Path == "" || u.Path == "/") {
		u.Path = "/events"
		endpoint = u.String()
	}
	return &RemoteReceiver{name: name, endpoint: endpoint}
}

func (r *RemoteReceiver) Name() string { return r.name }

func (r *RemoteReceiver) Start(ctx context.Context, out chan<- any) error {
	if r.endpoint == "" {
		return errors.New("remote receiver: endpoint required")
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return errors.New("remote receiver: already started")
	}
	subCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.out = out
	r.mu.Unlock()
	go r.runLoop(subCtx)
	return nil
}

func (r *RemoteReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	return nil
}

func (r *RemoteReceiver) runLoop(ctx context.Context) {
	backoff := time.Second
	for {
		err := r.connectAndPump(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (r *RemoteReceiver) connectAndPump(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, r.endpoint, &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	})
	if err != nil {
		return fmt.Errorf("remote dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 20) // 1 MiB / frame

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		ev, err := model.DecodeEvent(data)
		if err != nil {
			continue
		}
		r.emit(ev)
	}
}

func (r *RemoteReceiver) emit(ev any) {
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
