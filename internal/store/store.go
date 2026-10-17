// Package store keeps gnotifd's state in a SQLite file, or in a remote store
// that speaks the same SQL over HTTP: the chain cursor,
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

// ErrSchemaNewer reports a database made by a newer gnotifd.
var ErrSchemaNewer = errors.New("database was made by a newer gnotifd")

// MaxPerTarget is the most triggers one realm may have in each registry
// version; TargetTriggers returns no more.
const MaxPerTarget = 64

// Store is the SQLite database. It is safe for concurrent use.
type Store struct {
	db *sql.DB
	ex executor
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
	Subscription   Subscription
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

// Open opens or creates the store and its schema. A dsn starting with
// http:// or https:// is the URL of a remote store; anything else is the path
// of a SQLite file.
func Open(ctx context.Context, dsn string) (*Store, error) {
	s := &Store{}
	if strings.HasPrefix(dsn, "http://") || strings.HasPrefix(dsn, "https://") {
		s.ex = newRemote(dsn)
	} else {
		db, err := sql.Open("sqlite", "file:"+dsn+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate")
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", dsn, err)
		}
		s.db, s.ex = db, local{db: db}
	}
	if err := s.migrate(ctx, dsn); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// migrate refuses a database made by another schema version and otherwise
// applies the schema and records its version in one batch. Only the tables a
// gnotifd creates count: the database may hold others, such as a replication
// tool's.
func (s *Store) migrate(ctx context.Context, name string) error {
	res, err := s.Exec(ctx, []Statement{{
		SQL: `SELECT count(*) AS n FROM sqlite_master WHERE type = 'table' AND name IN ('cursor', 'triggers', 'subscriptions', 'optins', 'outbox', 'gnotifd_schema')`,
	}})
	if err != nil {
		return fmt.Errorf("count tables: %w", err)
	}
	var tables int64
	if err := column(res[0].Rows[0], "n", &tables); err != nil {
		return fmt.Errorf("count tables: %w", err)
	}
	version, found, err := s.ex.version(ctx)
	if err != nil {
		return err
	}
	if tables > 0 && (!found || version < schemaVersion) {
		return fmt.Errorf("%s: %w: start a new database with -start-height", name, ErrSchemaVersion)
	}
	if tables > 0 && version > schemaVersion {
		return fmt.Errorf("%s: %w: run the gnotifd that made it, or start a new database with -start-height", name, ErrSchemaNewer)
	}
	stmts := make([]Statement, 0, len(schema)+2)
	for _, q := range schema {
		stmts = append(stmts, Statement{SQL: q})
	}
	stmts = append(stmts, s.ex.setVersion(schemaVersion)...)
	if _, err := s.Exec(ctx, stmts); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.ex.close()
}

// Cursor returns the stored cursor, and false before the first SetCursor.
func (s *Store) Cursor(ctx context.Context) (Cursor, bool, error) {
	res, err := s.Exec(ctx, []Statement{{SQL: `SELECT height_done, bound, next_tx FROM cursor WHERE id = 1`}})
	if err != nil {
		return Cursor{}, false, fmt.Errorf("read cursor: %w", err)
	}
	if len(res[0].Rows) == 0 {
		return Cursor{}, false, nil
	}
	var c Cursor
	var nextTx int64
	for _, f := range []struct {
		name string
		dst  *int64
	}{{"height_done", &c.HeightDone}, {"bound", &c.Bound}, {"next_tx", &nextTx}} {
		if err := column(res[0].Rows[0], f.name, f.dst); err != nil {
			return Cursor{}, false, fmt.Errorf("read cursor: %w", err)
		}
	}
	c.NextTx = int(nextTx)
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
	_, err := s.Exec(ctx, []Statement{{`
		INSERT INTO subscriptions (endpoint, p256dh, auth, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth`,
		[]any{sub.Endpoint, sub.P256dh, sub.Auth, time.Now().UTC().UnixMilli()}}})
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
	res, err := s.Exec(ctx, []Statement{{`
		UPDATE subscriptions SET endpoint = ?, p256dh = ?, auth = ?
		WHERE endpoint = ? AND NOT EXISTS (SELECT 1 FROM subscriptions WHERE endpoint = ?)`,
		[]any{sub.Endpoint, sub.P256dh, sub.Auth, oldEndpoint, sub.Endpoint}}})
	if err != nil {
		return fmt.Errorf("move subscription: %w", err)
	}
	if res[0].Changes == 1 {
		return nil
	}
	return s.PutSubscription(ctx, sub)
}

// ReplaceOptins replaces every opt-in of the subscription at endpoint.
func (s *Store) ReplaceOptins(ctx context.Context, endpoint string, optins []Optin) error {
	const subscriptionID = `(SELECT id FROM subscriptions WHERE endpoint = ?)`
	stmts := make([]Statement, 0, len(optins)+2)
	stmts = append(stmts,
		Statement{`SELECT id FROM subscriptions WHERE endpoint = ?`, []any{endpoint}},
		Statement{`DELETE FROM optins WHERE subscription_id = ` + subscriptionID, []any{endpoint}})
	for _, o := range optins {
		stmts = append(stmts, Statement{
			`INSERT OR IGNORE INTO optins (subscription_id, trigger_id, value) VALUES (` + subscriptionID + `, ?, ?)`,
			[]any{endpoint, o.TriggerID, o.Value}})
	}
	res, err := s.Exec(ctx, stmts)
	if err != nil {
		return fmt.Errorf("replace opt-ins: %w", err)
	}
	if len(res[0].Rows) == 0 {
		return ErrUnknownSubscription
	}
	return nil
}

// DeleteSubscription removes the subscription at endpoint.
func (s *Store) DeleteSubscription(ctx context.Context, endpoint string) error {
	res, err := s.Exec(ctx, []Statement{{`DELETE FROM subscriptions WHERE endpoint = ?`, []any{endpoint}}})
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if res[0].Changes == 0 {
		return ErrUnknownSubscription
	}
	return nil
}

// RemoveSubscription removes a subscription by id, with its opt-ins and
// queued pushes.
func (s *Store) RemoveSubscription(ctx context.Context, id int64) error {
	if _, err := s.Exec(ctx, []Statement{{`DELETE FROM subscriptions WHERE id = ?`, []any{id}}}); err != nil {
		return fmt.Errorf("remove subscription: %w", err)
	}
	return nil
}

const selectDue = `
	SELECT o.id, o.subscription_id, s.endpoint, s.p256dh, s.auth,
	       o.title, o.body, o.link, o.attempts, o.created_at
	FROM outbox o JOIN subscriptions s ON s.id = o.subscription_id
	WHERE o.next_attempt_at <= ?
	ORDER BY o.next_attempt_at, o.id
	LIMIT ?`

// Due returns at most limit queued pushes whose next attempt is at or
// before now, earliest due first.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]Delivery, error) {
	res, err := s.Exec(ctx, []Statement{{selectDue, []any{now.UTC().UnixMilli(), limit}}})
	if err != nil {
		return nil, fmt.Errorf("read due pushes: %w", err)
	}
	var out []Delivery
	for _, r := range res[0].Rows {
		d, err := scanDelivery(r)
		if err != nil {
			return nil, fmt.Errorf("scan due push: %w", err)
		}
		out = append(out, d)
	}
	return out, nil
}

func scanDelivery(r map[string]any) (Delivery, error) {
	var d Delivery
	var attempts, created int64
	var err error
	for _, c := range []struct {
		name string
		dst  any
	}{
		{"id", &d.ID}, {"subscription_id", &d.SubscriptionID},
		{"endpoint", &d.Subscription.Endpoint}, {"p256dh", &d.Subscription.P256dh}, {"auth", &d.Subscription.Auth},
		{"title", &d.Notification.Title}, {"body", &d.Notification.Body}, {"link", &d.Notification.Link},
		{"attempts", &attempts}, {"created_at", &created},
	} {
		if err = column(r, c.name, c.dst); err != nil {
			return Delivery{}, err
		}
	}
	d.Attempts = int(attempts)
	d.CreatedAt = time.UnixMilli(created).UTC()
	return d, nil
}

// column stores row[name] in dst, a *int64 or a *string, and fails when the
// value has the other type.
func column(row map[string]any, name string, dst any) error {
	switch dst := dst.(type) {
	case *int64:
		v, ok := row[name].(int64)
		if !ok {
			return fmt.Errorf("column %s: %T is not an integer", name, row[name])
		}
		*dst = v
	case *string:
		v, ok := row[name].(string)
		if !ok {
			return fmt.Errorf("column %s: %T is not text", name, row[name])
		}
		*dst = v
	}
	return nil
}

// DeleteDelivery removes a queued push.
func (s *Store) DeleteDelivery(ctx context.Context, id int64) error {
	if _, err := s.Exec(ctx, []Statement{{`DELETE FROM outbox WHERE id = ?`, []any{id}}}); err != nil {
		return fmt.Errorf("delete push: %w", err)
	}
	return nil
}

// Reschedule counts a failed attempt and sets the next one at at.
func (s *Store) Reschedule(ctx context.Context, id int64, at time.Time) error {
	_, err := s.Exec(ctx, []Statement{{`UPDATE outbox SET attempts = attempts + 1, next_attempt_at = ? WHERE id = ?`, []any{at.UTC().UnixMilli(), id}}})
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
	st := CursorStatement(c)
	if _, err := tx.tx.Exec(st.SQL, st.Args...); err != nil {
		return fmt.Errorf("write cursor: %w", err)
	}
	return nil
}

// PutTrigger stores t, updating the trigger with the same id in place so
// its opt-ins survive.
func (tx *Tx) PutTrigger(t trigger.Trigger) error {
	st := PutTriggerStatement(t)
	if _, err := tx.tx.Exec(st.SQL, st.Args...); err != nil {
		return fmt.Errorf("put trigger %s: %w", t.ID, err)
	}
	return nil
}

// DeleteTrigger removes a trigger and its opt-ins.
func (tx *Tx) DeleteTrigger(id string) error {
	st := DeleteTriggerStatement(id)
	if _, err := tx.tx.Exec(st.SQL, st.Args...); err != nil {
		return fmt.Errorf("delete trigger %s: %w", id, err)
	}
	return nil
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
