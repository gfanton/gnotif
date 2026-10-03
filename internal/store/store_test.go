package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/trigger"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func fresh(t *testing.T) *Store {
	t.Helper()
	return open(t, filepath.Join(t.TempDir(), "gnotif.db"))
}

func putTrigger(t *testing.T, s *Store, id, param string) {
	t.Helper()
	require.NoError(t, s.Update(context.Background(), func(tx *Tx) error {
		return tx.PutTrigger(trigger.Trigger{
			ID: id, Target: "gno.land/r/demo/game", Event: "TurnPlayed", Param: param,
			Title: "Your turn", Body: "Game {game}", Link: "/", Declarer: "g1alice",
			Filter: []trigger.Pair{{Key: "mode", Value: "ranked"}},
		})
	}))
}

func subscribe(t *testing.T, s *Store, endpoint, keys string, optins ...Optin) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.PutSubscription(ctx, Subscription{Endpoint: endpoint, P256dh: keys + "-p", Auth: keys + "-a"}))
	if len(optins) > 0 {
		require.NoError(t, s.ReplaceOptins(ctx, endpoint, optins))
	}
}

func subscribers(t *testing.T, s *Store, triggerID, value string) []int64 {
	t.Helper()
	var ids []int64
	require.NoError(t, s.Update(context.Background(), func(tx *Tx) error {
		var err error
		ids, err = tx.Subscribers(triggerID, value)
		return err
	}))
	return ids
}

func enqueue(t *testing.T, s *Store, p Push) bool {
	t.Helper()
	var added bool
	require.NoError(t, s.Update(context.Background(), func(tx *Tx) error {
		var err error
		added, err = tx.Enqueue(p)
		return err
	}))
	return added
}

func push(subID int64, hash string) Push {
	return Push{
		SubscriptionID: subID, TriggerID: "t1", TxHash: hash, EventIndex: 0,
		Notification: trigger.Notification{Title: "Your turn", Body: "Game 7", Link: "/?game=7"},
		CreatedAt:    now,
	}
}

func TestCursorRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "gnotif.db")
	s, err := Open(ctx, path)
	require.NoError(t, err)

	_, ok, err := s.Cursor(ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, s.Update(ctx, func(tx *Tx) error { return tx.SetCursor(Cursor{HeightDone: 10, Bound: 20}) }))
	c, ok, err := s.Cursor(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, Cursor{HeightDone: 10, Bound: 20}, c)
	require.NoError(t, s.Close())

	reopened := open(t, path)
	c, ok, err = reopened.Cursor(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, Cursor{HeightDone: 10, Bound: 20}, c)
}

func TestUpdateRollsBack(t *testing.T) {
	ctx := context.Background()
	s := fresh(t)
	boom := errors.New("boom")
	err := s.Update(ctx, func(tx *Tx) error {
		require.NoError(t, tx.PutTrigger(trigger.Trigger{ID: "t1", Target: "gno.land/r/demo/game", Event: "E", Title: "T", Link: "/", Declarer: "g1"}))
		return boom
	})
	assert.ErrorIs(t, err, boom)
	triggers, err := s.Triggers(ctx)
	require.NoError(t, err)
	assert.Empty(t, triggers)
}

func TestTriggersRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	got, err := s.Triggers(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []trigger.Pair{{Key: "mode", Value: "ranked"}}, got[0].Filter)
	assert.Equal(t, "next", got[0].Param)

	var targets []string
	require.NoError(t, s.Update(ctx, func(tx *Tx) error {
		var err error
		targets, err = tx.Targets()
		return err
	}))
	assert.Equal(t, []string{"gno.land/r/demo/game"}, targets)
}

func TestEnqueueIdempotent(t *testing.T) {
	ctx := context.Background()
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	sub := subscribers(t, s, "t1", "g1bob")[0]

	assert.True(t, enqueue(t, s, push(sub, "h1")))
	assert.False(t, enqueue(t, s, push(sub, "h1")))
	due, err := s.Due(ctx, now, 10)
	require.NoError(t, err)
	assert.Len(t, due, 1)
}

