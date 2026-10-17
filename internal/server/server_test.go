package server

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/delivery"
	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/store/storetest"
	"github.com/gfanton/gnotif/internal/stream"
	"github.com/gfanton/gnotif/internal/subscribe"
	"github.com/gfanton/gnotif/internal/trigger"
	"github.com/gfanton/gnotif/internal/watch"
)

func config(t *testing.T, startHeight int64) Config {
	t.Helper()
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"latestBlockHeight":1}}`)
	}))
	t.Cleanup(idx.Close)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	return Config{
		Listener:     ln,
		Indexer:      idx.URL,
		Registry:     "gno.land/r/dev/gnotif/v0",
		StartHeight:  startHeight,
		DB:           filepath.Join(t.TempDir(), "gnotif.db"),
		Poll:         20 * time.Millisecond,
		MaxAge:       10 * time.Minute,
		VAPID:        delivery.Keys{Public: pub, Private: priv},
		VAPIDSubject: "test@gnotif.example",
		PushHosts:    subscribe.DefaultPushHosts,
		Log:          slog.New(slog.DiscardHandler),
	}
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, string(body)
}

func TestServesAPI(t *testing.T) {
	cfg := config(t, 1)
	base := "http://" + cfg.Listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()

	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/v1/vapid")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)

	resp, body := get(t, base+"/v1/vapid")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var vapid struct{ PublicKey string }
	require.NoError(t, json.Unmarshal([]byte(body), &vapid))
	assert.Equal(t, cfg.VAPID.Public, vapid.PublicKey)

	cancel()
	var err error
	require.Eventually(t, func() bool {
		select {
		case err = <-done:
			return true
		default:
			return false
		}
	}, 15*time.Second, 20*time.Millisecond)
	assert.NoError(t, err)
}

func TestFirstStartNeedsStartHeight(t *testing.T) {
	cfg := config(t, 0)
	err := Run(context.Background(), cfg)
	assert.True(t, errors.Is(err, watch.ErrNoStartHeight), err)
}

func TestRootNotServed(t *testing.T) {
	cfg := config(t, 1)
	base := "http://" + cfg.Listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})

	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/v1/vapid")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)

	resp, _ := get(t, base+"/")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// runServer starts Run and waits until it answers; stop cancels it and
// returns what Run returned.
func runServer(t *testing.T, cfg Config) (base string, stop func() error) {
	t.Helper()
	base = "http://" + cfg.Listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/v1/vapid")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)
	var once sync.Once
	var runErr error
	stop = func() error {
		once.Do(func() {
			cancel()
			select {
			case runErr = <-done:
			case <-time.After(15 * time.Second):
				runErr = errors.New("Run did not return")
			}
		})
		return runErr
	}
	t.Cleanup(func() { stop() })
	return base, stop
}

// Every write is a file the database's replica keeps and a restore reads, so
// gnotifd must not store its cursor on every poll of a quiet chain.
func TestQuietChainKeepsStoredCursor(t *testing.T) {
	cfg := config(t, 1)
	var polls, windows atomic.Int32
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Variables json.RawMessage }
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Variables) > 0 && string(req.Variables) != "null" {
			windows.Add(1)
		}
		polls.Add(1)
		io.WriteString(w, `{"data":{"latestBlockHeight":1}}`)
	}))
	t.Cleanup(idx.Close)
	cfg.Indexer = idx.URL
	_, stop := runServer(t, cfg)
	require.Eventually(t, func() bool { return polls.Load() >= 5 }, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, stop())
	require.Equal(t, int32(1), windows.Load(), "the one window read, and never read again")

	st, err := store.Open(context.Background(), cfg.DB)
	require.NoError(t, err)
	defer st.Close()
	cur, ok, err := st.Cursor(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, store.Cursor{HeightDone: 0, Bound: 1}, cur, "the cursor stored by the first poll")
}

func TestMatchReachesPushService(t *testing.T) {
	var pushes atomic.Int32
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pushes.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(service.Close)

	cfg := config(t, 1)
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DB)
	require.NoError(t, err)
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	auth := make([]byte, 16)
	rand.Read(auth)
	sub := store.Subscription{
		Endpoint: service.URL + "/push",
		P256dh:   base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}
	_, err = st.Exec(ctx, []store.Statement{
		store.PutTriggerStatement(trigger.Trigger{
			ID: "0000001", Target: "gno.land/r/demo/game", Event: "TurnPlayed", Param: "next",
			Declarer: "g1alice", Title: "Your turn", Body: "Game {game}", Link: "/",
		}),
		store.CursorStatement(store.Cursor{HeightDone: 10, Bound: 11}),
	})
	require.NoError(t, err)
	require.NoError(t, st.PutSubscription(ctx, sub))
	require.NoError(t, st.ReplaceOptins(ctx, sub.Endpoint, []store.Optin{{TriggerID: "0000001", Value: "g1bob"}}))
	require.NoError(t, st.Close())

	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"latestBlockHeight": 11,
			"getBlocks":         []any{map[string]any{"height": 11, "time": time.Now().UTC().Format(time.RFC3339Nano)}},
			"getTransactions": []any{map[string]any{
				"hash": "txa", "block_height": 11, "index": 0,
				"response": map[string]any{"events": []any{map[string]any{
					"__typename": "GnoEvent", "type": "TurnPlayed", "pkg_path": "gno.land/r/demo/game",
					"attrs": []any{map[string]any{"key": "game", "value": "7"}, map[string]any{"key": "next", "value": "g1bob"}},
				}}},
			}},
		}})
	}))
	t.Cleanup(idx.Close)
	cfg.Indexer = idx.URL
	runServer(t, cfg)

	require.Eventually(t, func() bool { return pushes.Load() == 1 }, 5*time.Second, 20*time.Millisecond)
}

func TestServesLiveStream(t *testing.T) {
	cfg := config(t, 1)
	cfg.StreamMax = 10
	base, stop := runServer(t, cfg)

	resp, body := get(t, base+"/v1/stream")
	assert.Equal(t, http.StatusUpgradeRequired, resp.StatusCode)
	assert.JSONEq(t, `{"error":"websocket upgrade required"}`, body)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, base+"/v1/stream", nil)
	require.NoError(t, err)
	defer c.CloseNow()
	exchange := func(frame, want string) {
		t.Helper()
		require.NoError(t, c.Write(ctx, websocket.MessageText, []byte(frame)))
		_, got, err := c.Read(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	}
	exchange(stream.PingFrame, stream.PongFrame)
	exchange(`{"type":"optins","optins":[]}`, `{"type":"optins"}`)

	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	_, _, err = c.Read(ctx)
	assert.Equal(t, websocket.StatusGoingAway, websocket.CloseStatus(err))
	assert.NoError(t, <-stopped)
}

func TestHubListener(t *testing.T) {
	cfg := config(t, 1)
	hub, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	cfg.HubListener = hub
	base, _ := runServer(t, cfg)

	resp, err := http.Post("http://"+hub.Addr().String()+"/message", "text/plain", strings.NewReader(stream.PingFrame))
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"reply":"{\"type\":\"pong\"}"}`, string(body))

	resp, err = http.Post(base+"/message", "text/plain", strings.NewReader(stream.PingFrame))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestServerErrorLogUsesSlog(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{Log: slog.New(slog.NewTextHandler(&buf, nil))}
	srv := newHTTPServer(cfg, http.NotFoundHandler())

	srv.ErrorLog.Print("tls: handshake failed")
	assert.Contains(t, buf.String(), "level=WARN")
	assert.Contains(t, buf.String(), "tls: handshake failed")

	assert.Equal(t, 5*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 10*time.Second, srv.ReadTimeout)
	assert.Equal(t, 10*time.Second, srv.WriteTimeout)
	assert.Equal(t, 60*time.Second, srv.IdleTimeout)
}

