package store

// schemaVersion is stored in PRAGMA user_version of every database this
// gnotifd creates.
const schemaVersion = 2

const schema = `
CREATE TABLE IF NOT EXISTS cursor (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	height_done INTEGER NOT NULL,
	bound       INTEGER NOT NULL,
	next_tx     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS triggers (
	id       TEXT PRIMARY KEY,
	target   TEXT NOT NULL,
	event    TEXT NOT NULL,
	filter   TEXT NOT NULL,
	param    TEXT NOT NULL,
	title    TEXT NOT NULL,
	body     TEXT NOT NULL,
	link     TEXT NOT NULL,
	declarer TEXT NOT NULL,
	verified INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS triggers_by_target ON triggers (target, event);

CREATE TABLE IF NOT EXISTS subscriptions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	endpoint   TEXT NOT NULL UNIQUE,
	p256dh     TEXT NOT NULL,
	auth       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS optins (
	subscription_id INTEGER NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
	trigger_id      TEXT NOT NULL REFERENCES triggers (id) ON DELETE CASCADE,
	value           TEXT NOT NULL,
	PRIMARY KEY (subscription_id, trigger_id, value)
);

CREATE INDEX IF NOT EXISTS optins_by_trigger ON optins (trigger_id, value);

CREATE TABLE IF NOT EXISTS outbox (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	subscription_id INTEGER NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
	trigger_id      TEXT NOT NULL,
	tx_hash         TEXT NOT NULL,
	event_index     INTEGER NOT NULL,
	title           TEXT NOT NULL,
	body            TEXT NOT NULL,
	link            TEXT NOT NULL,
	attempts        INTEGER NOT NULL DEFAULT 0,
	next_attempt_at INTEGER NOT NULL,
	created_at      INTEGER NOT NULL,
	UNIQUE (subscription_id, trigger_id, tx_hash, event_index)
);

CREATE INDEX IF NOT EXISTS outbox_due ON outbox (next_attempt_at, id);
`