func TestPutTriggerAgainKeepsOptins(t *testing.T) {
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	putTrigger(t, s, "t1", "next")
	assert.Len(t, subscribers(t, s, "t1", "g1bob"), 1)
}

func TestDeleteTriggerCascades(t *testing.T) {
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	require.NotEmpty(t, subscribers(t, s, "t1", "g1bob"))

	require.NoError(t, s.Update(context.Background(), func(tx *Tx) error { return tx.DeleteTrigger("t1") }))
	assert.Empty(t, subscribers(t, s, "t1", "g1bob"))
}

func TestPutSubscriptionKeepsOptins(t *testing.T) {
	ctx := context.Background()
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	subscribe(t, s, "E", "K2")

	ids := subscribers(t, s, "t1", "g1bob")
	require.Len(t, ids, 1)
	enqueue(t, s, push(ids[0], "h1"))
	due, err := s.Due(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, Subscription{Endpoint: "E", P256dh: "K2-p", Auth: "K2-a"}, due[0].Subscription)
}

func TestReplaceOptins(t *testing.T) {
	ctx := context.Background()
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	putTrigger(t, s, "t2", "")
	subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	require.NoError(t, s.ReplaceOptins(ctx, "E", []Optin{{TriggerID: "t2", Value: ""}}))

	assert.Empty(t, subscribers(t, s, "t1", "g1bob"))
	assert.Len(t, subscribers(t, s, "t2", ""), 1)
	assert.ErrorIs(t, s.ReplaceOptins(ctx, "unknown", nil), ErrUnknownSubscription)
	assert.ErrorIs(t, s.DeleteSubscription(ctx, "unknown"), ErrUnknownSubscription)
}

func TestSubscribers(t *testing.T) {
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	putTrigger(t, s, "t2", "")
	subscribe(t, s, "A", "KA", Optin{TriggerID: "t1", Value: "g1a"}, Optin{TriggerID: "t2", Value: ""})
	subscribe(t, s, "B", "KB", Optin{TriggerID: "t1", Value: "g1b"}, Optin{TriggerID: "t2", Value: ""})

	onlyA := subscribers(t, s, "t1", "g1a")
	require.Len(t, onlyA, 1)
	assert.NotContains(t, subscribers(t, s, "t1", "g1b"), onlyA[0])
	assert.Len(t, subscribers(t, s, "t2", ""), 2)
}

