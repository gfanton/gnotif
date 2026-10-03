package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/trigger"
)

func serve(t *testing.T, status int, body string, inspect func(t *testing.T, req map[string]any)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var req map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if inspect != nil {
			inspect(t, req)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, srv.Client())
}

func TestFetchDecodes(t *testing.T) {
	fixture, err := os.ReadFile("testdata/window.json")
	require.NoError(t, err)
	paths := []string{"gno.land/r/dev/gnotif/v0", "gno.land/r/demo/game"}
	c := serve(t, http.StatusOK, string(fixture), func(t *testing.T, req map[string]any) {
		vars := req["variables"].(map[string]any)
		where := vars["where"].(map[string]any)
		assert.Equal(t, map[string]any{"gt": 10.0, "lt": 13.0}, where["block_height"])
		assert.Equal(t, map[string]any{"eq": true}, where["success"])
		or := where["response"].(map[string]any)["events"].(map[string]any)["GnoEvent"].(map[string]any)["_or"]
		assert.Equal(t, []any{
			map[string]any{"pkg_path": map[string]any{"eq": "gno.land/r/dev/gnotif/v0"}},
			map[string]any{"pkg_path": map[string]any{"eq": "gno.land/r/demo/game"}},
		}, or)
		assert.Equal(t, map[string]any{"gt": 10.0, "lt": 13.0}, vars["blocks"].(map[string]any)["height"])
	})

	b, err := c.Fetch(context.Background(), Window{From: 10, To: 12, Paths: paths})
	require.NoError(t, err)
	assert.Equal(t, int64(15), b.Latest)
	require.Len(t, b.Events, 2)
	assert.Equal(t, "TriggerDeclared", b.Events[0].Type)
	assert.Equal(t, trigger.Event{
		TxHash: "aGFzaDI=", Height: 12, TxIndex: 1, Index: 1,
		PkgPath: "gno.land/r/demo/game", Type: "TurnPlayed",
		Attrs: []trigger.Pair{{Key: "game", Value: "7"}, {Key: "next", Value: "g1bob"}},
	}, b.Events[1])
	assert.True(t, time.Date(2026, 10, 1, 12, 0, 5, 500_000_000, time.UTC).Equal(b.BlockTimes[12]))
}

func TestFetchNull(t *testing.T) {
	c := serve(t, http.StatusOK, `{"data":{"latestBlockHeight":15,"getTransactions":null,"getBlocks":null}}`, nil)
	b, err := c.Fetch(context.Background(), Window{From: 10, To: 12, Paths: []string{"gno.land/r/dev/gnotif/v0"}})
	require.NoError(t, err)
	assert.Equal(t, int64(15), b.Latest)
	assert.Empty(t, b.Events)
}

func TestFetchErrors(t *testing.T) {
	c := serve(t, http.StatusOK, `{"errors":[{"message":"max elements per query reached (10000)"}],"data":{"latestBlockHeight":15,"getTransactions":[]}}`, nil)
	_, err := c.Fetch(context.Background(), Window{From: 10, To: 12, Paths: []string{"gno.land/r/dev/gnotif/v0"}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrQuery), err)
	assert.Contains(t, err.Error(), "max elements")
}

func TestFetchStatus(t *testing.T) {
	c := serve(t, http.StatusBadGateway, `bad gateway`, nil)
	_, err := c.Fetch(context.Background(), Window{From: 10, To: 12, Paths: []string{"gno.land/r/dev/gnotif/v0"}})
	assert.True(t, errors.Is(err, ErrQuery), err)
}

func TestLatest(t *testing.T) {
	c := serve(t, http.StatusOK, `{"data":{"latestBlockHeight":42}}`, func(t *testing.T, req map[string]any) {
		assert.Contains(t, req["query"], "latestBlockHeight")
	})
	h, err := c.Latest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), h)
}
