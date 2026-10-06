// Package store keeps gnotifd's state in one SQLite file: the chain cursor,
// the triggers read from the registry, browser subscriptions with their
// opt-ins, and the outbox of pushes waiting to be sent.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/gfanton/gnotif/internal/trigger"
)

// ErrUnknownSubscription reports an endpoint with no stored subscription.
var ErrUnknownSubscription = errors.New("unknown subscription")

// ErrSchemaVersion reports a database made by an older gnotifd.
var ErrSchemaVersion = errors.New("database was made by an older gnotifd")

// MaxPerTarget is the most triggers one realm may have; TargetTriggers
// returns no more.
const MaxPerTarget = 64

// Store is the SQLite database. It is safe for concurrent use.
type Store struct {
	db *sql.DB
}

// Cursor is the chain position of the watch loop. HeightDone is the last
// height fully processed; Bound is the indexer's latest height on the
// previous tick, or 0 when unset. NextTx above 0 means block HeightDone+1 is
// being read one transaction at a time, from index NextTx.
type Cursor struct {
	HeightDone, Bound int64
	NextTx            int
}

// Subscription is a browser push subscription.
type Subscription struct{ Endpoint, P256dh, Auth string }

// Optin asks for notifications from one trigger. Value is empty for a
// trigger without a parameter.
type Optin struct{ TriggerID, Value string }

// Push is a notification queued for one subscription. The subscription,
// trigger, transaction and event index identify it, so queueing it twice
// keeps one row.
type Push struct {
	SubscriptionID int64
	TriggerID      string
	TxHash         string
	EventIndex     int
	Notification   trigger.Notification
	CreatedAt      time.Time
}

// Delivery is a queued push joined with its subscription.
type Delivery struct {
	ID             int64
	SubscriptionID int64
	Subscription   Subscription
	Notification   trigger.Notification
	Attempts       int
	CreatedAt      time.Time
}

// Open opens or creates the database at path and its schema.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	var version, tables int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("read schema version: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables); err != nil {
		db.Close()
		return nil, fmt.Errorf("count tables: %w", err)
	}
	if tables > 0 && version < schemaVersion {
		db.Close()
		return nil, fmt.Errorf("%s: %w: start a new database with -start-height", path, ErrSchemaVersion)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		db.Close()
		return nil, fmt.Errorf("set schema version: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Cursor returns the stored cursor, and false before the first SetCursor.
func (s *Store) Cursor(ctx context.Context) (Cursor, bool, error) {
	var c Cursor
	err := s.db.QueryRowContext(ctx, `SELECT height_done, bound, next_tx FROM cursor WHERE id = 1`).Scan(&c.HeightDone, &c.Bound, &c.NextTx)
	if errors.Is(err, sql.ErrNoRows) {
		return Cursor{}, false, nil
	}
	if err != nil {
		return Cursor{}, false, fmt.Errorf("read cursor: %w", err)
	}
	return c, true, nil
}

// Update runs fn in one write transaction, committed when fn returns nil
// and rolled back otherwise.
func (s *Store) Update(ctx context.Context, fn func(*Tx) error) error {
	return s.inTx(ctx, func(tx *sql.Tx) error { return fn(&Tx{tx: tx}) })
}

func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Triggers returns every stored trigger in id order.
func (s *Store) Triggers(ctx context.Context) ([]trigger.Trigger, error) {
	rows, err := s.db.QueryContext(ctx, selectTriggers)
	if err != nil {
		return nil, fmt.Errorf("read triggers: %w", err)
	}
	return scanTriggers(rows)
}

// TargetTriggers returns the triggers of one realm in id order, at most
// MaxPerTarget.
func (s *Store) TargetTriggers(ctx context.Context, target string) ([]trigger.Trigger, error) {
	rows, err := s.db.QueryContext(ctx, selectTriggerColumns+` FROM triggers WHERE target = ? ORDER BY id LIMIT ?`, target, MaxPerTarget)
	if err != nil {
		return nil, fmt.Errorf("read triggers of %s: %w", target, err)
	}
	return scanTriggers(rows)
}

// TriggersByID returns the stored triggers among ids, keyed by id. Unknown
// ids are absent.
func (s *Store) TriggersByID(ctx context.Context, ids []string) (map[string]trigger.Trigger, error) {
	out := make(map[string]trigger.Trigger, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, selectTriggerColumns+` FROM triggers WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("read triggers by id: %w", err)
	}
	found, err := scanTriggers(rows)
	if err != nil {
		return nil, err
	}
	for _, t := range found {
		out[t.ID] = t
	}
	return out, nil
}

// PutSubscription stores sub, or replaces the keys of the subscription with
// the same endpoint and keeps its opt-ins.
func (s *Store) PutSubscription(ctx context.Context, sub Subscription) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (endpoint, p256dh, auth, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth`,
		sub.Endpoint, sub.P256dh, sub.Auth, time.Now().UTC().UnixMilli())
	if err != nil {
		return fmt.Errorf("put subscription: %w", err)
	}
	return nil
}

// MoveSubscription stores sub in place of the subscription at oldEndpoint,
// so its opt-ins and queued pushes follow the browser's new endpoint. It
// acts as PutSubscription when oldEndpoint is not stored or sub's endpoint
// already is.
func (s *Store) MoveSubscription(ctx context.Context, oldEndpoint string, sub Subscription) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions SET endpoint = ?, p256dh = ?, auth = ?
		WHERE endpoint = ? AND NOT EXISTS (SELECT 1 FROM subscriptions WHERE endpoint = ?)`,
		sub.Endpoint, sub.P256dh, sub.Auth, oldEndpoint, sub.Endpoint)
	if err != nil {
		return fmt.Errorf("move subscription: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 1 {
		return nil
	}
	return s.PutSubscription(ctx, sub)
}

// ReplaceOptins replaces every opt-in of the subscription at endpoint.
func (s *Store) ReplaceOptins(ctx context.Context, endpoint string, optins []Optin) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRow(`SELECT id FROM subscriptions WHERE endpoint = ?`, endpoint).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnknownSubscription
		}
		if err != nil {
			return fmt.Errorf("find subscription: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM optins WHERE subscription_id = ?`, id); err != nil {
			return fmt.Errorf("clear opt-ins: %w", err)
		}
		for _, o := range optins {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO optins (subscription_id, trigger_id, value) VALUES (?, ?, ?)`, id, o.TriggerID, o.Value); err != nil {
				return fmt.Errorf("add opt-in: %w", err)
			}
		}
		return nil
	})
}

// DeleteSubscription removes the subscription at endpoint.
func (s *Store) DeleteSubscription(ctx context.Context, endpoint string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE endpoint = ?`, endpoint)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrUnknownSubscription
	}
	return nil
}

