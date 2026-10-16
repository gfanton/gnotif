// Package watch polls the indexer for registry and realm events, keeps the
// stored triggers in step with the registry, and queues a push for every
// opt-in an event matches.
package watch

import (
	"context"
	"errors"
	"fmt"
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
	Registry    Registry // the registry version to follow, and every later one
	StartHeight int64    // first height to read when the store has no cursor
	Poll        time.Duration
	MaxAge      time.Duration // older events are recorded but never notified
	MaxWindow   int64         // most blocks read in one tick
	Wake        chan<- struct{}
	Now         func() time.Time
	Log         *slog.Logger
	// Publish receives every match, in chain order, once the transaction
	// that read it is committed. Nil publishes nowhere.
	Publish func([]trigger.Match)
	// StoreEvery is how long the watcher may read on past the stored cursor
	// through windows that change nothing; zero stores it every tick. A
	// restart reads those windows again.
	StoreEvery time.Duration
}

// Watcher runs the poll loop. Tick and Run are not safe for concurrent use.
type Watcher struct {
	cfg    Config
	window int64
	// held is the cursor read on past the stored one, nil while the stored
	// one is current.
	held     *store.Cursor
	storedAt time.Time
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
	cur, ok, err := w.cursor(ctx)
	if err != nil {
		return err
	}
	if !ok {
		if w.cfg.StartHeight < 1 {
			return ErrNoStartHeight
		}
		cur = store.Cursor{HeightDone: w.cfg.StartHeight - 1}
	}
	// Every other path below moves to a cursor without NextTx, so a block left
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
		return w.advance(ctx, store.Cursor{HeightDone: cur.HeightDone, Bound: latest}, nil)
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
		return w.advance(ctx, store.Cursor{HeightDone: cur.HeightDone, Bound: batch.Latest}, nil)
	}

	var c counts
	var matches []trigger.Match
	err = w.advance(ctx, store.Cursor{HeightDone: end, Bound: batch.Latest}, func(tx *store.Tx) (bool, error) {
		var err error
		c, matches, err = w.process(tx, batch.Events, batch.BlockTimes)
		return c.lasting(), err
	})
	if err != nil {
		return err
	}
	w.report(c, cur.HeightDone+1, end)
	w.publish(matches)
	return nil
}

// readBlock reads block HeightDone+1 one transaction at a time from
// cur.NextTx, moving to the next index after each one. A transaction too large
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
		var matches []trigger.Match
		err = w.advance(ctx, next, func(tx *store.Tx) (bool, error) {
			var err error
			c, matches, err = w.process(tx, events, blockTimes)
			return c.lasting(), err
		})
		if err != nil {
			return err
		}
		w.publish(matches)
		total.queued += c.queued
		total.stale += c.stale
	}
	return w.advance(ctx, store.Cursor{HeightDone: h, Bound: cur.Bound}, nil)
}

// cursor returns the held cursor, or else the stored one.
func (w *Watcher) cursor(ctx context.Context) (store.Cursor, bool, error) {
	if w.held != nil {
		return *w.held, true, nil
	}
	return w.cfg.Store.Cursor(ctx)
}

// advance runs apply, when not nil, and moves the cursor to next in one
// transaction. The cursor is stored at once when apply reports its window as
// lasting; otherwise it is held until the stored one is StoreEvery old.
func (w *Watcher) advance(ctx context.Context, next store.Cursor, apply func(*store.Tx) (bool, error)) error {
	hold := false
	err := w.cfg.Store.Update(ctx, func(tx *store.Tx) error {
		lasting := false
		if apply != nil {
			var err error
			if lasting, err = apply(tx); err != nil {
				return err
			}
		}
		if hold = !lasting && w.cfg.Now().Sub(w.storedAt) < w.cfg.StoreEvery; hold {
			return nil
		}
		return tx.SetCursor(next)
	})
	if err != nil {
		return err
	}
	if hold {
		w.held = &next
	} else {
		w.held, w.storedAt = nil, w.cfg.Now()
	}
	return nil
}

type counts struct{ queued, stale, registry, matched int }

// lasting reports whether a window is stored with its cursor at once: its
// registry events may write the triggers, and read again its matches would
// be published twice.
func (c counts) lasting() bool { return c.registry > 0 || c.matched > 0 }

// process applies events in the order given, which must be chain order: a
// trigger declared or removed by one event applies to every event after it.
// It returns every match, subscribed or not, for Publish.
func (w *Watcher) process(tx *store.Tx, events []trigger.Event, blockTimes map[int64]time.Time) (counts, []trigger.Match, error) {
	var c counts
	var matches []trigger.Match
	for _, e := range events {
		if version, ok := w.cfg.Registry.Version(e.PkgPath); ok {
			if err := w.applyRegistry(tx, e, version); err != nil {
				return c, nil, err
			}
			c.registry++
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
		e.Time = blockTime
		m, n, err := w.enqueue(tx, e)
		if err != nil {
			return c, nil, err
		}
		matches = append(matches, m...)
		c.matched += len(m)
		c.queued += n
	}
	return c, matches, nil
}

func (w *Watcher) publish(matches []trigger.Match) {
	if len(matches) > 0 && w.cfg.Publish != nil {
		w.cfg.Publish(matches)
	}
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

// applyRegistry replays an event of registry version into the triggers. A
// version may only touch ids it issued: ids restart in every version, and
// PutTrigger replaces a trigger of the same id.
func (w *Watcher) applyRegistry(tx *store.Tx, e trigger.Event, version int) error {
	foreign := func(id string) bool {
		if idVersion(id) == version {
			return false
		}
		w.cfg.Log.Error("skip registry event", "height", e.Height, "tx", e.TxHash,
			"err", fmt.Sprintf("id %s does not belong to registry version %d", id, version))
		return true
	}
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
		if foreign(t.ID) {
			return nil
		}
		return tx.PutTrigger(t)
	case "TriggerRemoved":
		id, ok := e.Attr("id")
		if !ok || id == "" {
			w.cfg.Log.Error("skip registry event", "height", e.Height, "tx", e.TxHash, "err", "TriggerRemoved without id")
			return nil
		}
		if foreign(id) {
			return nil
		}
		return tx.DeleteTrigger(id)
	}
	return nil
}

// enqueue queues a push for every subscriber of every trigger e matches, and
// returns those matches, subscribed or not.
func (w *Watcher) enqueue(tx *store.Tx, e trigger.Event) ([]trigger.Match, int, error) {
	triggers, err := tx.Matching(e.PkgPath, e.Type)
	if err != nil {
		return nil, 0, err
	}
	var matches []trigger.Match
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
		n := t.Render(e)
		matches = append(matches, trigger.Match{TriggerID: t.ID, Value: value, Notification: n, Event: e})
		subs, err := tx.Subscribers(t.ID, value)
		if err != nil {
			return nil, queued, err
		}
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
				return nil, queued, err
			}
			if added {
				queued++
			}
		}
	}
	return matches, queued, nil
}
