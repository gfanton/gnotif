// Package watch polls the indexer for registry and realm events, keeps the
// stored triggers in step with the registry, and hands the sender a push for
// every opt-in an event matches.
package watch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
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

// Store is the database the watcher loads from and commits each window to.
type Store interface {
	Exec(ctx context.Context, stmts []store.Statement) ([]store.Result, error)
	Triggers(ctx context.Context) ([]trigger.Trigger, error)
	Cursor(ctx context.Context) (store.Cursor, bool, error)
}

// Config holds the watcher's dependencies and settings.
type Config struct {
	Source      Source
	Store       Store
	Registry    Registry // the registry version to follow, and every later one
	StartHeight int64    // first height to read when the store has no cursor
	Poll        time.Duration
	MaxAge      time.Duration // older events are recorded but never notified
	MaxWindow   int64         // most blocks read in one tick
	// Send receives one push per subscriber of every match, once the batch
	// that read the subscribers is committed.
	Send func(ctx context.Context, pushes []store.Push)
	Now  func() time.Time
	Log  *slog.Logger
	// Publish receives every match, in chain order, once the batch that
	// read it is committed. Nil publishes nowhere.
	Publish func([]trigger.Match)
	// StoreEvery is how long the watcher may read on past the stored cursor
	// through windows that change nothing; zero stores it every tick. A
	// restart reads those windows again.
	StoreEvery time.Duration
}

// Watcher runs the poll loop. Load, Tick and Run are not safe for concurrent
// use; TargetTriggers and TriggersByID are, once Load has returned.
type Watcher struct {
	cfg    Config
	window int64
	// loaded is false until Load succeeds, and again after a failed batch
	// leaves the stored cursor and triggers unknown.
	loaded   bool
	cur      store.Cursor // the position read up to, stored or not
	storedAt time.Time
	triggers atomic.Pointer[Triggers]
}

// New returns a watcher.
func New(cfg Config) *Watcher {
	return &Watcher{cfg: cfg, window: cfg.MaxWindow}
}

// Load reads the stored cursor and triggers. Without a stored cursor,
// reading starts at StartHeight, and a StartHeight below 1 is
// ErrNoStartHeight. A tick loads first when nothing is loaded.
func (w *Watcher) Load(ctx context.Context) error {
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
	ts, err := w.cfg.Store.Triggers(ctx)
	if err != nil {
		return err
	}
	w.cur, w.loaded = cur, true
	w.triggers.Store(NewTriggers(ts))
	return nil
}

// TargetTriggers returns the triggers of one realm in id order, at most
// store.MaxPerTarget, as of the last committed window. The error is always
// nil.
func (w *Watcher) TargetTriggers(ctx context.Context, target string) ([]trigger.Trigger, error) {
	return w.triggers.Load().TargetTriggers(ctx, target)
}

// TriggersByID returns the triggers among ids, keyed by id, as of the last
// committed window. Unknown ids are absent. The error is always nil.
func (w *Watcher) TriggersByID(ctx context.Context, ids []string) (map[string]trigger.Trigger, error) {
	return w.triggers.Load().TriggersByID(ctx, ids)
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
// indexer refuses a query that returns too many transactions and the store a
// batch of too many statements, and a successful one doubles it, up to
// MaxWindow. A window of one block too large to read or to commit whole is
// read one transaction at a time.
func (w *Watcher) Tick(ctx context.Context) error {
	if err := w.tick(ctx); err != nil {
		w.window = max(1, w.window/2)
		return err
	}
	w.window = min(w.cfg.MaxWindow, 2*w.window)
	return nil
}

func (w *Watcher) tick(ctx context.Context) error {
	if !w.loaded {
		if err := w.Load(ctx); err != nil {
			return err
		}
	}
	cur := w.cur
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
		_, err = w.advance(ctx, store.Cursor{HeightDone: cur.HeightDone, Bound: latest}, nil, nil)
		return err
	}
	end := min(cur.Bound, cur.HeightDone+w.window)

	batch, err := w.cfg.Source.Fetch(ctx, indexer.Window{
		From: cur.HeightDone,
		To:   end,
	})
	if err != nil {
		return w.splitBlock(ctx, cur, end, err)
	}
	// An indexer that re-synced from scratch, or another instance behind the
	// same URL, can report a height below the stored bound. The window above
	// that height is incomplete: lower the bound and read it later.
	if batch.Latest < end {
		w.cfg.Log.Warn("indexer behind stored bound", "latest", batch.Latest, "bound", cur.Bound)
		_, err := w.advance(ctx, store.Cursor{HeightDone: cur.HeightDone, Bound: batch.Latest}, nil, nil)
		return err
	}

	stale, err := w.advance(ctx, store.Cursor{HeightDone: end, Bound: batch.Latest}, batch.Events, batch.BlockTimes)
	if err != nil {
		return w.splitBlock(ctx, cur, end, err)
	}
	w.warnStale(stale, cur.HeightDone+1, end)
	return nil
}

