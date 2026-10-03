package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/indexer"
	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

const (
	registry = "gno.land/r/dev/gnotif/v0"
	game     = "gno.land/r/demo/game"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// ---- fake tx-indexer

type fakeEvent struct {
	pkg, typ string
	attrs    []trigger.Pair
}

type fakeTx struct {
	height int64
	index  int
	hash   string
	events []fakeEvent
}

type fakeIndexer struct {
	mu      sync.Mutex
	latest  int64
	txs     []fakeTx
	times   map[int64]time.Time
	fail    bool
	windows []indexer.Window
}

func (f *fakeIndexer) add(height int64, events ...fakeEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txs = append(f.txs, fakeTx{height: height, hash: "tx" + string(rune('a'+len(f.txs))), events: events})
}

func (f *fakeIndexer) set(apply func(f *fakeIndexer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	apply(f)
}

func (f *fakeIndexer) recorded() []indexer.Window {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.windows)
}

func (f *fakeIndexer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct {
		Variables *struct {
			Where struct {
				BlockHeight struct{ Gt, Lt int64 } `json:"block_height"`
				Response    struct {
					Events struct {
						GnoEvent struct {
							Or []struct {
								PkgPath struct{ Eq string } `json:"pkg_path"`
							} `json:"_or"`
						}
					}
				}
			}
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Variables == nil {
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"latestBlockHeight": f.latest}})
		return
	}
	where := req.Variables.Where
	win := indexer.Window{From: where.BlockHeight.Gt, To: where.BlockHeight.Lt - 1}
	for _, o := range where.Response.Events.GnoEvent.Or {
		win.Paths = append(win.Paths, o.PkgPath.Eq)
	}
	f.windows = append(f.windows, win)
	if f.fail {
		json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]any{"message": "boom"}}})
		return
	}
	var txs []any
	for _, tx := range f.txs {
		if tx.height <= win.From || tx.height > win.To {
			continue
		}
		var events []any
		selected := false
		for _, e := range tx.events {
			selected = selected || slices.Contains(win.Paths, e.pkg)
			attrs := make([]any, 0, len(e.attrs))
			for _, a := range e.attrs {
				attrs = append(attrs, map[string]any{"key": a.Key, "value": a.Value})
			}
			events = append(events, map[string]any{"__typename": "GnoEvent", "type": e.typ, "pkg_path": e.pkg, "attrs": attrs})
		}
		if selected {
			txs = append(txs, map[string]any{"hash": tx.hash, "block_height": tx.height, "index": tx.index, "response": map[string]any{"events": events}})
		}
	}
	var blocks []any
	for h := win.From + 1; h <= win.To; h++ {
		t, ok := f.times[h]
		if !ok {
			t = now
		}
		blocks = append(blocks, map[string]any{"height": h, "time": t.Format(time.RFC3339Nano)})
	}
	json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"latestBlockHeight": f.latest, "getTransactions": txs, "getBlocks": blocks,
	}})
}

// ---- helpers

type harness struct {
	fake  *fakeIndexer
	store *store.Store
	wake  chan struct{}
	w     *Watcher
	log   *bytes.Buffer
}

