package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/store/storetest"
)

func TestRemoteNumbers(t *testing.T) {
	s := open(t, storetest.NewServer(t).URL)
	res := exec(t, s, Statement{SQL: `SELECT 1700000000000 AS ms, 0.5 AS half`})
	assert.Equal(t, []map[string]any{{"ms": int64(1700000000000), "half": 0.5}}, res[0].Rows)
}

func TestRemoteTextRoundTrip(t *testing.T) {
	const text = "Tour joué 🎲 — ваш ход"
	s := open(t, storetest.NewServer(t).URL)
	res := exec(t, s,
		Statement{SQL: `CREATE TABLE t (v TEXT)`},
		Statement{`INSERT INTO t (v) VALUES (?)`, []any{text}},
		Statement{SQL: `SELECT v FROM t`},
	)
	assert.Equal(t, []map[string]any{{"v": text}}, res[2].Rows)
}

func TestRemoteWrongResultCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"rows":[],"changes":0}]`))
	}))
	t.Cleanup(srv.Close)

	_, err := newRemote(srv.URL).exec(context.Background(), []Statement{{SQL: `SELECT 1`}, {SQL: `SELECT 2`}})
	require.ErrorContains(t, err, "results")
}

func TestRemoteErrorStatus(t *testing.T) {
	t.Run("sql error carries the server message", func(t *testing.T) {
		s := open(t, storetest.NewServer(t).URL)
		_, err := s.Exec(context.Background(), []Statement{{SQL: `SELECT * FROM missing`}})
		require.ErrorContains(t, err, "no such table: missing")
	})
	t.Run("bad request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusBadRequest)
		}))
		t.Cleanup(srv.Close)

		_, err := newRemote(srv.URL).exec(context.Background(), []Statement{{SQL: `SELECT 1`}})
		require.ErrorContains(t, err, "400")
	})
}

func TestOpenFreshRemote(t *testing.T) {
	s := open(t, storetest.NewServer(t).URL)
	res := exec(t, s,
		Statement{SQL: `SELECT version FROM gnotifd_schema`},
		Statement{SQL: `SELECT count(*) AS n FROM sqlite_master WHERE name = 'outbox'`},
	)
	assert.Equal(t, []map[string]any{{"version": int64(2)}}, res[0].Rows)
	assert.Equal(t, int64(1), res[1].Rows[0]["n"])
}

func TestOpenRemoteIgnoresTablesItDidNotCreate(t *testing.T) {
	srv := storetest.NewServer(t)
	_, err := newRemote(srv.URL).exec(context.Background(), []Statement{{SQL: `CREATE TABLE app_data (k TEXT)`}})
	require.NoError(t, err)

	s := open(t, srv.URL)
	exec(t, s, Statement{SQL: `SELECT version FROM gnotifd_schema`})
}

func TestOpenRemoteRefusesOldAndNewer(t *testing.T) {
	cases := map[string]struct {
		version int
		want    error
	}{
		"older": {1, ErrSchemaVersion},
		"newer": {3, ErrSchemaNewer},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := storetest.NewServer(t)
			_, err := newRemote(srv.URL).exec(context.Background(), []Statement{
				{SQL: `CREATE TABLE gnotifd_schema (id INTEGER PRIMARY KEY CHECK (id = 1), version INTEGER NOT NULL)`},
				{SQL: `INSERT INTO gnotifd_schema (id, version) VALUES (1, ?)`, Args: []any{tc.version}},
			})
			require.NoError(t, err)

			_, err = Open(context.Background(), srv.URL)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestOpenRemoteRefusesTablesWithoutVersion(t *testing.T) {
	srv := storetest.NewServer(t)
	_, err := newRemote(srv.URL).exec(context.Background(), []Statement{{SQL: `CREATE TABLE cursor (id INTEGER PRIMARY KEY)`}})
	require.NoError(t, err)

	_, err = Open(context.Background(), srv.URL)
	require.ErrorIs(t, err, ErrSchemaVersion)
}

func TestOpenRemoteUnreachable(t *testing.T) {
	srv := storetest.NewServer(t)
	srv.Close()

	_, err := Open(context.Background(), srv.URL)
	require.Error(t, err)
}
