// Package server wires gnotifd: the store, the watch loop, the delivery
// loop, the live hub, and an HTTP server for the /v1 API.
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
	"github.com/gfanton/gnotif/internal/stream"
	"github.com/gfanton/gnotif/internal/subscribe"
	"github.com/gfanton/gnotif/internal/trigger"
	"github.com/gfanton/gnotif/internal/watch"
)

const (
	maxWindow       = 1000
	storeEvery      = 10 * time.Minute
	pushTTL         = 24 * time.Hour
	shutdownTimeout = 10 * time.Second
	publishTimeout  = 5 * time.Second
)

// Config is gnotifd's configuration.
type Config struct {
	Listener      net.Listener // nil: listen on Listen
	Listen        string
	Indexer       string // tx-indexer GraphQL URL
	Registry      string // registry path ending in /v<N>; later versions are followed too
	StartHeight   int64  // first height to read when the database has no cursor
	DB            string
	Poll, MaxAge  time.Duration
	VAPID         delivery.Keys
	VAPIDSubject  string
	PushHosts     []string
	PushClient    *http.Client // nil: delivery.NewClient()
	IndexerClient *http.Client // nil: a client with a 30 s timeout
	StreamMax     int          // most live connections held at once
	HubListener   net.Listener // nil: listen on HubListen when it is set
	HubListen     string       // external-hub listener; empty: none
	Publish       string       // URL receiving each window's matches; empty: none
	PublishToken  string
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
	hubLn := cfg.HubListener
	if hubLn == nil && cfg.HubListen != "" {
		if hubLn, err = net.Listen("tcp", cfg.HubListen); err != nil {
			ln.Close()
			return fmt.Errorf("listen on %s: %w", cfg.HubListen, err)
		}
	}

	protocol := stream.Protocol{TriggersByID: st.TriggersByID, Log: cfg.Log}
	hub := stream.NewHub(stream.HubConfig{Protocol: protocol, Max: cfg.StreamMax, Log: cfg.Log})
	var publisher *stream.Publisher
	if cfg.Publish != "" {
		publisher = stream.NewPublisher(stream.PublisherConfig{
			URL: cfg.Publish, Token: cfg.PublishToken, Client: &http.Client{}, Timeout: publishTimeout, Log: cfg.Log,
		})
	}
	publish := func(matches []trigger.Match) {
		hub.Publish(matches)
		if publisher != nil {
			publisher.Publish(matches)
		}
	}

	now := func() time.Time { return time.Now().UTC() }
	wake := make(chan struct{}, 1)
	queue := func(ctx context.Context, pushes []store.Push) {
		pending := make([]store.Pending, len(pushes))
		at := now()
		for i, p := range pushes {
			pending[i] = store.Pending{Push: p, NextAttempt: at}
		}
		if err := st.Queue(ctx, pending); err != nil {
			cfg.Log.Error("drop pushes", "count", len(pushes), "err", err)
			return
		}
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	watcher := watch.New(watch.Config{
		Source:      indexer.New(cfg.Indexer, cmp.Or(cfg.IndexerClient, &http.Client{Timeout: 30 * time.Second})),
		Store:       st,
		Registry:    registry,
		StartHeight: cfg.StartHeight,
		Poll:        cfg.Poll,
		MaxAge:      cfg.MaxAge,
		MaxWindow:   maxWindow,
		StoreEvery:  storeEvery,
		Send:        queue,
		Now:         now,
		Log:         cfg.Log,
		Publish:     publish,
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
		Stream:    hub,
		Log:       cfg.Log,
	}))
	servers := []*http.Server{newHTTPServer(cfg, mux)}
	listeners := []net.Listener{ln}
	if hubLn != nil {
		servers = append(servers, newHTTPServer(cfg, stream.MessageHandler(protocol)))
		listeners = append(listeners, hubLn)
	}

	loopCtx, stop := context.WithCancel(ctx)
	defer stop()
	serveErr := make(chan error, len(servers))
	var wg sync.WaitGroup
	wg.Go(func() { watcher.Run(loopCtx) })
	wg.Go(func() { sender.Run(loopCtx) })
	if publisher != nil {
		wg.Go(func() { publisher.Run(loopCtx) })
	}
	for i, srv := range servers {
		wg.Go(func() {
			if err := srv.Serve(listeners[i]); !errors.Is(err, http.ErrServerClosed) {
				serveErr <- err
				stop()
			}
		})
	}
	cfg.Log.Info("gnotifd started", "addr", ln.Addr().String(), "registry", cfg.Registry, "indexer", cfg.Indexer)
	if hubLn != nil {
		cfg.Log.Info("external-hub listener started", "addr", hubLn.Addr().String())
	}

	<-loopCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	// Shutdown does not track upgraded connections, so the hub closes them.
	hub.Close(shutdownCtx)
	var shutdownErr error
	for _, srv := range servers {
		shutdownErr = errors.Join(shutdownErr, srv.Shutdown(shutdownCtx))
	}
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
