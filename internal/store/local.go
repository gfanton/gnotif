package store

import (
	"context"
	"database/sql"
	"fmt"
)

// local is the executor over the SQLite file.
type local struct {
	db *sql.DB
}

func (l local) close() error {
	return l.db.Close()
}

func (l local) exec(ctx context.Context, stmts []Statement) ([]Result, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	before, err := totalChanges(ctx, tx)
	if err != nil {
		return nil, err
	}
	results := make([]Result, len(stmts))
	for i, st := range stmts {
		results[i], err = runStatement(ctx, tx, st)
		if err != nil {
			return nil, fmt.Errorf("statement %d: %w", i, err)
		}
		after, err := totalChanges(ctx, tx)
		if err != nil {
			return nil, err
		}
		results[i].Changes = after - before
		before = after
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return results, nil
}

func totalChanges(ctx context.Context, tx *sql.Tx) (int64, error) {
	var n int64
	if err := tx.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&n); err != nil {
		return 0, fmt.Errorf("read changes: %w", err)
	}
	return n, nil
}

func runStatement(ctx context.Context, tx *sql.Tx, st Statement) (Result, error) {
	rows, err := tx.QueryContext(ctx, st.SQL, st.Args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}
	var res Result
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Result{}, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			if row[c], err = normalize(vals[i]); err != nil {
				return Result{}, fmt.Errorf("column %s: %w", c, err)
			}
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	if err := rows.Close(); err != nil {
		return Result{}, err
	}
	return res, nil
}

// normalize maps a scanned column to the value types Result documents.
func normalize(v any) (any, error) {
	switch v := v.(type) {
	case nil, int64, float64, string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", v)
	}
}
