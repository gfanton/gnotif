package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/delivery"
	"github.com/gfanton/gnotif/internal/subscribe"
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
