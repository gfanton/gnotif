// Command gnotifd sends browser Web Push notifications for gno.land realm
// events that match triggers declared in the gnotif registry realm.
//
//	gnotifd keygen
//	gnotifd -indexer URL -registry PKGPATH -vapid-subject EMAIL [flags]
//
// The VAPID key pair comes from GNOTIF_VAPID_PUBLIC_KEY and
// GNOTIF_VAPID_PRIVATE_KEY; keygen prints a new pair in that form.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/gfanton/gnotif/internal/delivery"
	"github.com/gfanton/gnotif/internal/server"
	"github.com/gfanton/gnotif/internal/subscribe"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "keygen" {
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			fmt.Fprintln(stderr, "gnotifd keygen:", err)
			return 1
		}
		fmt.Fprintf(stdout, "GNOTIF_VAPID_PUBLIC_KEY=%s\nGNOTIF_VAPID_PRIVATE_KEY=%s\n", pub, priv)
		return 0
	}
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		fmt.Fprintln(stderr, "gnotifd:", err)
		return 2
	}
	cfg.Log = slog.New(slog.NewTextHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, cfg); err != nil {
		cfg.Log.Error("gnotifd stopped", "err", err)
		return 1
	}
	return 0
}

func parseConfig(args []string, getenv func(string) string) (server.Config, error) {
	fs := flag.NewFlagSet("gnotifd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var cfg server.Config
	var pushHosts string
	fs.StringVar(&cfg.Listen, "listen", "127.0.0.1:8080", "HTTP listen address")
	fs.StringVar(&cfg.Indexer, "indexer", "", "tx-indexer GraphQL URL (required)")
	fs.StringVar(&cfg.Registry, "registry", "", "package path of the gnotif registry realm (required)")
	fs.Int64Var(&cfg.StartHeight, "start-height", 0, "registry deploy height, required on first start")
	fs.StringVar(&cfg.DB, "db", "gnotif.db", "SQLite database file")
	fs.DurationVar(&cfg.Poll, "poll", 5*time.Second, "indexer poll interval")
	fs.DurationVar(&cfg.MaxAge, "max-age", 10*time.Minute, "oldest event that still sends a notification")
	fs.StringVar(&cfg.VAPIDSubject, "vapid-subject", "", "contact email or https URL sent to push services (required)")
	fs.StringVar(&pushHosts, "push-hosts", strings.Join(subscribe.DefaultPushHosts, ","), "comma-separated push service hosts")
	if err := fs.Parse(args); err != nil {
		return server.Config{}, err
	}

	// webpush-go adds "mailto:" to a subject that is not an https URL; a
	// doubled prefix makes Apple's push service reject every push.
	cfg.VAPIDSubject = strings.TrimPrefix(cfg.VAPIDSubject, "mailto:")
	cfg.VAPID = delivery.Keys{Public: getenv("GNOTIF_VAPID_PUBLIC_KEY"), Private: getenv("GNOTIF_VAPID_PRIVATE_KEY")}
	for _, req := range []struct{ name, value string }{
		{"-indexer", cfg.Indexer},
		{"-registry", cfg.Registry},
		{"-vapid-subject", cfg.VAPIDSubject},
		{"GNOTIF_VAPID_PUBLIC_KEY", cfg.VAPID.Public},
		{"GNOTIF_VAPID_PRIVATE_KEY", cfg.VAPID.Private},
	} {
		if req.value == "" {
			return server.Config{}, errors.New(req.name + " is required")
		}
	}
	for host := range strings.SplitSeq(pushHosts, ",") {
		if host = strings.TrimSpace(host); host != "" {
			cfg.PushHosts = append(cfg.PushHosts, host)
		}
	}
	return cfg, nil
}
