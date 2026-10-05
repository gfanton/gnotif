package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/trigger"
)

type published struct {
	header http.Header
	items  []struct{ Key, Frame string }
}

// receiver records each request; hold, when set, decides per request whether
// to wait for the client to give up instead of answering.
type receiver struct {
	mu   sync.Mutex
	seen int // requests received, recorded or held
	got  []published
	hold func(n int) bool
	srv  *httptest.Server
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		r.mu.Lock()
		hold := r.hold != nil && r.hold(r.seen)
		r.seen++
		if !hold {
			var p published
			p.header = req.Header.Clone()
			if json.Unmarshal(body, &p.items) != nil {
				p.items = nil
			}
			r.got = append(r.got, p)
		}
		r.mu.Unlock()
		if hold {
			<-req.Context().Done()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) batches() []published {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]published(nil), r.got...)
}

func run(t *testing.T, p *Publisher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func TestPublisherSendsBatchesInOrder(t *testing.T) {
	r := newReceiver(t)
	p := NewPublisher(PublisherConfig{URL: r.srv.URL, Token: "t0k", Client: r.srv.Client(), Timeout: 2 * time.Second, Log: slog.New(slog.DiscardHandler)})
	bob, alice, carol := testMatch("g1bob"), testMatch("g1alice"), testMatch("g1carol")
	p.Publish([]trigger.Match{bob})
	p.Publish([]trigger.Match{alice, carol})
	run(t, p)

	require.Eventually(t, func() bool { return len(r.batches()) == 2 }, 5*time.Second, 10*time.Millisecond)
	got := r.batches()
	item := func(m trigger.Match) struct{ Key, Frame string } {
		return struct{ Key, Frame string }{Key(m.TriggerID, m.Value), string(NotificationFrame(m))}
	}
	assert.Equal(t, []struct{ Key, Frame string }{item(bob)}, got[0].items)
	assert.Equal(t, []struct{ Key, Frame string }{item(alice), item(carol)}, got[1].items)
	assert.Equal(t, "Bearer t0k", got[0].header.Get("Authorization"))
	assert.Equal(t, "application/json", got[0].header.Get("Content-Type"))
}

func TestPublisherWithoutToken(t *testing.T) {
	r := newReceiver(t)
	p := NewPublisher(PublisherConfig{URL: r.srv.URL, Client: r.srv.Client(), Timeout: 2 * time.Second, Log: slog.New(slog.DiscardHandler)})
	p.Publish([]trigger.Match{testMatch("g1bob")})
	run(t, p)

	require.Eventually(t, func() bool { return len(r.batches()) == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Empty(t, r.batches()[0].header.Get("Authorization"))
}

func TestPublisherDropsWhenQueueFull(t *testing.T) {
	r := newReceiver(t)
	var log syncBuffer
	p := NewPublisher(PublisherConfig{URL: r.srv.URL, Client: r.srv.Client(), Timeout: 2 * time.Second, Log: slog.New(slog.NewTextHandler(&log, nil))})
	for range 65 {
		p.Publish([]trigger.Match{testMatch("g1bob")})
	}
	assert.Equal(t, 1, strings.Count(log.String(), "live publish queue full"))

	run(t, p)
	require.Eventually(t, func() bool { return len(r.batches()) == 64 }, 10*time.Second, 10*time.Millisecond)
	assert.Never(t, func() bool { return len(r.batches()) > 64 }, 300*time.Millisecond, 50*time.Millisecond)
}

func TestPublisherTimesOut(t *testing.T) {
	r := newReceiver(t)
	r.hold = func(n int) bool { return n == 0 }
	var log syncBuffer
	p := NewPublisher(PublisherConfig{URL: r.srv.URL, Client: r.srv.Client(), Timeout: 50 * time.Millisecond, Log: slog.New(slog.NewTextHandler(&log, nil))})
	held, next := testMatch("g1bob"), testMatch("g1alice")
	p.Publish([]trigger.Match{held})
	p.Publish([]trigger.Match{next})
	run(t, p)

	require.Eventually(t, func() bool { return len(r.batches()) == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, Key(next.TriggerID, next.Value), r.batches()[0].items[0].Key)
	assert.Contains(t, log.String(), "live publish failed")
}

// syncBuffer is a bytes.Buffer safe for the publisher's goroutine to write
// while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
