package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/trigger"
)

func TestSubscribersStatementReadsOptedIn(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		seedTrigger(t, s, "t1", "next")
		subscribe(t, s, "E1", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
		subscribe(t, s, "E2", "K2", Optin{TriggerID: "t1", Value: "g1bob"})
		subscribe(t, s, "E3", "K3", Optin{TriggerID: "t1", Value: "g1alice"})

		got, err := SubscribersOf(exec(t, s, SubscribersStatement("t1", "g1bob"))[0])
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Less(t, got[0].ID, got[1].ID)
		assert.Equal(t, Subscription{Endpoint: "E1", P256dh: "K1-p", Auth: "K1-a"}, got[0].Subscription)
		assert.Equal(t, Subscription{Endpoint: "E2", P256dh: "K2-p", Auth: "K2-a"}, got[1].Subscription)
	})
}

func TestSubscribersReadBeforeRemoval(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		seedTrigger(t, s, "t1", "next")
		subscribe(t, s, "E1", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
		subscribe(t, s, "E2", "K2", Optin{TriggerID: "t1", Value: "g1bob"})

		res := exec(t, s, SubscribersStatement("t1", "g1bob"), DeleteTriggerStatement("t1"))
		got, err := SubscribersOf(res[0])
		require.NoError(t, err)
		assert.Len(t, got, 2)
		assert.Empty(t, optedIn(t, s, "t1", "g1bob"))
	})
}

func TestSubscribersOfRejectsMalformedRow(t *testing.T) {
	_, err := SubscribersOf(Result{Rows: []map[string]any{{"id": "1"}}})
	require.Error(t, err)
}

// queueFixture seeds a trigger with one subscriber and returns that
// subscriber's id.
func queueFixture(t *testing.T, s *Store) int64 {
	t.Helper()
	seedTrigger(t, s, "t1", "next")
	subscribe(t, s, "E1", "K1", Optin{TriggerID: "t1", Value: "g1bob"})
	got, err := SubscribersOf(exec(t, s, SubscribersStatement("t1", "g1bob"))[0])
	require.NoError(t, err)
	require.Len(t, got, 1)
	return got[0].ID
}

func TestQueueIsIdempotent(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		ctx := context.Background()
		s := fresh(t)
		p := Pending{Push: push(queueFixture(t, s), "h1"), Attempts: 3, NextAttempt: now}

		require.NoError(t, s.Queue(ctx, []Pending{p}))
		require.NoError(t, s.Queue(ctx, []Pending{p}))

		due, err := s.Due(ctx, now, 10)
		require.NoError(t, err)
		require.Len(t, due, 1)
		assert.Equal(t, 3, due[0].Attempts)
		assert.Equal(t, p.Notification, due[0].Notification)
		assert.Equal(t, p.SubscriptionID, due[0].SubscriptionID)
	})
}

func TestQueueDueAtNextAttempt(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		ctx := context.Background()
		s := fresh(t)
		p := Pending{Push: push(queueFixture(t, s), "h1"), Attempts: 1, NextAttempt: now.Add(time.Minute)}

		require.NoError(t, s.Queue(ctx, []Pending{p}))

		due, err := s.Due(ctx, now, 10)
		require.NoError(t, err)
		assert.Empty(t, due)
		due, err = s.Due(ctx, p.NextAttempt, 10)
		require.NoError(t, err)
		assert.Len(t, due, 1)
	})
}

func TestQueueSplitsLargeLists(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		sub := queueFixture(t, s)
		const n = 2500
		pending := make([]Pending, n)
		for i := range pending {
			pending[i] = Pending{Push: push(sub, fmt.Sprintf("h%d", i)), NextAttempt: now}
		}

		require.NoError(t, s.Queue(context.Background(), pending))

		res := exec(t, s, Statement{SQL: `SELECT count(*) AS n FROM outbox`})
		var count int64
		require.NoError(t, column(res[0].Rows[0], "n", &count))
		assert.EqualValues(t, n, count)
	})
}

// A subscription removed after the watcher read it must not cost the other
// pushes written in the same batch.
func TestQueueSkipsRemovedSubscription(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		ctx := context.Background()
		s := fresh(t)
		live := queueFixture(t, s)
		subscribe(t, s, "E2", "K2")
		var removed int64
		res := exec(t, s, Statement{`SELECT id FROM subscriptions WHERE endpoint = 'E2'`, nil})
		require.NoError(t, column(res[0].Rows[0], "id", &removed))
		require.NoError(t, s.DeleteSubscription(ctx, "E2"))

		require.NoError(t, s.Queue(ctx, []Pending{
			{Push: push(removed, "h1"), NextAttempt: now},
			{Push: push(live, "h2"), NextAttempt: now},
		}))

		res = exec(t, s, Statement{SQL: `SELECT subscription_id, tx_hash FROM outbox`})
		assert.Equal(t, []map[string]any{{"subscription_id": live, "tx_hash": "h2"}}, res[0].Rows)
	})
}

func TestTriggersLoadsAllInOrder(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		ctx := context.Background()
		s := fresh(t)
		mk := func(id, target, event string) trigger.Trigger {
			return trigger.Trigger{
				ID: id, Target: target, Event: event, Param: "next",
				Filter: []trigger.Pair{{Key: "mode", Value: "ranked"}},
				Title:  "Your turn", Body: "Game {game}", Link: "/", Declarer: "g1alice", Verified: true,
			}
		}
		stored := []trigger.Trigger{
			mk("t1", "gno.land/r/a", "A"), mk("t2", "gno.land/r/a", "A"),
			mk("t3", "gno.land/r/a", "B"), mk("t4", "gno.land/r/b", "A"),
		}
		for _, i := range []int{3, 2, 1, 0} {
			exec(t, s, PutTriggerStatement(stored[i]))
		}

		got, err := s.Triggers(ctx)
		require.NoError(t, err)
		assert.Equal(t, stored, got)
	})
}

func TestCursorStatementStoresCursor(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		want := Cursor{HeightDone: 10, Bound: 12, NextTx: 3}
		exec(t, s, CursorStatement(Cursor{HeightDone: 1}))
		exec(t, s, CursorStatement(want))
		got, ok, err := s.Cursor(context.Background())
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, want, got)
	})
}
