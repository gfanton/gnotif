package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	c := serve(t, http.StatusOK, string(fixture), func(t *testing.T, req map[string]any) {
		vars := req["variables"].(map[string]any)
		where := vars["where"].(map[string]any)
		assert.Equal(t, map[string]any{"gt": 10.0, "lt": 13.0}, where["block_height"])
		assert.Equal(t, map[string]any{"eq": true}, where["success"])
		assert.Equal(t, map[string]any{"gt": 10.0, "lt": 13.0}, vars["blocks"].(map[string]any)["height"])
	})

	b, err := c.Fetch(context.Background(), Window{From: 10, To: 12})
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
	b, err := c.Fetch(context.Background(), Window{From: 10, To: 12})
	require.NoError(t, err)
	assert.Equal(t, int64(15), b.Latest)
	assert.Empty(t, b.Events)
}

func TestTooManyElements(t *testing.T) {
	c := serve(t, http.StatusOK, `{"errors":[{"message":"max elements per query reached (10000)"}],"data":{"latestBlockHeight":15,"getTransactions":[]}}`, nil)
	_, err := c.Fetch(context.Background(), Window{From: 10, To: 12})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooLarge), err)
	assert.False(t, errors.Is(err, ErrQuery), err)
}

func TestGraphQLErrorIsQuery(t *testing.T) {
	c := serve(t, http.StatusOK, `{"errors":[{"message":"boom"}],"data":null}`, nil)
	_, err := c.Fetch(context.Background(), Window{From: 10, To: 12})
	assert.True(t, errors.Is(err, ErrQuery), err)
	assert.False(t, errors.Is(err, ErrTooLarge), err)
	assert.Contains(t, err.Error(), "boom")
}

func TestTransientIsQuery(t *testing.T) {
	c := serve(t, http.StatusBadGateway, `bad gateway`, nil)
	_, err := c.Fetch(context.Background(), Window{From: 10, To: 12})
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

func TestFetchFilter(t *testing.T) {
	c := serve(t, http.StatusOK, `{"data":{"latestBlockHeight":15,"getTransactions":[
	  {"hash":"a","block_height":11,"index":0,"response":{"events":[{"__typename":"GnoEvent","type":"X","pkg_path":"gno.land/r/one","attrs":[]}]}},
	  {"hash":"b","block_height":11,"index":1,"response":{"events":[{"__typename":"GnoEvent","type":"Y","pkg_path":"gno.land/r/two","attrs":[]}]}}],
	  "getBlocks":[]}}`, func(t *testing.T, req map[string]any) {
		where := req["variables"].(map[string]any)["where"].(map[string]any)
		assert.Equal(t, map[string]any{
			"success":      map[string]any{"eq": true},
			"block_height": map[string]any{"gt": 10.0, "lt": 13.0},
			"response": map[string]any{"events": map[string]any{"GnoEvent": map[string]any{
				"pkg_path": map[string]any{"exists": true},
			}}},
		}, where)
		assert.NotContains(t, req["query"], "_or")
	})
	b, err := c.Fetch(context.Background(), Window{From: 10, To: 12})
	require.NoError(t, err)
	require.Len(t, b.Events, 2)
	assert.Equal(t, "gno.land/r/one", b.Events[0].PkgPath)
	assert.Equal(t, "gno.land/r/two", b.Events[1].PkgPath)
}

func TestTooLargeBody(t *testing.T) {
	c := serve(t, http.StatusOK, strings.Repeat(" ", maxResponse+1), nil)
	_, err := c.Fetch(context.Background(), Window{From: 10, To: 12})
	assert.True(t, errors.Is(err, ErrTooLarge), err)
	assert.False(t, errors.Is(err, ErrQuery), err)
}

func TestBlockTxs(t *testing.T) {
	c := serve(t, http.StatusOK, `{"data":{"getTransactions":[{"hash":"b","index":3},{"hash":"a","index":1}],
	  "getBlocks":[{"height":12,"time":"2026-10-01T12:00:05.5Z"}]}}`, func(t *testing.T, req map[string]any) {
		assert.NotContains(t, req["query"], "events")
		where := req["variables"].(map[string]any)["where"].(map[string]any)
		assert.Equal(t, map[string]any{"eq": 12.0}, where["block_height"])
		assert.Equal(t, map[string]any{"eq": true}, where["success"])
		assert.Contains(t, where, "response")
	})
	txs, at, err := c.BlockTxs(context.Background(), 12)
	require.NoError(t, err)
	assert.Equal(t, []TxRef{{Hash: "a", Index: 1}, {Hash: "b", Index: 3}}, txs)
	assert.True(t, time.Date(2026, 10, 1, 12, 0, 5, 500_000_000, time.UTC).Equal(at))
}

func TestBlockTxsMissingBlock(t *testing.T) {
	c := serve(t, http.StatusOK, `{"data":{"getTransactions":[],"getBlocks":[]}}`, nil)
	_, _, err := c.BlockTxs(context.Background(), 12)
	assert.True(t, errors.Is(err, ErrQuery), err)
}

func TestFetchTx(t *testing.T) {
	c := serve(t, http.StatusOK, `{"data":{"latestBlockHeight":15,"getTransactions":[
	  {"hash":"a","block_height":12,"index":0,"response":{"events":[
	    {"__typename":"StorageDepositEvent"},
	    {"__typename":"GnoEvent","type":"X","pkg_path":"gno.land/r/one","attrs":[{"key":"k","value":"v"}]}]}}]}}`,
		func(t *testing.T, req map[string]any) {
			where := req["variables"].(map[string]any)["where"].(map[string]any)
			assert.Equal(t, map[string]any{"eq": 12.0}, where["block_height"])
			assert.Equal(t, map[string]any{"eq": 0.0}, where["index"])
			assert.Equal(t, map[string]any{"eq": true}, where["success"])
		})
	b, err := c.FetchTx(context.Background(), 12, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(15), b.Latest)
	assert.Equal(t, []trigger.Event{{
		TxHash: "a", Height: 12, TxIndex: 0, Index: 1,
		PkgPath: "gno.land/r/one", Type: "X", Attrs: []trigger.Pair{{Key: "k", Value: "v"}},
	}}, b.Events)
}

func TestFetchTxNotOne(t *testing.T) {
	cases := map[string]string{
		"none": `{"data":{"latestBlockHeight":15,"getTransactions":[]}}`,
		"null": `{"data":{"latestBlockHeight":15,"getTransactions":null}}`,
		"two": `{"data":{"latestBlockHeight":15,"getTransactions":[
		  {"hash":"a","block_height":12,"index":0,"response":{"events":[]}},
		  {"hash":"b","block_height":12,"index":0,"response":{"events":[]}}]}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c := serve(t, http.StatusOK, body, nil)
			_, err := c.FetchTx(context.Background(), 12, 0)
			assert.True(t, errors.Is(err, ErrQuery), err)
			assert.False(t, errors.Is(err, ErrTooLarge), err)
		})
	}
}
