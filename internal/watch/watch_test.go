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
	"strings"
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

// The indexer's error for a query over its element limit, which the client
// reads as ErrTooLarge, and another, which it reads as ErrQuery.
const (
	maxElements = "max elements per query reached (10000)"
	boom        = "boom"
)

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

// txAt names a transaction by its block height and its index in the block.
type txAt struct {
	height int64
	index  int
}

type fakeIndexer struct {
	mu         sync.Mutex
	latest     int64
	txs        []fakeTx
	times      map[int64]time.Time
	fail       bool                      // every window answers boom
	windowErrs map[indexer.Window]string // the error one window answers
	txErrs     map[txAt]string           // the error one transaction answers
	windows    []indexer.Window
	fetched    []txAt
}

func (f *fakeIndexer) add(height int64, events ...fakeEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := 0
	for _, tx := range f.txs {
		if tx.height == height {
			index++
		}
	}
	f.txs = append(f.txs, fakeTx{height: height, index: index, hash: "tx" + string(rune('a'+len(f.txs))), events: events})
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

func (f *fakeIndexer) fetchedTxs() []txAt {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.fetched)
}

func (f *fakeIndexer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct {
		Variables *struct {
			Where struct {
				BlockHeight struct{ Gt, Lt, Eq *int64 } `json:"block_height"`
				Index       *struct{ Eq int }           `json:"index"`
			}
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Variables == nil {
		reply(w, map[string]any{"latestBlockHeight": f.latest})
		return
	}
	where := req.Variables.Where
	switch {
	case where.Index != nil:
		f.serveTx(w, txAt{height: *where.BlockHeight.Eq, index: where.Index.Eq})
	case where.BlockHeight.Eq != nil:
		f.serveBlock(w, *where.BlockHeight.Eq)
	default:
		f.serveWindow(w, indexer.Window{From: *where.BlockHeight.Gt, To: *where.BlockHeight.Lt - 1})
	}
}

func (f *fakeIndexer) serveWindow(w http.ResponseWriter, win indexer.Window) {
	f.windows = append(f.windows, win)
	if f.fail {
		replyError(w, boom)
		return
	}
	if msg, ok := f.windowErrs[win]; ok {
		replyError(w, msg)
		return
	}
	var txs []any
	for _, tx := range f.txs {
		if tx.height > win.From && tx.height <= win.To && len(tx.events) > 0 {
			txs = append(txs, tx.encode())
		}
	}
	var blocks []any
	for h := win.From + 1; h <= win.To; h++ {
		blocks = append(blocks, f.block(h))
	}
	reply(w, map[string]any{"latestBlockHeight": f.latest, "getTransactions": txs, "getBlocks": blocks})
}

func (f *fakeIndexer) serveBlock(w http.ResponseWriter, height int64) {
	var txs []any
	for _, tx := range f.txs {
		if tx.height == height && len(tx.events) > 0 {
			txs = append(txs, map[string]any{"hash": tx.hash, "index": tx.index})
		}
	}
	reply(w, map[string]any{"getTransactions": txs, "getBlocks": []any{f.block(height)}})
}

func (f *fakeIndexer) serveTx(w http.ResponseWriter, at txAt) {
	f.fetched = append(f.fetched, at)
	if msg, ok := f.txErrs[at]; ok {
		replyError(w, msg)
		return
	}
	var txs []any
	for _, tx := range f.txs {
		if tx.height == at.height && tx.index == at.index {
			txs = append(txs, tx.encode())
		}
	}
	reply(w, map[string]any{"latestBlockHeight": f.latest, "getTransactions": txs})
}

func (f *fakeIndexer) block(height int64) map[string]any {
	t, ok := f.times[height]
	if !ok {
		t = now
	}
	return map[string]any{"height": height, "time": t.Format(time.RFC3339Nano)}
}

func (tx fakeTx) encode() map[string]any {
	events := make([]any, 0, len(tx.events))
	for _, e := range tx.events {
		attrs := make([]any, 0, len(e.attrs))
		for _, a := range e.attrs {
			attrs = append(attrs, map[string]any{"key": a.Key, "value": a.Value})
		}
		events = append(events, map[string]any{"__typename": "GnoEvent", "type": e.typ, "pkg_path": e.pkg, "attrs": attrs})
	}
	return map[string]any{"hash": tx.hash, "block_height": tx.height, "index": tx.index, "response": map[string]any{"events": events}}
}

func reply(w http.ResponseWriter, data map[string]any) {
	json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func replyError(w http.ResponseWriter, msg string) {
	json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]any{"message": msg}}})
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
	fake := &fakeIndexer{times: map[int64]time.Time{}, windowErrs: map[indexer.Window]string{}, txErrs: map[txAt]string{}}
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

