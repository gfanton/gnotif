// Package storetest serves gnotifd's remote store protocol on a SQLite file,
// for tests of code that talks to a remote store. It implements the protocol
// on its own, apart from package store, so a client is tested against code it
// does not share.
package storetest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

type request struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

type result struct {
	Rows    []map[string]any `json:"rows"`
	Changes int64            `json:"changes"`
}

// NewServer starts a server for a new, empty database in t.TempDir(), closed
// when the test ends. As the protocol requires, it enforces foreign keys and
// runs each request as one transaction. It answers 400 to a malformed body and 500 with
// {"error": message} to a failing statement.
func NewServer(t testing.TB) *httptest.Server {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "remote.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	db.SetMaxOpenConns(1)
	srv := httptest.NewServer(handler(db))
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return srv
}

func handler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqs []request
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		if r.Method != http.MethodPost || dec.Decode(&reqs) != nil {
			http.Error(w, "malformed request", http.StatusBadRequest)
			return
		}
		results, err := run(r, db, reqs)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})
}

func run(r *http.Request, db *sql.DB, reqs []request) ([]result, error) {
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	results := make([]result, len(reqs))
	before, err := totalChanges(tx)
	if err != nil {
		return nil, err
	}
	for i, req := range reqs {
		args := make([]any, len(req.Args))
		for j, a := range req.Args {
			if args[j], err = bind(a); err != nil {
				return nil, fmt.Errorf("statement %d: %w", i, err)
			}
		}
		if results[i].Rows, err = query(r, tx, req.SQL, args); err != nil {
			return nil, fmt.Errorf("statement %d: %w", i, err)
		}
		after, err := totalChanges(tx)
		if err != nil {
			return nil, err
		}
		results[i].Changes = after - before
		before = after
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func totalChanges(tx *sql.Tx) (int64, error) {
	var n int64
	err := tx.QueryRow(`SELECT total_changes()`).Scan(&n)
	return n, err
}

// bind turns a decoded JSON argument into a value SQLite stores as itself.
func bind(a any) (any, error) {
	switch a := a.(type) {
	case nil, string:
		return a, nil
	case json.Number:
		if i, err := a.Int64(); err == nil {
			return i, nil
		}
		return a.Float64()
	default:
		return nil, fmt.Errorf("unsupported argument type %T", a)
	}
}

func query(r *http.Request, tx *sql.Tx, q string, args []any) ([]map[string]any, error) {
	rows, err := tx.QueryContext(r.Context(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			switch v := vals[i].(type) {
			case nil, int64, float64, string:
				row[c] = v
			case []byte:
				row[c] = string(v)
			default:
				return nil, fmt.Errorf("column %s: unsupported value type %T", c, v)
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, rows.Close()
}
