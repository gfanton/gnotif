package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// remoteTimeout is how long gnotifd waits for a remote store's answer, so a
// hung call cannot block the store for good.
const remoteTimeout = 40 * time.Second

// maxErrorBody bounds how much of a failed answer is read for its message.
const maxErrorBody = 64 << 10

// remote is the executor over the SQL-over-HTTP protocol: one POST of a JSON
// array of {"sql", "args"}, answered by one {"rows", "changes"} per statement.
type remote struct {
	url    string
	client *http.Client
}

type wireStatement struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

type wireResult struct {
	Rows    []map[string]any `json:"rows"`
	Changes int64            `json:"changes"`
}

func newRemote(url string) *remote {
	return &remote{url: url, client: &http.Client{Timeout: remoteTimeout}}
}

func (r *remote) close() error {
	r.client.CloseIdleConnections()
	return nil
}

func (r *remote) exec(ctx context.Context, stmts []Statement) ([]Result, error) {
	if len(stmts) > MaxStatements {
		return nil, fmt.Errorf("%d statements: %w", len(stmts), ErrBatchTooLarge)
	}
	wire := make([]wireStatement, len(stmts))
	for i, st := range stmts {
		wire[i] = wireStatement{SQL: st.SQL, Args: st.Args}
		if wire[i].Args == nil {
			wire[i].Args = []any{}
		}
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote store: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp)
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var answered []wireResult
	if err := dec.Decode(&answered); err != nil {
		return nil, fmt.Errorf("decode answer: %w", err)
	}
	if len(answered) != len(stmts) {
		return nil, fmt.Errorf("remote store answered %d results for %d statements", len(answered), len(stmts))
	}
	results := make([]Result, len(answered))
	for i, a := range answered {
		results[i].Changes = a.Changes
		for _, row := range a.Rows {
			for col, v := range row {
				if row[col], err = decodeValue(v); err != nil {
					return nil, fmt.Errorf("statement %d: column %s: %w", i, col, err)
				}
			}
			results[i].Rows = append(results[i].Rows, row)
		}
	}
	return results, nil
}

func statusError(resp *http.Response) error {
	var failure struct {
		Error string `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if json.Unmarshal(raw, &failure) == nil && failure.Error != "" {
		return fmt.Errorf("remote store: status %d: %s", resp.StatusCode, failure.Error)
	}
	return fmt.Errorf("remote store: status %d", resp.StatusCode)
}

// decodeValue maps a JSON value to the types Result documents: an integral
// number is an int64, any other a float64.
func decodeValue(v any) (any, error) {
	switch v := v.(type) {
	case nil, string:
		return v, nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, nil
		}
		return v.Float64()
	default:
		return nil, fmt.Errorf("unsupported value type %T", v)
	}
}

func (r *remote) version(ctx context.Context) (int, bool, error) {
	res, err := r.exec(ctx, []Statement{{SQL: `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'gnotifd_schema'`}})
	if err != nil {
		return 0, false, fmt.Errorf("find schema version: %w", err)
	}
	if len(res[0].Rows) == 0 {
		return 0, false, nil
	}
	res, err = r.exec(ctx, []Statement{{SQL: `SELECT version FROM gnotifd_schema WHERE id = 1`}})
	if err != nil {
		return 0, false, fmt.Errorf("read schema version: %w", err)
	}
	if len(res[0].Rows) == 0 {
		return 0, false, nil
	}
	var v int64
	if err := column(res[0].Rows[0], "version", &v); err != nil {
		return 0, false, fmt.Errorf("read schema version: %w", err)
	}
	return int(v), true, nil
}

// setVersion creates the one-row table that holds the version, because a
// server of the protocol need not support PRAGMA user_version, and stores v
// in it.
func (r *remote) setVersion(v int) []Statement {
	return []Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS gnotifd_schema (id INTEGER PRIMARY KEY CHECK (id = 1), version INTEGER NOT NULL)`},
		{SQL: `INSERT INTO gnotifd_schema (id, version) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET version = excluded.version`, Args: []any{v}},
	}
}