// bodies returns the bodies of the queued pushes, in the order they were queued.
func (h *harness) bodies(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, d := range h.due(t) {
		out = append(out, d.Notification.Body)
	}
	return out
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
		{Key: "declarer", Value: "g1alice"}, {Key: "verified", Value: "true"},
	}}
}

// unverified is declared by a caller other than the target realm.
func unverified(id, target string) fakeEvent {
	e := declared(id, target)
	i := slices.IndexFunc(e.attrs, func(p trigger.Pair) bool { return p.Key == "verified" })
	e.attrs[i].Value = "false"
	return e
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
	assert.Contains(t, h.log.String(), `msg="indexer behind stored bound" latest=10 bound=12`)

	h.fake.add(11, turn("7", "g1bob"))
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	h.tick(t)
	h.tick(t)
	assert.Equal(t, int64(12), h.cursor(t).HeightDone)
	assert.Len(t, h.due(t), 1)

	// An indexer that re-synced from scratch stays below height_done for
	// several ticks, and every one of them must warn.
	const warning = `msg="indexer behind stored bound"`
	h.log.Reset()
	h.setCursor(t, store.Cursor{HeightDone: 100, Bound: 120})
	h.fake.set(func(f *fakeIndexer) { f.latest = 5 })
	for i := 1; i <= 3; i++ {
		h.tick(t)
		assert.Equal(t, i, strings.Count(h.log.String(), warning), "warnings after tick %d", i)
		assert.Equal(t, int64(100), h.cursor(t).HeightDone)
	}
	assert.Contains(t, h.log.String(), warning+" latest=5 height_done=100")

	h.fake.set(func(f *fakeIndexer) { f.latest = 130 })
	h.tick(t)
	assert.Equal(t, 3, strings.Count(h.log.String(), warning), "no warning once the indexer is past height_done")
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

func TestUnverifiedIgnored(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	h.fake.add(10, unverified("0000001", game))
	h.fake.add(11, declared("0000002", game))
	h.tick(t)
	assert.Equal(t, []string{"0000002"}, h.triggerIDs(t))
	assert.Equal(t, store.Cursor{HeightDone: 12, Bound: 12}, h.cursor(t))
}

func TestRemoveUnknownTrigger(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 9, Bound: 12})
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	h.fake.add(10, fakeEvent{pkg: registry, typ: "TriggerRemoved", attrs: []trigger.Pair{{Key: "id", Value: "0000099"}}})
	h.fake.add(11, declared("0000001", game))
	h.tick(t)
	assert.Equal(t, []string{"0000001"}, h.triggerIDs(t))
	assert.Equal(t, store.Cursor{HeightDone: 12, Bound: 12}, h.cursor(t))
}

func TestMatchingPerEvent(t *testing.T) {
	const other = "gno.land/r/demo/other"
	triggers := []trigger.Trigger{
		{ID: "any", Target: game, Event: "TurnPlayed"},
		{ID: "over", Target: game, Event: "GameOver"},
		{ID: "ranked", Target: game, Event: "TurnPlayed", Filter: []trigger.Pair{{Key: "mode", Value: "ranked"}}},
		{ID: "elsewhere", Target: other, Event: "TurnPlayed"},
	}
	cases := map[string]struct {
		event fakeEvent
		want  []string
	}{
		"filter fails":           {fakeEvent{pkg: game, typ: "TurnPlayed", attrs: []trigger.Pair{{Key: "mode", Value: "casual"}}}, []string{"any"}},
		"filter passes":          {fakeEvent{pkg: game, typ: "TurnPlayed", attrs: []trigger.Pair{{Key: "mode", Value: "ranked"}}}, []string{"any", "ranked"}},
		"other event":            {fakeEvent{pkg: game, typ: "GameOver"}, []string{"over"}},
		"other realm":            {fakeEvent{pkg: other, typ: "TurnPlayed"}, []string{"elsewhere"}},
		"realm without triggers": {fakeEvent{pkg: "gno.land/r/demo/none", typ: "TurnPlayed", attrs: []trigger.Pair{{Key: "mode", Value: "ranked"}}}, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, 1)
			for _, tr := range triggers {
				tr.Title, tr.Body, tr.Link, tr.Declarer, tr.Verified = "T", tr.ID, "/", "g1alice", true
				require.NoError(t, h.store.Update(context.Background(), func(tx *store.Tx) error { return tx.PutTrigger(tr) }))
				h.optIn(t, "E-"+tr.ID, tr.ID, "")
			}
			h.setCursor(t, store.Cursor{HeightDone: 10, Bound: 11})
			h.fake.set(func(f *fakeIndexer) { f.latest = 11 })
			h.fake.add(11, tc.event)
			h.tick(t)
			assert.ElementsMatch(t, tc.want, h.bodies(t))
			assert.Equal(t, int64(11), h.cursor(t).HeightDone)
		})
	}
}

