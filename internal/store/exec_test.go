package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecReturnsRowsAndChanges(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		results, err := s.Exec(context.Background(), []Statement{
			{SQL: `CREATE TABLE x (id INTEGER PRIMARY KEY, v TEXT)`},
			{SQL: `INSERT INTO x (id, v) VALUES (?, ?)`, Args: []any{1, "a"}},
			{SQL: `SELECT id, v FROM x`},
		})
		require.NoError(t, err)
		require.Len(t, results, 3)
		require.EqualValues(t, 1, results[1].Changes)
		require.Equal(t, []map[string]any{{"id": int64(1), "v": "a"}}, results[2].Rows)
	})
}

func TestExecIsAtomic(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		ctx := context.Background()
		_, err := s.Exec(ctx, []Statement{{SQL: `CREATE TABLE x (id INTEGER PRIMARY KEY, v TEXT)`}})
		require.NoError(t, err)

		_, err = s.Exec(ctx, []Statement{
			{SQL: `INSERT INTO x (id, v) VALUES (1, 'a')`},
			{SQL: `INSERT INTO x (id, v) VALUES (1, 'b')`},
		})
		require.Error(t, err)

		results, err := s.Exec(ctx, []Statement{{SQL: `SELECT count(*) AS n FROM x`}})
		require.NoError(t, err)
		require.Equal(t, int64(0), results[0].Rows[0]["n"])
	})
}

func TestExecRefusesOverMaxStatements(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		ctx := context.Background()
		_, err := s.Exec(ctx, []Statement{{SQL: `CREATE TABLE x (id INTEGER PRIMARY KEY)`}})
		require.NoError(t, err)

		stmts := make([]Statement, MaxStatements+1)
		for i := range stmts {
			stmts[i] = Statement{SQL: `INSERT INTO x (id) VALUES (?)`, Args: []any{i}}
		}
		_, err = s.Exec(ctx, stmts)
		require.ErrorIs(t, err, ErrBatchTooLarge)

		results, err := s.Exec(ctx, []Statement{{SQL: `SELECT count(*) AS n FROM x`}})
		require.NoError(t, err)
		require.Equal(t, int64(0), results[0].Rows[0]["n"])
	})
}

func TestExecChangesOnlyCountsRowWrites(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		results, err := s.Exec(context.Background(), []Statement{
			{SQL: `CREATE TABLE x (id INTEGER PRIMARY KEY)`},
			{SQL: `INSERT INTO x (id) VALUES (1)`},
			{SQL: `CREATE TABLE y (id INTEGER PRIMARY KEY)`},
			{SQL: `SELECT id FROM x`},
			{SQL: `INSERT INTO x (id) VALUES (2) RETURNING id`},
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, results[1].Changes)
		require.EqualValues(t, 0, results[2].Changes)
		require.EqualValues(t, 0, results[3].Changes)
		require.EqualValues(t, 1, results[4].Changes)
		require.Equal(t, []map[string]any{{"id": int64(2)}}, results[4].Rows)
	})
}

func TestExecRefusesValuesOutsideTheContract(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		ctx := context.Background()
		_, err := s.Exec(ctx, []Statement{
			{SQL: `CREATE TABLE t (d DATETIME)`},
			{SQL: `INSERT INTO t (d) VALUES ('2026-01-01 00:00:00')`},
		})
		require.NoError(t, err)

		_, err = s.Exec(ctx, []Statement{{SQL: `SELECT d FROM t`}})
		require.ErrorContains(t, err, "unsupported value type time.Time")
	})
}

func TestExecNormalizesValues(t *testing.T) {
	eachBackend(t, func(t *testing.T, fresh func(*testing.T) *Store) {
		s := fresh(t)
		results, err := s.Exec(context.Background(), []Statement{
			{SQL: `SELECT 1 AS i, 1.5 AS f, 'x' AS s, NULL AS n, CAST('y' AS BLOB) AS b`},
		})
		require.NoError(t, err)
		require.Equal(t, []map[string]any{{"i": int64(1), "f": 1.5, "s": "x", "n": nil, "b": "y"}}, results[0].Rows)
	})
}
