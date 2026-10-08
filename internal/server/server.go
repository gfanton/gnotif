// Package server wires gnotifd: the store, the watch loop, the delivery
// loop, and an HTTP server for the /v1 subscription API.
package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gfanton/gnotif/internal/delivery"
	"github.com/gfanton/gnotif/internal/indexer"
	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/subscribe"
	"github.com/gfanton/gnotif/internal/watch"
)

const (
	maxWindow       = 1000
	pushTTL         = 24 * time.Hour
	shutdownTimeout = 10 * time.Second
)

// Config is gnotifd's configuration.
type Config struct {
	Listener      net.Listener // nil: listen on Listen
	Listen        string
	Indexer       string // tx-indexer GraphQL URL
	Registry      string // package path of the gnotif registry realm
	StartHeight   int64  // first height to read when the database has no cursor
	DB            string
	Poll, MaxAge  time.Duration
	VAPID         delivery.Keys
	VAPIDSubject  string
	PushHosts     []string
	PushClient    *http.Client // nil: delivery.NewClient()
	IndexerClient *http.Client // nil: a client with a 30 s timeout
	Log           *slog.Logger
}

// Run serves until ctx ends, then shuts the HTTP server down and waits for
// both loops.
func Run(ctx context.Context, cfg Config) error {
	registry, err := watch.ParseRegistry(cfg.Registry)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, ok, err := st.Cursor(ctx); err != nil {
		return err
	} else if !ok && cfg.StartHeight < 1 {
		return watch.ErrNoStartHeight
	}

	ln := cfg.Listener
	if ln == nil {
		if ln, err = net.Listen("tcp", cfg.Listen); err != nil {
			return fmt.Errorf("listen on %s: %w", cfg.Listen, err)
		}
	}

	now := func() time.Time { return time.Now().UTC() }
	wake := make(chan struct{}, 1)
	watcher := watch.New(watch.Config{
		Source:      indexer.New(cfg.Indexer, cmp.Or(cfg.IndexerClient, &http.Client{Timeout: 30 * time.Second})),
		Store:       st,
		Registry:    registry,
		StartHeight: cfg.StartHeight,
		Poll:        cfg.Poll,
		MaxAge:      cfg.MaxAge,
		MaxWindow:   maxWindow,
		Wake:        wake,
		Now:         now,
		Log:         cfg.Log,
	})
	sender := delivery.New(delivery.Config{
		Store:   st,
		Client:  cmp.Or(cfg.PushClient, delivery.NewClient()),
		Keys:    cfg.VAPID,
		Subject: cfg.VAPIDSubject,
		TTL:     pushTTL,
		Wake:    wake,
		Now:     now,
		Log:     cfg.Log,
	})

	mux := http.NewServeMux()
	mux.Handle("/v1/", subscribe.NewHandler(subscribe.Config{
		Store:     st,
		PublicKey: cfg.VAPID.Public,
		PushHosts: cfg.PushHosts,
		Log:       cfg.Log,
	}))
	srv := newHTTPServer(cfg, mux)

	loopCtx, stop := context.WithCancel(ctx)
	defer stop()
	serveErr := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() { watcher.Run(loopCtx) })
	wg.Go(func() { sender.Run(loopCtx) })
	wg.Go(func() {
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			stop()
		}
	})
	cfg.Log.Info("gnotifd started", "addr", ln.Addr().String(), "registry", cfg.Registry, "indexer", cfg.Indexer)

	<-loopCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	wg.Wait()
	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	default:
	}
	if shutdownErr != nil {
		return fmt.Errorf("shut down: %w", shutdownErr)
	}
	return nil
}

func newHTTPServer(cfg Config, h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(cfg.Log.Handler(), slog.LevelWarn),
	}
}