// RemoveSubscription removes a subscription by id, with its opt-ins and
// queued pushes.
func (s *Store) RemoveSubscription(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("remove subscription: %w", err)
	}
	return nil
}

// Due returns at most limit queued pushes whose next attempt is at or
// before now, oldest first.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Delivery, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id, o.subscription_id, s.endpoint, s.p256dh, s.auth,
		       o.title, o.body, o.link, o.attempts, o.created_at
		FROM outbox o JOIN subscriptions s ON s.id = o.subscription_id
		WHERE o.next_attempt_at <= ?
		ORDER BY o.id
		LIMIT ?`, now.UTC().UnixMilli(), limit)
	if err != nil {
		return nil, fmt.Errorf("read due pushes: %w", err)
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		var created int64
		if err := rows.Scan(&d.ID, &d.SubscriptionID, &d.Subscription.Endpoint, &d.Subscription.P256dh, &d.Subscription.Auth,
			&d.Notification.Title, &d.Notification.Body, &d.Notification.Link, &d.Attempts, &created); err != nil {
			return nil, fmt.Errorf("scan due push: %w", err)
		}
		d.CreatedAt = time.UnixMilli(created).UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDelivery removes a queued push.
func (s *Store) DeleteDelivery(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM outbox WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete push: %w", err)
	}
	return nil
}

// Reschedule counts a failed attempt and sets the next one at at.
func (s *Store) Reschedule(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE outbox SET attempts = attempts + 1, next_attempt_at = ? WHERE id = ?`, at.UTC().UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("reschedule push: %w", err)
	}
	return nil
}

// Tx is a write transaction opened by Update. Its methods run under the
// context Update was given.
type Tx struct {
	tx *sql.Tx
}

// SetCursor stores the cursor.
func (tx *Tx) SetCursor(c Cursor) error {
	_, err := tx.tx.Exec(`
		INSERT INTO cursor (id, height_done, bound, next_tx) VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET height_done = excluded.height_done, bound = excluded.bound, next_tx = excluded.next_tx`,
		c.HeightDone, c.Bound, c.NextTx)
	if err != nil {
		return fmt.Errorf("write cursor: %w", err)
	}
	return nil
}