func TestInvalidRegistry(t *testing.T) {
	cfg := config(t, 1)
	cfg.Registry = "gno.land/r/dev/gnotif"
	err := Run(context.Background(), cfg)
	assert.ErrorIs(t, err, watch.ErrInvalidRegistry)
	assert.NoFileExists(t, cfg.DB)
}

// Checking an opt-in reads the watcher's trigger snapshot, so it must keep
// answering once the store cannot be reached.
func TestOptinChecksWithoutStore(t *testing.T) {
	const target = "gno.land/r/demo/game"
	srv := storetest.NewServer(t)
	ctx := context.Background()
	st, err := store.Open(ctx, srv.URL)
	require.NoError(t, err)
	_, err = st.Exec(ctx, []store.Statement{store.PutTriggerStatement(trigger.Trigger{
		ID: "0000001", Target: target, Event: "TurnPlayed", Param: "next",
		Declarer: "g1alice", Title: "Your turn", Body: "Game {game}", Link: "/",
	})})
	require.NoError(t, err)
	require.NoError(t, st.Close())

	cfg := config(t, 1)
	cfg.DB = srv.URL
	hub, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	cfg.HubListener = hub
	base, _ := runServer(t, cfg)
	srv.Close()

	req, err := http.NewRequest(http.MethodPut, base+"/v1/subscription/optins",
		strings.NewReader(`{"endpoint":"https://fcm.googleapis.com/fcm/send/abc","optins":[{"trigger":"nope","value":""}]}`))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp, body := get(t, base+"/v1/triggers?target="+target)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, `"id":"0000001"`)

	resp, err = http.Post("http://"+hub.Addr().String()+"/message", "text/plain",
		strings.NewReader(`{"type":"optins","optins":[{"trigger":"0000001","value":"g1bob"}]}`))
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var answer struct{ Reply string }
	require.NoError(t, json.Unmarshal(raw, &answer))
	assert.JSONEq(t, `{"type":"optins"}`, answer.Reply)
}

func TestStartFailsWithUnreachableStore(t *testing.T) {
	srv := storetest.NewServer(t)
	cfg := config(t, 1)
	cfg.DB = srv.URL
	srv.Close()

	assert.Error(t, Run(context.Background(), cfg))
}
