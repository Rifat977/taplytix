// Package agent provides the WebSocket broadcaster used by the taplytix-agent
// binary. It accepts dashboard connections and forwards every event received
// on its input channel as a JSON-encoded WireEvent to all connected clients.
package agent

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/rifat977/taplytix/internal/model"
)

// Broadcaster fans an event stream out to one or more WebSocket clients.
// It owns the HTTP handler used for connection upgrade.
type Broadcaster struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{clients: make(map[*client]struct{})}
}

// Pump pulls events from `in` and forwards them to every connected client.
// Returns when ctx is done or `in` is closed.
func (b *Broadcaster) Pump(ctx context.Context, in <-chan any) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			data, err := model.EncodeEvent(ev)
			if err != nil {
				continue
			}
			b.fanout(data)
		}
	}
}

func (b *Broadcaster) fanout(data []byte) {
	b.mu.Lock()
	clients := make([]*client, 0, len(b.clients))
	for c := range b.clients {
		clients = append(clients, c)
	}
	b.mu.Unlock()
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			// Slow client — drop rather than block. The dashboard will
			// catch up on its next tick.
		}
	}
}

// ServeHTTP accepts a WebSocket upgrade and serves one client until it
// disconnects. Mounted at the desired path by the caller.
func (b *Broadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // CORS handled by transport (TLS / firewall)
	})
	if err != nil {
		return
	}

	c := &client{conn: conn, send: make(chan []byte, 1024)}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.clients, c)
		b.mu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	// Reader: required for WS keepalive (ping/close); we don't expect
	// dashboard-to-agent traffic so anything received is a signal to bail.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-c.send:
			writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, msg)
			writeCancel()
			if err != nil {
				return
			}
		}
	}
}

// ClientCount reports the number of currently connected clients. Useful
// for tests and admin endpoints.
func (b *Broadcaster) ClientCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.clients)
}

// ListenAndServe is a convenience helper used by the agent binary. Starts a
// listener on `addr` and serves the broadcaster at `/events`.
func (b *Broadcaster) ListenAndServe(ctx context.Context, addr string) error {
	srv, lis, err := b.buildServer(addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// ListenAndServeTLS is the TLS variant; certFile and keyFile must point at
// PEM-encoded files.
func (b *Broadcaster) ListenAndServeTLS(ctx context.Context, addr, certFile, keyFile string) error {
	srv, lis, err := b.buildServer(addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.ServeTLS(lis, certFile, keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (b *Broadcaster) buildServer(addr string) (*http.Server, net.Listener, error) {
	mux := http.NewServeMux()
	mux.Handle("/events", b)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	return srv, lis, nil
}