func TestDueAndReschedule(t *testing.T) {
	ctx := context.Background()
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	sub := subscribers(t, s, "t1", "g1bob")[0]
	for _, h := range []string{"h1", "h2", "h3"} {
		enqueue(t, s, push(sub, h))
	}

	due, err := s.Due(ctx, now, 2)
	require.NoError(t, err)
	require.Len(t, due, 2)
	assert.Less(t, due[0].ID, due[1].ID)
	assert.Equal(t, sub, due[0].SubscriptionID)
	assert.Equal(t, "Your turn", due[0].Notification.Title)
	assert.True(t, now.Equal(due[0].CreatedAt))
	first := due[0].ID

	require.NoError(t, s.Reschedule(ctx, first, now.Add(time.Minute)))
	due, err = s.Due(ctx, now, 10)
	require.NoError(t, err)
	assert.Len(t, due, 2)
	for _, d := range due {
		assert.NotEqual(t, first, d.ID)
	}
	later, err := s.Due(ctx, now.Add(time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, later, 3)
	assert.Equal(t, 1, later[0].Attempts)

	require.NoError(t, s.DeleteDelivery(ctx, first))
	due, err = s.Due(ctx, now.Add(time.Minute), 10)
	require.NoError(t, err)
	assert.Len(t, due, 2)
}

func TestRemoveSubscriptionCascades(t *testing.T) {
	ctx := context.Background()
	s := fresh(t)
	putTrigger(t, s, "t1", "next")
	subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	sub := subscribers(t, s, "t1", "g1bob")[0]
	enqueue(t, s, push(sub, "h1"))

	require.NoError(t, s.RemoveSubscription(ctx, sub))
	assert.Empty(t, subscribers(t, s, "t1", "g1bob"))
	due, err := s.Due(ctx, now, 10)
	require.NoError(t, err)
	assert.Empty(t, due)
}

func TestMoveSubscription(t *testing.T) {
	ctx := context.Background()
	t.Run("opt-ins and pushes follow the new endpoint", func(t *testing.T) {
		s := fresh(t)
		putTrigger(t, s, "t1", "next")
		subscribe(t, s, "OLD", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
		sub := subscribers(t, s, "t1", "g1bob")[0]
		enqueue(t, s, push(sub, "h1"))

		require.NoError(t, s.MoveSubscription(ctx, "OLD", Subscription{Endpoint: "NEW", P256dh: "K2-p", Auth: "K2-a"}))
		assert.Equal(t, []int64{sub}, subscribers(t, s, "t1", "g1bob"))
		due, err := s.Due(ctx, now, 10)
		require.NoError(t, err)
		require.Len(t, due, 1)
		assert.Equal(t, Subscription{Endpoint: "NEW", P256dh: "K2-p", Auth: "K2-a"}, due[0].Subscription)
		assert.ErrorIs(t, s.DeleteSubscription(ctx, "OLD"), ErrUnknownSubscription)
	})
	t.Run("unknown old endpoint stores the new one", func(t *testing.T) {
		s := fresh(t)
		require.NoError(t, s.MoveSubscription(ctx, "GONE", Subscription{Endpoint: "NEW", P256dh: "K2-p", Auth: "K2-a"}))
		require.NoError(t, s.DeleteSubscription(ctx, "NEW"))
	})
	t.Run("new endpoint already stored keeps both", func(t *testing.T) {
		s := fresh(t)
		putTrigger(t, s, "t1", "next")
		subscribe(t, s, "OLD", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
		subscribe(t, s, "NEW", "K2", Optin{TriggerID: "t1", Value: "g1carol"})

		require.NoError(t, s.MoveSubscription(ctx, "OLD", Subscription{Endpoint: "NEW", P256dh: "K3-p", Auth: "K3-a"}))
		assert.Len(t, subscribers(t, s, "t1", "g1bob"), 1)
		assert.Len(t, subscribers(t, s, "t1", "g1carol"), 1)
	})
}

// Delivery deletes rows by an id read at the start of a batch, so an id
// must never name a different row later.
func TestIDsNotReused(t *testing.T) {
	ctx := context.Background()
	t.Run("subscription", func(t *testing.T) {
		s := fresh(t)
		putTrigger(t, s, "t1", "next")
		subscribe(t, s, "B", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
		old := subscribers(t, s, "t1", "g1bob")[0]
		require.NoError(t, s.DeleteSubscription(ctx, "B"))

		subscribe(t, s, "C", "K2", Optin{TriggerID: "t1", Value: "g1carol"})
		assert.NotEqual(t, old, subscribers(t, s, "t1", "g1carol")[0])
	})
	t.Run("outbox", func(t *testing.T) {
		s := fresh(t)
		putTrigger(t, s, "t1", "next")
		subscribe(t, s, "E", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
		sub := subscribers(t, s, "t1", "g1bob")[0]
		enqueue(t, s, push(sub, "h1"))
		due, err := s.Due(ctx, now, 10)
		require.NoError(t, err)
		require.Len(t, due, 1)
		require.NoError(t, s.DeleteDelivery(ctx, due[0].ID))

		enqueue(t, s, push(sub, "h2"))
		due2, err := s.Due(ctx, now, 10)
		require.NoError(t, err)
		require.Len(t, due2, 1)
		assert.NotEqual(t, due[0].ID, due2[0].ID)
	})
}
