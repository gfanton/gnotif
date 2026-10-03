// Package watch polls the indexer for registry and realm events, keeps the
// stored triggers in step with the registry, and queues a push for every
// opt-in an event matches.
package watch

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gfanton/gnotif/internal/indexer"
	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

// ErrNoStartHeight reports a first start without a height to read from.
var ErrNoStartHeight = errors.New("start height required on first start")

// Source is the indexer the watcher reads.
type Source interface {
	Latest(ctx context.Context) (int64, error)
	Fetch(ctx context.Context, w indexer.Window) (indexer.Batch, error)
}

// Config holds the watcher's dependencies and settings.
type Config struct {
	Source      Source
	Store       *store.Store
	Registry    string // package path of the gnotif registry realm
	StartHeight int64  // first height to read when the store has no cursor
	Poll        time.Duration
	MaxAge      time.Duration // older events are recorded but never notified
	MaxWindow   int64         // most blocks read in one tick
	Wake        chan<- struct{}
	Now         func() time.Time
	Log         *slog.Logger
}

// Watcher runs the poll loop. Tick and Run are not safe for concurrent use.
type Watcher struct {
	cfg    Config
	window int64
}

// New returns a watcher.
func New(cfg Config) *Watcher {
	return &Watcher{cfg: cfg, window: cfg.MaxWindow}
}

// Run ticks every Poll until ctx ends.
func (w *Watcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.Poll)
	defer ticker.Stop()
	for {
		if err := w.Tick(ctx); err != nil && ctx.Err() == nil {
			w.cfg.Log.Warn("watch tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Tick processes one window. A failed tick leaves the cursor in place and
// halves the next window, since the indexer refuses a query that returns
// too many transactions; a successful one restores MaxWindow.
func (w *Watcher) Tick(ctx context.Context) error {
	if err := w.tick(ctx); err != nil {
		w.window = max(1, w.window/2)
		return err
	}
	w.window = w.cfg.MaxWindow
	return nil
}

func (w *Watcher) tick(ctx context.Context) error {
	cur, ok, err := w.cfg.Store.Cursor(ctx)
	if err != nil {
		return err
	}
	if !ok {
		if w.cfg.StartHeight < 1 {
			return ErrNoStartHeight
		}
		cur = store.Cursor{HeightDone: w.cfg.StartHeight - 1}
	}

	// The indexer resolves the fields of one query concurrently, so a window
	// is bounded by the height reported on the previous tick, which is
	// complete, never by the height reported alongside it.
	if cur.Bound <= cur.HeightDone {
		latest, err := w.cfg.Source.Latest(ctx)
		if err != nil {
			return err
		}
		return w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
			return tx.SetCursor(store.Cursor{HeightDone: cur.HeightDone, Bound: latest})
		})
	}
	end := min(cur.Bound, cur.HeightDone+w.window)

	var targets []string
	if err := w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
		var err error
		targets, err = tx.Targets()
		return err
	}); err != nil {
		return err
	}
	batch, err := w.cfg.Source.Fetch(ctx, indexer.Window{
		From:  cur.HeightDone,
		To:    end,
		Paths: append([]string{w.cfg.Registry}, targets...),
	})
	if err != nil {
		return err
	}
	// An indexer that re-synced from scratch, or another instance behind the
	// same URL, can report a height below the stored bound. The window above
	// that height is incomplete: lower the bound and read it later.
	if batch.Latest < end {
		w.cfg.Log.Warn("indexer behind stored bound", "latest", batch.Latest, "bound", cur.Bound)
		return w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
			return tx.SetCursor(store.Cursor{HeightDone: cur.HeightDone, Bound: batch.Latest})
		})
	}

	queued, stale := 0, 0
	err = w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
		triggers, err := tx.Triggers()
		if err != nil {
			return err
		}
		for _, e := range batch.Events {
			if e.PkgPath == w.cfg.Registry {
				changed, err := w.applyRegistry(tx, e)
				if err != nil {
					return err
				}
				if changed {
					if triggers, err = tx.Triggers(); err != nil {
						return err
					}
				}
				continue
			}
			blockTime, ok := batch.BlockTimes[e.Height]
			if !ok {
				w.cfg.Log.Warn("skip event without block time", "height", e.Height, "tx", e.TxHash)
				continue
			}
			if w.cfg.Now().Sub(blockTime) > w.cfg.MaxAge {
				stale++
				continue
			}
			n, err := w.enqueue(tx, triggers, e)
			if err != nil {
				return err
			}
			queued += n
		}
		return tx.SetCursor(store.Cursor{HeightDone: end, Bound: batch.Latest})
	})
	if err != nil {
		return err
	}
	if stale > 0 {
		w.cfg.Log.Warn("skipped stale events", "count", stale, "from", cur.HeightDone+1, "to", end, "max_age", w.cfg.MaxAge)
	}
	if queued > 0 && w.cfg.Wake != nil {
		select {
		case w.cfg.Wake <- struct{}{}:
		default:
		}
	}
	return nil
}

func (w *Watcher) applyRegistry(tx *store.Tx, e trigger.Event) (bool, error) {
	switch e.Type {
	case "TriggerDeclared":
		t, err := trigger.FromDeclared(e.Attrs)
		if err != nil {
			w.cfg.Log.Error("skip registry event", "height", e.Height, "tx", e.TxHash, "err", err)
			return false, nil
		}
		return true, tx.PutTrigger(t)
	case "TriggerRemoved":
		id, ok := e.Attr("id")
		if !ok || id == "" {
			w.cfg.Log.Error("skip registry event", "height", e.Height, "tx", e.TxHash, "err", "TriggerRemoved without id")
			return false, nil
		}
		return true, tx.DeleteTrigger(id)
	}
	return false, nil
}

func (w *Watcher) enqueue(tx *store.Tx, triggers []trigger.Trigger, e trigger.Event) (int, error) {
	queued := 0
	for _, t := range triggers {
		if !t.Matches(e) {
			continue
		}
		var value string
		if t.Param != "" {
			v, ok := e.Attr(t.Param)
			if !ok {
				continue
			}
			value = v
		}
		subs, err := tx.Subscribers(t.ID, value)
		if err != nil {
			return queued, err
		}
		n := t.Render(e)
		for _, sub := range subs {
			added, err := tx.Enqueue(store.Push{
				SubscriptionID: sub,
				TriggerID:      t.ID,
				TxHash:         e.TxHash,
				EventIndex:     e.Index,
				Notification:   n,
				CreatedAt:      w.cfg.Now(),
			})
			if err != nil {
				return queued, err
			}
			if added {
				queued++
			}
		}
	}
	return queued, nil
}