func TestWindowDoubles(t *testing.T) {
	h := newHarness(t, 1)
	h.setCursor(t, store.Cursor{HeightDone: 0, Bound: 100_000})
	h.fake.set(func(f *fakeIndexer) {
		f.latest = 100_000
		f.fail = true
	})
	for range 3 {
		require.ErrorIs(t, h.w.Tick(context.Background()), indexer.ErrQuery)
	}
	h.fake.set(func(f *fakeIndexer) { f.fail = false })
	for range 5 {
		h.tick(t)
	}
	var sizes []int64
	for _, w := range h.fake.recorded() {
		sizes = append(sizes, w.To-w.From)
	}
	assert.Equal(t, []int64{1000, 500, 250, 125, 250, 500, 1000, 1000}, sizes)
}

// oversizedBlock stores a trigger g1bob opted in to and puts three turns for
// g1bob in block 11, the only block of the next window, which is too large.
func oversizedBlock(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, 1)
	h.putTrigger(t, "0000001", game)
	h.optIn(t, "E", "0000001", "g1bob")
	h.setCursor(t, store.Cursor{HeightDone: 10, Bound: 11})
	h.fake.set(func(f *fakeIndexer) {
		f.latest = 12
		f.windowErrs[indexer.Window{From: 10, To: 11}] = maxElements
	})
	h.fake.add(11, turn("7", "g1bob"))
	h.fake.add(11, turn("8", "g1bob"))
	h.fake.add(11, turn("9", "g1bob"))
	return h
}

func TestSkipsOversizedTransaction(t *testing.T) {
	h := oversizedBlock(t)
	h.fake.set(func(f *fakeIndexer) { f.txErrs[txAt{height: 11, index: 1}] = maxElements })
	h.tick(t)

	assert.Equal(t, []string{"Game 7, turn 1", "Game 9, turn 1"}, h.bodies(t))
	assert.Equal(t, []txAt{{11, 0}, {11, 1}, {11, 2}}, h.fake.fetchedTxs())
	assert.Equal(t, 1, strings.Count(h.log.String(), "level=ERROR"))
	assert.Contains(t, h.log.String(), `level=ERROR msg="skip oversized transaction" height=11 index=1 tx=txb`)
	assert.Equal(t, store.Cursor{HeightDone: 11, Bound: 11}, h.cursor(t))
}

func TestResumesInsideBlock(t *testing.T) {
	h := newHarness(t, 1)
	h.putTrigger(t, "0000001", game)
	h.optIn(t, "E", "0000001", "g1bob")
	h.setCursor(t, store.Cursor{HeightDone: 10, Bound: 12, NextTx: 2})
	h.fake.set(func(f *fakeIndexer) { f.latest = 12 })
	for _, g := range []string{"1", "2", "3", "4"} {
		h.fake.add(11, turn(g, "g1bob"))
	}
	h.tick(t)

	assert.Equal(t, []txAt{{11, 2}, {11, 3}}, h.fake.fetchedTxs())
	assert.Empty(t, h.fake.recorded())
	assert.Equal(t, []string{"Game 3, turn 1", "Game 4, turn 1"}, h.bodies(t))
	assert.Equal(t, store.Cursor{HeightDone: 11, Bound: 12}, h.cursor(t))

	h.tick(t)
	assert.Equal(t, []indexer.Window{{From: 11, To: 12}}, h.fake.recorded())
}

func TestTransientNeverSkips(t *testing.T) {
	h := oversizedBlock(t)
	h.fake.set(func(f *fakeIndexer) { f.txErrs[txAt{height: 11, index: 1}] = boom })
	require.ErrorIs(t, h.w.Tick(context.Background()), indexer.ErrQuery)
	assert.Equal(t, store.Cursor{HeightDone: 10, Bound: 11, NextTx: 1}, h.cursor(t))
	assert.Equal(t, []string{"Game 7, turn 1"}, h.bodies(t))

	h.fake.set(func(f *fakeIndexer) { delete(f.txErrs, txAt{height: 11, index: 1}) })
	h.tick(t)
	assert.Equal(t, []string{"Game 7, turn 1", "Game 8, turn 1", "Game 9, turn 1"}, h.bodies(t))
	assert.Equal(t, []txAt{{11, 0}, {11, 1}, {11, 1}, {11, 2}}, h.fake.fetchedTxs())
	assert.Equal(t, store.Cursor{HeightDone: 11, Bound: 11}, h.cursor(t))
	assert.NotContains(t, h.log.String(), "level=ERROR")
}