// splitBlock reads a window of one block one transaction at a time when err
// says it is too large to read or to commit whole, and otherwise returns err.
func (w *Watcher) splitBlock(ctx context.Context, cur store.Cursor, end int64, err error) error {
	if !tooLarge(err) || end != cur.HeightDone+1 {
		return err
	}
	w.cfg.Log.Warn("block too large, reading it one transaction at a time", "height", end, "err", err)
	return w.readBlock(ctx, cur)
}

// tooLarge reports the indexer refusing a read, or the store a batch, for
// its size.
func tooLarge(err error) bool {
	return errors.Is(err, indexer.ErrTooLarge) || errors.Is(err, store.ErrBatchTooLarge)
}

// readBlock reads block HeightDone+1 one transaction at a time from
// cur.NextTx, moving to the next index after each one. A transaction too large
// to read or to commit is skipped and its events are lost; any other error
// stops the block at that transaction.
func (w *Watcher) readBlock(ctx context.Context, cur store.Cursor) error {
	h := cur.HeightDone + 1
	refs, at, err := w.cfg.Source.BlockTxs(ctx, h)
	if err != nil {
		return err
	}
	blockTimes := map[int64]time.Time{h: at}
	stale := 0
	defer func() { w.warnStale(stale, h, h) }()
	for _, ref := range refs {
		if ref.Index < cur.NextTx {
			continue
		}
		next := store.Cursor{HeightDone: cur.HeightDone, Bound: cur.Bound, NextTx: ref.Index + 1}
		batch, err := w.cfg.Source.FetchTx(ctx, h, ref.Index)
		n := 0
		if err == nil {
			n, err = w.advance(ctx, next, batch.Events, blockTimes)
		}
		if tooLarge(err) {
			w.cfg.Log.Error("skip oversized transaction", "height", h, "index", ref.Index, "tx", ref.Hash, "err", err)
			_, err = w.advance(ctx, next, nil, nil)
		}
		if err != nil {
			return err
		}
		stale += n
	}
	_, err = w.advance(ctx, store.Cursor{HeightDone: h, Bound: cur.Bound}, nil, nil)
	return err
}

// advance processes events and moves the cursor to next in one batch, which
// stores the cursor with a window that has registry events or matches, and
// otherwise once the stored one is StoreEvery old. Only a committed window
// reaches the snapshot, Send and Publish. It returns the number of stale
// events skipped.
func (w *Watcher) advance(ctx context.Context, next store.Cursor, events []trigger.Event, blockTimes map[int64]time.Time) (int, error) {
	b := w.process(events, blockTimes)
	stored := b.lasting() || w.cfg.Now().Sub(w.storedAt) >= w.cfg.StoreEvery
	if stored {
		b.stmts = append(b.stmts, store.CursorStatement(next))
	}
	var pushes []store.Push
	if len(b.stmts) > 0 {
		res, err := w.cfg.Store.Exec(ctx, b.stmts)
		if err == nil {
			pushes, err = b.pushes(res, w.cfg.Now())
		}
		if err != nil {
			// Exec refuses an oversized batch before running any of it.
			if !errors.Is(err, store.ErrBatchTooLarge) {
				w.loaded = false
			}
			return 0, fmt.Errorf("commit window: %w", err)
		}
	}
	w.triggers.Store(b.triggers)
	w.cur = next
	if stored {
		w.storedAt = w.cfg.Now()
	}
	if len(pushes) > 0 {
		w.cfg.Send(ctx, pushes)
	}
	w.publish(b.matches)
	return b.stale, nil
}