func newHarness(t *testing.T, startHeight int64) *harness {
	t.Helper()
	fake := &fakeIndexer{times: map[int64]time.Time{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "gnotif.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	wake := make(chan struct{}, 1)
	var log bytes.Buffer
	w := New(Config{
		Source:      indexer.New(srv.URL, srv.Client()),
		Store:       st,
		Registry:    registry,
		StartHeight: startHeight,
		Poll:        10 * time.Millisecond,
		MaxAge:      10 * time.Minute,
		MaxWindow:   1000,
		Wake:        wake,
		Now:         func() time.Time { return now },
		Log:         slog.New(slog.NewTextHandler(&log, nil)),
	})
	return &harness{fake: fake, store: st, wake: wake, w: w, log: &log}
}

func (h *harness) setCursor(t *testing.T, c store.Cursor) {
	t.Helper()
	require.NoError(t, h.store.Update(context.Background(), func(tx *store.Tx) error { return tx.SetCursor(c) }))
}

func (h *harness) cursor(t *testing.T) store.Cursor {
	t.Helper()
	c, ok, err := h.store.Cursor(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	return c
}

func (h *harness) tick(t *testing.T) {
	t.Helper()
	require.NoError(t, h.w.Tick(context.Background()))
}

func (h *harness) putTrigger(t *testing.T, id, target string) {
	t.Helper()
	require.NoError(t, h.store.Update(context.Background(), func(tx *store.Tx) error {
		return tx.PutTrigger(trigger.Trigger{
			ID: id, Target: target, Event: "TurnPlayed", Param: "next", Declarer: "g1alice",
			Title: "Your turn", Body: "Game {game}, turn {turn}", Link: "/?game={game}",
		})
	}))
}

func (h *harness) optIn(t *testing.T, endpoint, triggerID, value string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.store.PutSubscription(ctx, store.Subscription{Endpoint: endpoint, P256dh: "p", Auth: "a"}))
	require.NoError(t, h.store.ReplaceOptins(ctx, endpoint, []store.Optin{{TriggerID: triggerID, Value: value}}))
}

func (h *harness) due(t *testing.T) []store.Delivery {
	t.Helper()
	d, err := h.store.Due(context.Background(), now.Add(time.Hour), 100)
	require.NoError(t, err)
	return d
}

func (h *harness) triggerIDs(t *testing.T) []string {
	t.Helper()
	ts, err := h.store.Triggers(context.Background())
	require.NoError(t, err)
	var ids []string
	for _, tr := range ts {
		ids = append(ids, tr.ID)
	}
	return ids
}

func declared(id, target string) fakeEvent {
	return fakeEvent{pkg: registry, typ: "TriggerDeclared", attrs: []trigger.Pair{
		{Key: "id", Value: id}, {Key: "target", Value: target}, {Key: "event", Value: "TurnPlayed"},
		{Key: "filter", Value: ""}, {Key: "param", Value: "next"}, {Key: "title", Value: "Your turn"},
		{Key: "body", Value: "Game {game}, turn {turn}"}, {Key: "link", Value: "/?game={game}"},
		{Key: "declarer", Value: "g1alice"}, {Key: "verified", Value: "false"},
	}}
}

func turn(gameID, next string) fakeEvent {
	return fakeEvent{pkg: game, typ: "TurnPlayed", attrs: []trigger.Pair{
		{Key: "game", Value: gameID}, {Key: "next", Value: next}, {Key: "turn", Value: "1"},
	}}
}

// ---- tests

func TestFirstTickSetsBound(t *testing.T) {
	h := newHarness(t, 5)
	h.fake.set(func(f *fakeIndexer) { f.latest = 9 })
	h.tick(t)
	assert.Equal(t, store.Cursor{HeightDone: 4, Bound: 9}, h.cursor(t))
	assert.Empty(t, h.fake.recorded())
}

func TestNoStartHeight(t *testing.T) {
	h := newHarness(t, 0)
	assert.ErrorIs(t, h.w.Tick(context.Background()), ErrNoStartHeight)
}

func TestWindowBoundedByPreviousHeight(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 4, Bound: 9})
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	h.fake.add(9, declared("0000001", game))
	h.fake.add(11, declared("0000002", game))

	h.tick(t)
	assert.Equal(t, []string{"0000001"}, h.triggerIDs(t))
	assert.Equal(t, store.Cursor{HeightDone: 9, Bound: 12}, h.cursor(t))

	h.tick(t)
	assert.Equal(t, []string{"0000001", "0000002"}, h.triggerIDs(t))
	assert.Equal(t, store.Cursor{HeightDone: 12, Bound: 12}, h.cursor(t))

	h.tick(t)
	assert.Len(t, h.fake.recorded(), 2)
	assert.Equal(t, store.Cursor{HeightDone: 12, Bound: 12}, h.cursor(t))
}

func TestDeclareThenNotify(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 0, Bound: 5})
	h.fake.set(func(f *fakeIndexer) { f.latest = 8 })
	h.fake.add(3, declared("0000001", game))
	h.tick(t)
	require.Equal(t, []string{"0000001"}, h.triggerIDs(t))

	h.optIn(t, "E", "0000001", "g1bob")
	h.fake.add(7, turn("7", "g1bob"), turn("8", "g1alice"))
	h.tick(t)

	due := h.due(t)
	require.Len(t, due, 1)
	assert.Equal(t, "E", due[0].Subscription.Endpoint)
	assert.Equal(t, trigger.Notification{Title: "Your turn", Body: "Game 7, turn 1", Link: "/?game=7"}, due[0].Notification)
	select {
	case <-h.wake:
	default:
		t.Fatal("no wake signal after queueing a push")
	}
}

func TestPathsAreRegistryAndTargets(t *testing.T) {
	h := newHarness(t, 1)
	h.putTrigger(t, "0000001", "gno.land/r/demo/b")
	h.putTrigger(t, "0000002", "gno.land/r/demo/a")
	h.setCursor(t, store.Cursor{HeightDone: 0, Bound: 5})
	h.fake.set(func(f *fakeIndexer) { f.latest = 5 })
	h.tick(t)
	windows := h.fake.recorded()
	require.Len(t, windows, 1)
	assert.Equal(t, []string{registry, "gno.land/r/demo/a", "gno.land/r/demo/b"}, windows[0].Paths)
}

