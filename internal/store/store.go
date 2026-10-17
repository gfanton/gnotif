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
// version.
const MaxPerTarget = 64

// Store is gnotifd's database: a SQLite file or a remote store. It is safe
// for concurrent use.
type Store struct {
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
	// Match is the match the push is for. It is kept in memory only: the
	// outbox does not store it, so it is nil once a push is read back.
	Match     *trigger.Match
	CreatedAt time.Time
}

// Delivery is a queued push joined with its subscription.
type Delivery struct {
	ID             int64
	SubscriptionID int64
	Subscription   Subscription
	Notification   trigger.Notification
	Attempts       int
	NextAttempt    time.Time
	CreatedAt      time.Time
}

// DueKey is a place in the outbox's due order: by next attempt, then by id.
// The zero DueKey comes before every queued push.
type DueKey struct {
	At time.Time
	ID int64
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
		s.ex = local{db: db}
	}
	if err := s.migrate(ctx, dsn); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// migrate refuses a database made by another schema version and otherwise
// applies the schema and records its version in one batch. Only the tables
// gnotifd creates count: the database may hold others.
func (s *Store) migrate(ctx context.Context, name string) error {
	res, err := s.Exec(ctx, []Statement{{
		SQL: `SELECT count(*) AS n FROM sqlite_master WHERE type = 'table' AND name IN ('cursor', 'triggers', 'subscriptions', 'optins', 'outbox', 'gnotifd_schema')`,
	}})
	if err != nil {
		return fmt.Errorf("count tables: %w", err)
	}
	if len(res[0].Rows) == 0 {
		return errors.New("count tables: no row in the answer")
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
	       o.title, o.body, o.link, o.attempts, o.next_attempt_at, o.created_at
	FROM outbox o JOIN subscriptions s ON s.id = o.subscription_id
	WHERE o.next_attempt_at <= ? AND (o.next_attempt_at, o.id) > (?, ?)
	ORDER BY o.next_attempt_at, o.id
	LIMIT ?`

// Due returns at most limit queued pushes whose next attempt is at or
// before now, in due order, starting past the key after.
func (s *Store) Due(ctx context.Context, now time.Time, after DueKey, limit int) ([]Delivery, error) {
	res, err := s.Exec(ctx, []Statement{{selectDue, []any{now.UTC().UnixMilli(), after.At.UTC().UnixMilli(), after.ID, limit}}})
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
	var attempts, next, created int64
	var err error
	for _, c := range []struct {
		name string
		dst  any
	}{
		{"id", &d.ID}, {"subscription_id", &d.SubscriptionID},
		{"endpoint", &d.Subscription.Endpoint}, {"p256dh", &d.Subscription.P256dh}, {"auth", &d.Subscription.Auth},
		{"title", &d.Notification.Title}, {"body", &d.Notification.Body}, {"link", &d.Notification.Link},
		{"attempts", &attempts}, {"next_attempt_at", &next}, {"created_at", &created},
	} {
		if err = column(r, c.name, c.dst); err != nil {
			return Delivery{}, err
		}
	}
	d.Attempts = int(attempts)
	d.NextAttempt = time.UnixMilli(next).UTC()
	d.CreatedAt = time.UnixMilli(created).UTC()
	return d, nil
}

// column stores row[name] in dst, a *int64 or a *string, and fails when the
// value has the other type or dst is neither.
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
	default:
		return fmt.Errorf("column %s: unsupported destination %T", name, dst)
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

const selectTriggerColumns = `SELECT id, target, event, filter, param, title, body, link, declarer, verified`
