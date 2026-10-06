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
	BlockTxs(ctx context.Context, height int64) ([]indexer.TxRef, time.Time, error)
	FetchTx(ctx context.Context, height int64, index int) (indexer.Batch, error)
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

// Tick processes one window. A failed tick halves the next window, since the
// indexer refuses a query that returns too many transactions, and a
// successful one doubles it, up to MaxWindow. A window of one block too large
// to read whole is read one transaction at a time.
func (w *Watcher) Tick(ctx context.Context) error {
	if err := w.tick(ctx); err != nil {
		w.window = max(1, w.window/2)
		return err
	}
	w.window = min(w.cfg.MaxWindow, 2*w.window)
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
	// Every other path below stores a cursor without NextTx, so a block left
	// half read is finished first.
	if cur.NextTx > 0 {
		return w.readBlock(ctx, cur)
	}

	// The indexer resolves the fields of one query concurrently, so a window
	// is bounded by the height reported on the previous tick, which is
	// complete, never by the height reported alongside it.
	if cur.Bound <= cur.HeightDone {
		latest, err := w.cfg.Source.Latest(ctx)
		if err != nil {
			return err
		}
		if latest < cur.HeightDone {
			w.cfg.Log.Warn("indexer behind stored bound", "latest", latest, "height_done", cur.HeightDone)
		}
		return w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
			return tx.SetCursor(store.Cursor{HeightDone: cur.HeightDone, Bound: latest})
		})
	}
	end := min(cur.Bound, cur.HeightDone+w.window)

	batch, err := w.cfg.Source.Fetch(ctx, indexer.Window{
		From: cur.HeightDone,
		To:   end,
	})
	if errors.Is(err, indexer.ErrTooLarge) && end == cur.HeightDone+1 {
		w.cfg.Log.Warn("block too large, reading it one transaction at a time", "height", end, "err", err)
		return w.readBlock(ctx, cur)
	}
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

	var c counts
	err = w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
		var err error
		if c, err = w.process(tx, batch.Events, batch.BlockTimes); err != nil {
			return err
		}
		return tx.SetCursor(store.Cursor{HeightDone: end, Bound: batch.Latest})
	})
	if err != nil {
		return err
	}
	w.report(c, cur.HeightDone+1, end)
	return nil
}

// readBlock reads block HeightDone+1 one transaction at a time from
// cur.NextTx, storing the next index after each one. A transaction too large
// to read is skipped and its events are lost; any other error stops the block
// at that transaction.
func (w *Watcher) readBlock(ctx context.Context, cur store.Cursor) error {
	h := cur.HeightDone + 1
	refs, at, err := w.cfg.Source.BlockTxs(ctx, h)
	if err != nil {
		return err
	}
	blockTimes := map[int64]time.Time{h: at}
	var total counts
	defer func() { w.report(total, h, h) }()
	for _, ref := range refs {
		if ref.Index < cur.NextTx {
			continue
		}
		var events []trigger.Event
		batch, err := w.cfg.Source.FetchTx(ctx, h, ref.Index)
		switch {
		case errors.Is(err, indexer.ErrTooLarge):
			w.cfg.Log.Error("skip oversized transaction", "height", h, "index", ref.Index, "tx", ref.Hash, "err", err)
		case err != nil:
			return err
		default:
			events = batch.Events
		}
		next := store.Cursor{HeightDone: cur.HeightDone, Bound: cur.Bound, NextTx: ref.Index + 1}
		var c counts
		err = w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
			var err error
			if c, err = w.process(tx, events, blockTimes); err != nil {
				return err
			}
			return tx.SetCursor(next)
		})
		if err != nil {
			return err
		}
		total.queued += c.queued
		total.stale += c.stale
	}
	return w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
		return tx.SetCursor(store.Cursor{HeightDone: h, Bound: cur.Bound})
	})
}

type counts struct{ queued, stale int }

// process applies events in the order given, which must be chain order: a
// trigger declared or removed by one event applies to every event after it.
func (w *Watcher) process(tx *store.Tx, events []trigger.Event, blockTimes map[int64]time.Time) (counts, error) {
	var c counts
	for _, e := range events {
		if e.PkgPath == w.cfg.Registry {
			if err := w.applyRegistry(tx, e); err != nil {
				return c, err
			}
			continue
		}
		blockTime, ok := blockTimes[e.Height]
		if !ok {
			w.cfg.Log.Warn("skip event without block time", "height", e.Height, "tx", e.TxHash)
			continue
		}
		if w.cfg.Now().Sub(blockTime) > w.cfg.MaxAge {
			c.stale++
			continue
		}
		n, err := w.enqueue(tx, e)
		if err != nil {
			return c, err
		}
		c.queued += n
	}
	return c, nil
}

// report warns of the stale events read in heights from through to,
// inclusive, and wakes the sender when pushes were queued.
func (w *Watcher) report(c counts, from, to int64) {
	if c.stale > 0 {
		w.cfg.Log.Warn("skipped stale events", "count", c.stale, "from", from, "to", to, "max_age", w.cfg.MaxAge)
	}
	if c.queued > 0 && w.cfg.Wake != nil {
		select {
		case w.cfg.Wake <- struct{}{}:
		default:
		}
	}
}

func (w *Watcher) applyRegistry(tx *store.Tx, e trigger.Event) error {
	switch e.Type {
	case "TriggerDeclared":
		t, err := trigger.FromDeclared(e.Attrs)
		if err != nil {
			w.cfg.Log.Error("skip registry event", "height", e.Height, "tx", e.TxHash, "err", err)
			return nil
		}
		if !t.Verified {
			w.cfg.Log.Debug("skip unverified trigger", "id", t.ID, "target", t.Target, "height", e.Height, "tx", e.TxHash)
			return nil
		}
		return tx.PutTrigger(t)
	case "TriggerRemoved":
		id, ok := e.Attr("id")
		if !ok || id == "" {
			w.cfg.Log.Error("skip registry event", "height", e.Height, "tx", e.TxHash, "err", "TriggerRemoved without id")
			return nil
		}
		return tx.DeleteTrigger(id)
	}
	return nil
}

func (w *Watcher) enqueue(tx *store.Tx, e trigger.Event) (int, error) {
	triggers, err := tx.Matching(e.PkgPath, e.Type)
	if err != nil {
		return 0, err
	}
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
