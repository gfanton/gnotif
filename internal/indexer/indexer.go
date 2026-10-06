// Package indexer reads realm events from a gnolang/tx-indexer GraphQL endpoint.
package indexer

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gfanton/gnotif/internal/trigger"
)

// ErrQuery reports an indexer answer that is not a complete result: a
// non-200 status, a GraphQL errors array, or a body that does not decode.
var ErrQuery = errors.New("indexer query failed")

// ErrTooLarge reports an answer the indexer cannot give whole, or the client
// will not read whole: a window with more transactions than one query
// returns, or a body over maxResponse. A narrower query can succeed.
var ErrTooLarge = errors.New("indexer answer too large")

const (
	maxResponse = 64 << 20

	// maxElementsMessage is the indexer's error when a query matches 10,000
	// transactions or more; it returns a partial result with it.
	maxElementsMessage = "max elements per query reached"
)

// Client queries one indexer.
type Client struct {
	url  string
	http *http.Client
}

// New returns a client for the GraphQL endpoint at url.
func New(url string, hc *http.Client) *Client {
	return &Client{url: url, http: hc}
}

// Window selects heights From < h <= To.
type Window struct {
	From, To int64
}

// TxRef names a transaction by its hash and its index in its block.
type TxRef struct {
	Hash  string
	Index int
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
// height at the time of the query. It covers every successful transaction in
// the window that has a GnoEvent, whatever the realm.
func (c *Client) Fetch(ctx context.Context, w Window) (Batch, error) {
	data, err := post[windowData](ctx, c, windowQuery, newWindowVars(w))
	if err != nil {
		return Batch{}, err
	}
	b := Batch{Latest: data.LatestBlockHeight, BlockTimes: make(map[int64]time.Time, len(data.GetBlocks))}
	for _, tx := range data.GetTransactions {
		b.Events = append(b.Events, gnoEvents(tx)...)
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

// BlockTxs lists the successful transactions of one block that have a
// GnoEvent, by index, with the block's time. It reads no events.
func (c *Client) BlockTxs(ctx context.Context, height int64) ([]TxRef, time.Time, error) {
	data, err := post[blockTxsData](ctx, c, blockTxsQuery, newBlockTxsVars(height))
	if err != nil {
		return nil, time.Time{}, err
	}
	if len(data.GetBlocks) != 1 {
		return nil, time.Time{}, fmt.Errorf("%w: block %d: got %d blocks", ErrQuery, height, len(data.GetBlocks))
	}
	at, err := time.Parse(time.RFC3339Nano, data.GetBlocks[0].Time)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("%w: block %d time: %v", ErrQuery, height, err)
	}
	txs := make([]TxRef, len(data.GetTransactions))
	for i, tx := range data.GetTransactions {
		txs[i] = TxRef{Hash: tx.Hash, Index: tx.Index}
	}
	slices.SortFunc(txs, func(a, b TxRef) int { return cmp.Compare(a.Index, b.Index) })
	return txs, at.UTC(), nil
}

// FetchTx reads the GnoEvents of one successful transaction, with the latest
// height at the time of the query. Batch.BlockTimes is empty. An answer
// without exactly one transaction is ErrQuery: a transaction BlockTxs listed
// can be missing only from an indexer that is behind.
func (c *Client) FetchTx(ctx context.Context, height int64, index int) (Batch, error) {
	data, err := post[txData](ctx, c, txQuery, newTxVars(height, index))
	if err != nil {
		return Batch{}, err
	}
	if n := len(data.GetTransactions); n != 1 {
		return Batch{}, fmt.Errorf("%w: block %d tx %d: got %d transactions", ErrQuery, height, index, n)
	}
	return Batch{
		Latest:     data.LatestBlockHeight,
		Events:     gnoEvents(data.GetTransactions[0]),
		BlockTimes: map[int64]time.Time{},
	}, nil
}

// gnoEvents keeps the GnoEvents of tx; Index is the event's position among
// all of the transaction's events.
func gnoEvents(tx transaction) []trigger.Event {
	var out []trigger.Event
	for i, ev := range tx.Response.Events {
		if ev.Typename != "GnoEvent" {
			continue
		}
		attrs := make([]trigger.Pair, len(ev.Attrs))
		for j, a := range ev.Attrs {
			attrs[j] = trigger.Pair{Key: a.Key, Value: a.Value}
		}
		out = append(out, trigger.Event{
			TxHash:  tx.Hash,
			Height:  tx.BlockHeight,
			TxIndex: tx.Index,
			Index:   i,
			PkgPath: ev.PkgPath,
			Type:    ev.Type,
			Attrs:   attrs,
		})
	}
	return out
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
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return zero, fmt.Errorf("%w: read: %v", ErrQuery, err)
	}
	if len(raw) > maxResponse {
		return zero, fmt.Errorf("%w: body over %d bytes", ErrTooLarge, maxResponse)
	}
	var out struct {
		Data   T `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("%w: decode: %v", ErrQuery, err)
	}
	if len(out.Errors) > 0 {
		msgs := make([]string, len(out.Errors))
		for i, e := range out.Errors {
			msgs[i] = e.Message
		}
		joined := strings.Join(msgs, "; ")
		if strings.Contains(joined, maxElementsMessage) {
			return zero, fmt.Errorf("%w: %s", ErrTooLarge, joined)
		}
		return zero, fmt.Errorf("%w: %s", ErrQuery, joined)
	}
	return out.Data, nil
}