// PutTrigger stores t, updating the trigger with the same id in place so
// its opt-ins survive.
func (tx *Tx) PutTrigger(t trigger.Trigger) error {
	_, err := tx.tx.Exec(`
		INSERT INTO triggers (id, target, event, filter, param, title, body, link, declarer, verified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			target = excluded.target, event = excluded.event, filter = excluded.filter,
			param = excluded.param, title = excluded.title, body = excluded.body,
			link = excluded.link, declarer = excluded.declarer, verified = excluded.verified`,
		t.ID, t.Target, t.Event, trigger.FormatFilter(t.Filter), t.Param, t.Title, t.Body, t.Link, t.Declarer, t.Verified)
	if err != nil {
		return fmt.Errorf("put trigger %s: %w", t.ID, err)
	}
	return nil
}

// DeleteTrigger removes a trigger and its opt-ins.
func (tx *Tx) DeleteTrigger(id string) error {
	if _, err := tx.tx.Exec(`DELETE FROM triggers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete trigger %s: %w", id, err)
	}
	return nil
}

// Targets returns the distinct realms the triggers watch, sorted.
func (tx *Tx) Targets() ([]string, error) {
	rows, err := tx.tx.Query(`SELECT DISTINCT target FROM triggers ORDER BY target`)
	if err != nil {
		return nil, fmt.Errorf("read targets: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

// Triggers returns every stored trigger in id order.
func (tx *Tx) Triggers() ([]trigger.Trigger, error) {
	rows, err := tx.tx.Query(selectTriggers)
	if err != nil {
		return nil, fmt.Errorf("read triggers: %w", err)
	}
	return scanTriggers(rows)
}

// Matching returns the triggers on target for event, in id order.
func (tx *Tx) Matching(target, event string) ([]trigger.Trigger, error) {
	rows, err := tx.tx.Query(selectMatching, target, event)
	if err != nil {
		return nil, fmt.Errorf("read triggers matching %s %s: %w", target, event, err)
	}
	return scanTriggers(rows)
}

// Subscribers returns the subscriptions opted in to triggerID with value.
func (tx *Tx) Subscribers(triggerID, value string) ([]int64, error) {
	rows, err := tx.tx.Query(`SELECT subscription_id FROM optins WHERE trigger_id = ? AND value = ? ORDER BY subscription_id`, triggerID, value)
	if err != nil {
		return nil, fmt.Errorf("read subscribers: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Enqueue queues p, due at once. It reports false when p is already queued.
func (tx *Tx) Enqueue(p Push) (bool, error) {
	created := p.CreatedAt.UTC().UnixMilli()
	res, err := tx.tx.Exec(`
		INSERT OR IGNORE INTO outbox
			(subscription_id, trigger_id, tx_hash, event_index, title, body, link, next_attempt_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.SubscriptionID, p.TriggerID, p.TxHash, p.EventIndex,
		p.Notification.Title, p.Notification.Body, p.Notification.Link, created, created)
	if err != nil {
		return false, fmt.Errorf("enqueue push: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("enqueue push: %w", err)
	}
	return n == 1, nil
}

const (
	selectTriggerColumns = `SELECT id, target, event, filter, param, title, body, link, declarer, verified`
	selectTriggers       = selectTriggerColumns + ` FROM triggers ORDER BY id`
	selectMatching       = selectTriggerColumns + ` FROM triggers WHERE target = ? AND event = ? ORDER BY id`
)

func scanTriggers(rows *sql.Rows) ([]trigger.Trigger, error) {
	defer rows.Close()
	var out []trigger.Trigger
	for rows.Next() {
		var t trigger.Trigger
		var filter string
		if err := rows.Scan(&t.ID, &t.Target, &t.Event, &filter, &t.Param, &t.Title, &t.Body, &t.Link, &t.Declarer, &t.Verified); err != nil {
			return nil, fmt.Errorf("scan trigger: %w", err)
		}
		pairs, err := trigger.ParseFilter(filter)
		if err != nil {
			return nil, fmt.Errorf("trigger %s: %w", t.ID, err)
		}
		t.Filter = pairs
		out = append(out, t)
	}
	return out, rows.Err()
}
