package stream

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/gfanton/gnotif/internal/trigger"
)

const (
	sendBuffer   = 16
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
	if !h.reserve(c) {
		writeError(w, http.StatusServiceUnavailable, "too many live connections")
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
			queue(c, frame)
		}
	}
}

// Close closes every connection with 1001, refuses new ones, and returns once
// every connection has ended.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	for c := range h.conns {
		stop(c, websocket.StatusGoingAway)
	}
	h.mu.Unlock()
	h.wg.Wait()
}

func (h *Hub) reserve(c *conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.conns) >= h.cfg.Max {
		return false
	}
	h.conns[c] = struct{}{}
	h.wg.Add(1)
	return true
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
		queue(c, r.Frame)
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

func queue(c *conn, frame []byte) {
	select {
	case c.send <- frame:
	default:
		stop(c, websocket.StatusTryAgainLater)
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
