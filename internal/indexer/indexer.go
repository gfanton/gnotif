// Package indexer reads realm events from a gnolang/tx-indexer GraphQL endpoint.
package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gfanton/gnotif/internal/trigger"
)

// ErrQuery reports an indexer answer that is not a complete result: a
// non-200 status, a GraphQL errors array, or a body that does not decode.
var ErrQuery = errors.New("indexer query failed")

const maxResponse = 64 << 20

// Client queries one indexer.
type Client struct {
	url  string
	http *http.Client
}

// New returns a client for the GraphQL endpoint at url.
func New(url string, hc *http.Client) *Client {
	return &Client{url: url, http: hc}
}

// Window selects heights From < h <= To, and the transactions with a
// GnoEvent from one of Paths.
type Window struct {
	From, To int64
	Paths    []string
}

// Batch is the result of one window.
type Batch struct {
	Latest     int64
	Events     []trigger.Event // GnoEvents of successful transactions, in chain order
	BlockTimes map[int64]time.Time
}

// Latest returns the indexer's latest height.
func (c *Client) Latest(ctx context.Context) (int64, error) {
	data, err := post[latestData](ctx, c, latestQuery, nil)
	if err != nil {
		return 0, err
	}
	return data.LatestBlockHeight, nil
}

// Fetch reads the GnoEvents and block times of a window, with the latest
// height at the time of the query.
func (c *Client) Fetch(ctx context.Context, w Window) (Batch, error) {
	data, err := post[windowData](ctx, c, windowQuery, newWindowVars(w))
	if err != nil {
		return Batch{}, err
	}
	b := Batch{Latest: data.LatestBlockHeight, BlockTimes: make(map[int64]time.Time, len(data.GetBlocks))}
	for _, tx := range data.GetTransactions {
		for i, ev := range tx.Response.Events {
			if ev.Typename != "GnoEvent" {
				continue
			}
			attrs := make([]trigger.Pair, len(ev.Attrs))
			for j, a := range ev.Attrs {
				attrs[j] = trigger.Pair{Key: a.Key, Value: a.Value}
			}
			b.Events = append(b.Events, trigger.Event{
				TxHash:  tx.Hash,
				Height:  tx.BlockHeight,
				TxIndex: tx.Index,
				Index:   i,
				PkgPath: ev.PkgPath,
				Type:    ev.Type,
				Attrs:   attrs,
			})
		}
	}
	for _, blk := range data.GetBlocks {
		t, err := time.Parse(time.RFC3339Nano, blk.Time)
		if err != nil {
			return Batch{}, fmt.Errorf("%w: block %d time: %v", ErrQuery, blk.Height, err)
		}
		b.BlockTimes[blk.Height] = t.UTC()
	}
	return b, nil
}

func post[T any](ctx context.Context, c *Client, query string, vars any) (T, error) {
	var zero T
	body, err := json.Marshal(struct {
		Query     string `json:"query"`
		Variables any    `json:"variables,omitempty"`
	}{query, vars})
	if err != nil {
		return zero, fmt.Errorf("encode query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return zero, fmt.Errorf("%w: %v", ErrQuery, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("%w: status %d", ErrQuery, resp.StatusCode)
	}
	var out struct {
		Data   T `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponse)).Decode(&out); err != nil {
		return zero, fmt.Errorf("%w: decode: %v", ErrQuery, err)
	}
	if len(out.Errors) > 0 {
		msgs := make([]string, len(out.Errors))
		for i, e := range out.Errors {
			msgs[i] = e.Message
		}
		return zero, fmt.Errorf("%w: %s", ErrQuery, strings.Join(msgs, "; "))
	}
	return out.Data, nil
}