// windowBatch is a window processed against a copy of the snapshot.
type windowBatch struct {
	triggers *Triggers // the snapshot once the batch commits
	stmts    []store.Statement
	matches  []trigger.Match
	reads    []int // per match, the index in stmts of its subscribers' read
	registry int   // registry events, applied or skipped
	stale    int
}

// lasting reports whether a window is stored with its cursor at once: its
// registry events may write the triggers, and read again its matches would
// be published twice.
func (b *windowBatch) lasting() bool { return b.registry > 0 || len(b.matches) > 0 }

// pushes returns one push per subscriber of every match, in match order,
// from the results of b.stmts.
func (b *windowBatch) pushes(res []store.Result, createdAt time.Time) ([]store.Push, error) {
	var out []store.Push
	for i, m := range b.matches {
		subs, err := store.SubscribersOf(res[b.reads[i]])
		if err != nil {
			return nil, err
		}
		for _, s := range subs {
			out = append(out, store.Push{
				SubscriptionID: s.ID,
				Subscription:   s.Subscription,
				TriggerID:      m.TriggerID,
				TxHash:         m.Event.TxHash,
				EventIndex:     m.Event.Index,
				Notification:   m.Notification,
				CreatedAt:      createdAt,
			})
		}
	}
	return out, nil
}

// process applies events in the order given, which must be chain order: a
// trigger declared or removed by one event applies to every event after it.
// It keeps every match, subscribed or not, for Publish.
func (w *Watcher) process(events []trigger.Event, blockTimes map[int64]time.Time) *windowBatch {
	b := &windowBatch{triggers: w.triggers.Load()}
	for _, e := range events {
		if version, ok := w.cfg.Registry.Version(e.PkgPath); ok {
			w.applyRegistry(b, e, version)
			b.registry++
			continue
		}
		blockTime, ok := blockTimes[e.Height]
		if !ok {
			w.cfg.Log.Warn("skip event without block time", "height", e.Height, "tx", e.TxHash)
			continue
		}
		if w.cfg.Now().Sub(blockTime) > w.cfg.MaxAge {
			b.stale++
			continue
		}
		e.Time = blockTime
		b.match(e)
	}
	return b
}

func (w *Watcher) publish(matches []trigger.Match) {
	if len(matches) > 0 && w.cfg.Publish != nil {
		w.cfg.Publish(matches)
	}
}

// warnStale warns of the stale events read in heights from through to,
// inclusive.
func (w *Watcher) warnStale(count int, from, to int64) {
	if count > 0 {
		w.cfg.Log.Warn("skipped stale events", "count", count, "from", from, "to", to, "max_age", w.cfg.MaxAge)
	}
}

// applyRegistry replays an event of registry version onto b. A version may
// only touch ids it issued: ids restart in every version, and a declaration
// replaces the trigger of the same id.
func (w *Watcher) applyRegistry(b *windowBatch, e trigger.Event, version int) {
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
			return
		}
		if !t.Verified {
			w.cfg.Log.Debug("skip unverified trigger", "id", t.ID, "target", t.Target, "height", e.Height, "tx", e.TxHash)
			return
		}
		if foreign(t.ID) {
			return
		}
		edit := b.triggers.edit()
		edit.put(t)
		b.triggers = edit.done()
		b.stmts = append(b.stmts, store.PutTriggerStatement(t))
	case "TriggerRemoved":
		id, ok := e.Attr("id")
		if !ok || id == "" {
			w.cfg.Log.Error("skip registry event", "height", e.Height, "tx", e.TxHash, "err", "TriggerRemoved without id")
			return
		}
		if foreign(id) {
			return
		}
		edit := b.triggers.edit()
		edit.delete(id)
		b.triggers = edit.done()
		b.stmts = append(b.stmts, store.DeleteTriggerStatement(id))
	}
}

// match appends, for every trigger e matches, the read of the subscribers
// opted in to the trigger with the event's value.
func (b *windowBatch) match(e trigger.Event) {
	for _, t := range b.triggers.Matching(e.PkgPath, e.Type) {
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
		b.reads = append(b.reads, len(b.stmts))
		b.stmts = append(b.stmts, store.SubscribersStatement(t.ID, value))
		b.matches = append(b.matches, trigger.Match{TriggerID: t.ID, Value: value, Notification: t.Render(e), Event: e})
	}
}