func TestRemovedTriggerStopsMatching(t *testing.T) {
	h := newHarness(t, 1)
	h.putTrigger(t, "0000001", game)
	h.optIn(t, "E", "0000001", "g1bob")
	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	h.fake.add(10, fakeEvent{pkg: registry, typ: "TriggerRemoved", attrs: []trigger.Pair{{Key: "id", Value: "0000001"}}})
	h.fake.add(11, turn("7", "g1bob"))
	h.tick(t)
	assert.Empty(t, h.due(t))
	assert.Empty(t, h.triggerIDs(t))
}

func TestReplayIsHarmless(t *testing.T) {
	h := newHarness(t, 1)
	h.putTrigger(t, "0000001", game)
	h.optIn(t, "E", "0000001", "g1bob")
	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	h.fake.add(11, turn("7", "g1bob"))
	h.tick(t)
	require.Len(t, h.due(t), 1)

	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.tick(t)
	assert.Len(t, h.due(t), 1)
}

func TestOldEventsSkipped(t *testing.T) {
	h := newHarness(t, 1)
	h.putTrigger(t, "0000001", game)
	h.optIn(t, "E", "0000001", "g1bob")
	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.fake.set(func(f *fakeIndexer) {
		f.latest = 12
		f.times[11] = now.Add(-11 * time.Minute)
	})
	h.fake.add(11, turn("7", "g1bob"))
	h.tick(t)
	assert.Empty(t, h.due(t))
	assert.Equal(t, int64(12), h.cursor(t).HeightDone)
	// A chain that makes no empty blocks stamps an event with the previous
	// block's time, so a skip must be visible to the operator.
	assert.Contains(t, h.log.String(), `msg="skipped stale events" count=1`)
}

func TestCatchUpWindowCapped(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 0, Bound: 5000})
	h.fake.set(func(f *fakeIndexer) { f.latest = 5000 })
	h.tick(t)
	windows := h.fake.recorded()
	require.Len(t, windows, 1)
	assert.Equal(t, int64(0), windows[0].From)
	assert.Equal(t, int64(1000), windows[0].To)
}

func TestFailedTickHalvesWindow(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 0, Bound: 5000})
	h.fake.set(func(f *fakeIndexer) {
		f.latest = 5000
		f.fail = true
	})
	require.Error(t, h.w.Tick(context.Background()))
	assert.Equal(t, store.Cursor{HeightDone: 0, Bound: 5000}, h.cursor(t))

	h.fake.set(func(f *fakeIndexer) { f.fail = false })
	h.tick(t)
	h.tick(t)
	windows := h.fake.recorded()
	require.Len(t, windows, 3)
	assert.Equal(t, indexer.Window{From: 0, To: 1000}, indexer.Window{From: windows[0].From, To: windows[0].To})
	assert.Equal(t, indexer.Window{From: 0, To: 500}, indexer.Window{From: windows[1].From, To: windows[1].To})
	assert.Equal(t, indexer.Window{From: 500, To: 1500}, indexer.Window{From: windows[2].From, To: windows[2].To})
}

func TestMalformedDeclaredSkipped(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	bad := declared("0000001", game)
	bad.attrs = bad.attrs[1:]
	h.fake.add(10, bad)
	h.fake.add(11, declared("0000002", game))
	h.tick(t)
	assert.Equal(t, []string{"0000002"}, h.triggerIDs(t))
	assert.Equal(t, int64(12), h.cursor(t).HeightDone)
}

func TestIndexerBehindBound(t *testing.T) {
	h := newHarness(t, 1)
	h.putTrigger(t, "0000001", game)
	h.optIn(t, "E", "0000001", "g1bob")
	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.fake.set(func(f *fakeIndexer) { f.latest = 10 })
	h.tick(t)
	assert.Equal(t, store.Cursor{HeightDone: 9, Bound: 10}, h.cursor(t), "the cursor must not pass the indexer's height")

	h.fake.add(11, turn("7", "g1bob"))
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	h.tick(t)
	h.tick(t)
	assert.Equal(t, int64(12), h.cursor(t).HeightDone)
	assert.Len(t, h.due(t), 1)
}

func TestRunStopsOnCancel(t *testing.T) {
	h := newHarness(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.w.Run(ctx) }()
	cancel()
	var err error
	require.Eventually(t, func() bool {
		select {
		case err = <-done:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond)
	assert.NoError(t, err)
}
