package stream

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/gfanton/gnotif/internal/trigger"
)

const (
	// ceiling: a connection more than sendBuffer frames behind is closed, so
	// one window matching more than that for a page drops it even when it
	// reads; a queue capped in bytes if catch-up windows grow past it.
	sendBuffer   = 256
	writeTimeout = 10 * time.Second
)

// HubConfig holds the hub's dependencies and its cap.
type HubConfig struct {
	Protocol Protocol
	Max      int // most connections held at once; 0 refuses every upgrade
	Log      *slog.Logger
}

// Hub holds live connections in this process and sends each one the matches
// its set names.
type Hub struct {
	cfg    HubConfig
	wg     sync.WaitGroup
	mu     sync.Mutex
	conns  map[*conn]struct{}
	byKey  map[string]map[*conn]struct{}
	closed bool
}

type conn struct {
	ws   *websocket.Conn
	send chan []byte
	// stop carries the first close requested by another goroutine; the
	// writer sends it, so no caller waits on the peer's close handshake.
	stop chan websocket.StatusCode
	keys []string // guarded by Hub.mu
	// cancel ends the connection at once, its close handshake included;
	// guarded by Hub.mu.
	cancel  context.CancelFunc
	dropped atomic.Bool // closed for reading too slowly
}

// NewHub returns a hub holding no connections.
func NewHub(cfg HubConfig) *Hub {
	return &Hub{cfg: cfg, conns: map[*conn]struct{}{}, byKey: map[string]map[*conn]struct{}{}}
}

// ServeHTTP upgrades GET /v1/stream and serves the connection until it ends.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		writeError(w, http.StatusUpgradeRequired, "websocket upgrade required")
		return
	}
	c := &conn{send: make(chan []byte, sendBuffer), stop: make(chan websocket.StatusCode, 1)}
	if refusal := h.reserve(c); refusal != "" {
		writeError(w, http.StatusServiceUnavailable, refusal)
		return
	}
	defer h.release(c)

	// Every origin may connect: the protocol carries no credentials.
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return // Accept has answered the request
	}
	ws.SetReadLimit(MaxFrame)
	c.ws = ws
	h.serve(c)
}

// Publish queues each match's frame for the connections whose set names it.
// It never blocks: a connection whose buffer is full is closed instead.
func (h *Hub) Publish(matches []trigger.Match) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range matches {
		set := h.byKey[Key(m.TriggerID, m.Value)]
		if len(set) == 0 {
			continue
		}
		frame := NotificationFrame(m)
		for c := range set {
			h.queue(c, frame)
		}
	}
}

// Close closes every connection with 1001, refuses new ones, and returns once
// every connection has ended. Connections still open when ctx ends are cut
// without waiting for their peer's close.
func (h *Hub) Close(ctx context.Context) {
	h.mu.Lock()
	h.closed = true
	for c := range h.conns {
		stop(c, websocket.StatusGoingAway)
	}
	h.mu.Unlock()

	ended := make(chan struct{})
	go func() {
		defer close(ended)
		h.wg.Wait()
	}()
	select {
	case <-ended:
		return
	case <-ctx.Done():
	}
	h.mu.Lock()
	for c := range h.conns {
		if c.cancel != nil {
			c.cancel()
		}
	}
	h.mu.Unlock()
	<-ended
}

// reserve adds c to the hub, or returns why it cannot.
func (h *Hub) reserve(c *conn) (refusal string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.cfg.Max == 0:
		return "live notifications are off on this server"
	case h.closed:
		return "too many live connections"
	case len(h.conns) >= h.cfg.Max:
		h.cfg.Log.Warn("live connection refused at the cap", "max", h.cfg.Max)
		return "too many live connections"
	}
	h.conns[c] = struct{}{}
	h.wg.Add(1)
	return ""
}

func (h *Hub) release(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.index(c, nil)
	delete(h.conns, c)
	h.wg.Done()
}

// index replaces c's keys. The caller holds h.mu.
func (h *Hub) index(c *conn, keys []string) {
	for _, k := range c.keys {
		delete(h.byKey[k], c)
		if len(h.byKey[k]) == 0 {
			delete(h.byKey, k)
		}
	}
	c.keys = keys
	for _, k := range keys {
		if h.byKey[k] == nil {
			h.byKey[k] = map[*conn]struct{}{}
		}
		h.byKey[k][c] = struct{}{}
	}
}

func (h *Hub) serve(c *conn) {
	// The library answers some read errors, such as a frame over the limit,
	// with a close frame only; ending the connection here lets the client
	// see the close.
	defer c.ws.CloseNow()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.mu.Lock()
	c.cancel = cancel
	h.mu.Unlock()
	written := make(chan struct{})
	go func() {
		defer close(written)
		write(ctx, c)
	}()
	h.read(ctx, c)
	cancel()
	<-written
}

func (h *Hub) read(ctx context.Context, c *conn) {
	for {
		typ, frame, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			c.ws.Close(websocket.StatusUnsupportedData, "")
			return
		}
		r := h.cfg.Protocol.Handle(ctx, frame)
		if r.Replaced {
			h.mu.Lock()
			h.index(c, r.Keys)
			h.mu.Unlock()
		}
		h.queue(c, r.Frame)
	}
}

// write sends c's frames until the connection ends. A requested close goes
// out before any frame still waiting.
func write(ctx context.Context, c *conn) {
	for {
		select {
		case code := <-c.stop:
			c.ws.Close(code, "")
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case code := <-c.stop:
			c.ws.Close(code, "")
			return
		case frame := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.ws.Write(wctx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				// A peer that does not read cannot read a close frame either.
				c.ws.CloseNow()
				return
			}
		}
	}
}

func (h *Hub) queue(c *conn, frame []byte) {
	select {
	case c.send <- frame:
	default:
		if c.dropped.CompareAndSwap(false, true) {
			stop(c, websocket.StatusTryAgainLater)
			h.cfg.Log.Warn("live connection dropped: too slow to read", "behind", sendBuffer)
		}
	}
}

func stop(c *conn, code websocket.StatusCode) {
	select {
	case c.stop <- code:
	default:
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
