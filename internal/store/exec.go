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
// string values, keyed by column name. Changes counts the rows a statement
// without result columns inserted, updated or deleted, and is 0 otherwise.
type Result struct {
	Rows    []map[string]any
	Changes int64
}

// executor runs statements as one atomic batch: all take effect, or none.
type executor interface {
	exec(ctx context.Context, stmts []Statement) ([]Result, error)
	close() error
}

// Exec runs stmts in order as one atomic batch and returns one Result per
// statement. A failing statement rolls the whole batch back.
func (s *Store) Exec(ctx context.Context, stmts []Statement) ([]Result, error) {
	if len(stmts) > MaxStatements {
		return nil, fmt.Errorf("%d statements: %w", len(stmts), ErrBatchTooLarge)
	}
	return s.ex.exec(ctx, stmts)
}
