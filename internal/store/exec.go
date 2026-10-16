package store

import (
	"context"
	"errors"
	"fmt"
)

// MaxStatements is the most statements one Exec call accepts.
const MaxStatements = 1000

// ErrBatchTooLarge reports an Exec call over MaxStatements.
var ErrBatchTooLarge = errors.New("store: batch over MaxStatements")

// Statement is one SQL statement with its positional arguments.
type Statement struct {
	SQL  string
	Args []any
}

// Result is the outcome of one Statement. Rows hold nil, int64, float64 or
// string values, keyed by column name. Changes counts the rows the
// statement inserted, updated or deleted, RETURNING included; it is 0 for a
// SELECT or a schema change.
type Result struct {
	Rows    []map[string]any
	Changes int64
}

// executor runs statements as one atomic batch: all take effect, or none.
type executor interface {
	exec(ctx context.Context, stmts []Statement) ([]Result, error)
	close() error
	// version reports the schema version a database holds. found is false
	// when it holds none.
	version(ctx context.Context) (v int, found bool, err error)
	// setVersion returns the statements that record v as the schema version.
	setVersion(v int) []Statement
}

// Exec runs stmts in order as one atomic batch and returns one Result per
// statement. A failing statement rolls the whole batch back.
func (s *Store) Exec(ctx context.Context, stmts []Statement) ([]Result, error) {
	if len(stmts) > MaxStatements {
		return nil, fmt.Errorf("%d statements: %w", len(stmts), ErrBatchTooLarge)
	}
	return s.ex.exec(ctx, stmts)
}
