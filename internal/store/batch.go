package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/gfanton/gnotif/internal/trigger"
)

// PutTriggerStatement stores t, updating the trigger with the same id in
// place so its opt-ins survive.
func PutTriggerStatement(t trigger.Trigger) Statement {
	var verified int64
	if t.Verified {
		verified = 1
	}
	return Statement{`
		INSERT INTO triggers (id, target, event, filter, param, title, body, link, declarer, verified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			target = excluded.target, event = excluded.event, filter = excluded.filter,
			param = excluded.param, title = excluded.title, body = excluded.body,
			link = excluded.link, declarer = excluded.declarer, verified = excluded.verified`,
		[]any{t.ID, t.Target, t.Event, trigger.FormatFilter(t.Filter), t.Param, t.Title, t.Body, t.Link, t.Declarer, verified}}
}

// DeleteTriggerStatement removes a trigger and its opt-ins.
func DeleteTriggerStatement(id string) Statement {
	return Statement{`DELETE FROM triggers WHERE id = ?`, []any{id}}
}

// CursorStatement stores the cursor.
func CursorStatement(c Cursor) Statement {
	return Statement{`
		INSERT INTO cursor (id, height_done, bound, next_tx) VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET height_done = excluded.height_done, bound = excluded.bound, next_tx = excluded.next_tx`,
		[]any{c.HeightDone, c.Bound, c.NextTx}}
}

// SubscribersStatement reads the subscriptions opted in to triggerID with
// value, in id order. SubscribersOf decodes its Result.
func SubscribersStatement(triggerID, value string) Statement {
	return Statement{`
		SELECT s.id, s.endpoint, s.p256dh, s.auth FROM optins o JOIN subscriptions s ON s.id = o.subscription_id
		WHERE o.trigger_id = ? AND o.value = ? ORDER BY s.id`,
		[]any{triggerID, value}}
}

// Subscriber is a subscription with its id.
type Subscriber struct {
	ID           int64
	Subscription Subscription
}

// SubscribersOf decodes the Result of a SubscribersStatement.
func SubscribersOf(r Result) ([]Subscriber, error) {
	out := make([]Subscriber, 0, len(r.Rows))
	for _, row := range r.Rows {
		var s Subscriber
		for _, c := range []struct {
			name string
			dst  any
		}{
			{"id", &s.ID}, {"endpoint", &s.Subscription.Endpoint},
			{"p256dh", &s.Subscription.P256dh}, {"auth", &s.Subscription.Auth},
		} {
			if err := column(row, c.name, c.dst); err != nil {
				return nil, fmt.Errorf("read subscriber: %w", err)
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// Triggers returns every stored trigger, ordered by target, event and id.
func (s *Store) Triggers(ctx context.Context) ([]trigger.Trigger, error) {
	res, err := s.Exec(ctx, []Statement{{SQL: selectTriggerColumns + ` FROM triggers ORDER BY target, event, id`}})
	if err != nil {
		return nil, fmt.Errorf("read triggers: %w", err)
	}
	out := make([]trigger.Trigger, 0, len(res[0].Rows))
	for _, row := range res[0].Rows {
		var t trigger.Trigger
		var filter string
		var verified int64
		for _, c := range []struct {
			name string
			dst  any
		}{
			{"id", &t.ID}, {"target", &t.Target}, {"event", &t.Event}, {"filter", &filter},
			{"param", &t.Param}, {"title", &t.Title}, {"body", &t.Body}, {"link", &t.Link},
			{"declarer", &t.Declarer}, {"verified", &verified},
		} {
			if err := column(row, c.name, c.dst); err != nil {
				return nil, fmt.Errorf("read triggers: %w", err)
			}
		}
		t.Verified = verified != 0
		if t.Filter, err = trigger.ParseFilter(filter); err != nil {
			return nil, fmt.Errorf("trigger %s: %w", t.ID, err)
		}
		out = append(out, t)
	}
	return out, nil
}

// Pending is a push to write to the outbox with its attempt count and the
// time of its next attempt.
type Pending struct {
	Push
	Attempts    int
	NextAttempt time.Time
}

// Queue writes pending to the outbox, skipping pushes already queued and
// pushes whose subscription is gone. It runs in batches of MaxStatements,
// so a failure part way leaves the earlier batches written; queueing the
// same list again completes it.
func (s *Store) Queue(ctx context.Context, pending []Pending) error {
	for chunk := range slices.Chunk(pending, MaxStatements) {
		stmts := make([]Statement, len(chunk))
		for i, p := range chunk {
			// OR IGNORE does not cover a foreign key, so the select finds the
			// subscription or writes nothing.
			stmts[i] = Statement{`
				INSERT OR IGNORE INTO outbox
					(subscription_id, trigger_id, tx_hash, event_index, title, body, link, attempts, next_attempt_at, created_at)
				SELECT id, ?, ?, ?, ?, ?, ?, ?, ?, ? FROM subscriptions WHERE id = ?`,
				[]any{p.TriggerID, p.TxHash, p.EventIndex,
					p.Notification.Title, p.Notification.Body, p.Notification.Link,
					p.Attempts, p.NextAttempt.UTC().UnixMilli(), p.CreatedAt.UTC().UnixMilli(), p.SubscriptionID}}
		}
		if _, err := s.Exec(ctx, stmts); err != nil {
			return fmt.Errorf("queue pushes: %w", err)
		}
	}
	return nil
}
