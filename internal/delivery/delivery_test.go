package delivery

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type answer struct {
	status int
	header http.Header
}

type pushService struct {
	srv      *httptest.Server
	mu       sync.Mutex
	answers  map[string]answer
	requests []*http.Request
}

func newPushService(t *testing.T) *pushService {
	t.Helper()
	p := &pushService{answers: map[string]answer{}}
	p.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.requests = append(p.requests, r.Clone(context.Background()))
		a, ok := p.answers[r.URL.Path]
		if !ok {
			a = answer{status: http.StatusCreated}
		}
		maps.Copy(w.Header(), a.header)
		w.WriteHeader(a.status)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *pushService) answer(path string, a answer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answers[path] = a
}

type harness struct {
	push   *pushService
	store  *store.Store
	sender *Sender
	log    *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "gnotif.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	require.NoError(t, st.Update(ctx, func(tx *store.Tx) error {
		return tx.PutTrigger(trigger.Trigger{ID: "t1", Target: "gno.land/r/demo/game", Event: "TurnPlayed", Param: "next", Title: "Your turn", Link: "/", Declarer: "g1"})
	}))
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	p := newPushService(t)
	var log bytes.Buffer
	s := New(Config{
		Store:   st,
		Client:  p.srv.Client(),
		Keys:    Keys{Public: pub, Private: priv},
		Subject: "test@gnotif.example",
		TTL:     24 * time.Hour,
		Now:     func() time.Time { return now },
		Log:     slog.New(slog.NewTextHandler(&log, nil)),
	})
	return &harness{push: p, store: st, sender: s, log: &log}
}

// queue subscribes endpoint with fresh browser keys and queues one push for it.
func (h *harness) queue(t *testing.T, endpoint string, created time.Time, attempts int) {
	t.Helper()
	h.queuePush(t, queued{endpoint: endpoint, created: created, attempts: attempts})
}

// queued describes a push for queuePush. Empty fields get the defaults
// queue uses: fresh browser keys and a "Your turn" notification.
type queued struct {
	endpoint     string
	created      time.Time
	attempts     int
	p256dh       string
	notification trigger.Notification
}

func (h *harness) queuePush(t *testing.T, q queued) {
	t.Helper()
	ctx := context.Background()
	endpoint, created, attempts := q.endpoint, q.created, q.attempts
	if q.p256dh == "" {
		key, err := ecdh.P256().GenerateKey(rand.Reader)
		require.NoError(t, err)
		q.p256dh = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	}
	if q.notification == (trigger.Notification{}) {
		q.notification = trigger.Notification{Title: "Your turn", Body: "Game 7", Link: "/?game=7"}
	}
	auth := make([]byte, 16)
	rand.Read(auth)
	require.NoError(t, h.store.PutSubscription(ctx, store.Subscription{
		Endpoint: endpoint,
		P256dh:   q.p256dh,
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}))
	require.NoError(t, h.store.ReplaceOptins(ctx, endpoint, []store.Optin{{TriggerID: "t1", Value: endpoint}}))
	var sub int64
	require.NoError(t, h.store.Update(ctx, func(tx *store.Tx) error {
		ids, err := tx.Subscribers("t1", endpoint)
		if err != nil {
			return err
		}
		sub = ids[0]
		_, err = tx.Enqueue(store.Push{
			SubscriptionID: sub, TriggerID: "t1", TxHash: endpoint,
			Notification: q.notification,
			CreatedAt:    created,
		})
		return err
	}))
	for range attempts {
		due, err := h.store.Due(ctx, created.Add(10*time.Hour), 100)
		require.NoError(t, err)
		for _, d := range due {
			if d.SubscriptionID == sub {
				require.NoError(t, h.store.Reschedule(ctx, d.ID, created))
			}
		}
	}
}

// delivery returns the queued push for endpoint due at or before at.
func (h *harness) delivery(t *testing.T, endpoint string, at time.Time) (store.Delivery, bool) {
	t.Helper()
	due, err := h.store.Due(context.Background(), at, 100)
	require.NoError(t, err)
	for _, d := range due {
		if d.Subscription.Endpoint == endpoint {
			return d, true
		}
	}
	return store.Delivery{}, false
}

func (h *harness) subscribed(t *testing.T, endpoint string) bool {
	t.Helper()
	err := h.store.ReplaceOptins(context.Background(), endpoint, nil)
	if errors.Is(err, store.ErrUnknownSubscription) {
		return false
	}
	require.NoError(t, err)
	return true
}

func closedEndpoint(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "https://" + addr + "/push/closed"
}

func TestFlushOutcomes(t *testing.T) {
	type want struct {
		dueIn      time.Duration // -1: no delivery left
		attempts   int
		subscribed bool
		errorLog   bool
	}
	cases := map[string]struct {
		status   int
		header   http.Header
		endpoint func(h *harness) string
		want     want
	}{
		"201":           {status: 201, want: want{dueIn: -1, subscribed: true}},
		"404":           {status: 404, want: want{dueIn: -1, subscribed: false}},
		"410":           {status: 410, want: want{dueIn: -1, subscribed: false}},
		"429":           {status: 429, header: http.Header{"Retry-After": {"120"}}, want: want{dueIn: 120 * time.Second, attempts: 1, subscribed: true}},
		"500":           {status: 500, want: want{dueIn: 30 * time.Second, attempts: 1, subscribed: true}},
		"400":           {status: 400, want: want{dueIn: -1, subscribed: true, errorLog: true}},
		"network error": {endpoint: func(*harness) string { return closedEndpoint(t) }, want: want{dueIn: 30 * time.Second, attempts: 1, subscribed: true}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			endpoint := h.push.srv.URL + "/push/" + strings.ReplaceAll(name, " ", "-")
			if tc.endpoint != nil {
				endpoint = tc.endpoint(h)
			} else {
				h.push.answer("/push/"+name, answer{status: tc.status, header: tc.header})
			}
			h.queue(t, endpoint, now, 0)

			require.NoError(t, h.sender.Flush(context.Background()))

			if tc.want.dueIn < 0 {
				_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
				assert.False(t, left, "delivery should be gone")
			} else {
				_, early := h.delivery(t, endpoint, now.Add(tc.want.dueIn-time.Second))
				assert.False(t, early, "delivery due too early")
				d, ok := h.delivery(t, endpoint, now.Add(tc.want.dueIn))
				require.True(t, ok, "delivery not rescheduled at Now + %s", tc.want.dueIn)
				assert.Equal(t, tc.want.attempts, d.Attempts)
			}
			assert.Equal(t, tc.want.subscribed, h.subscribed(t, endpoint))
			assert.Equal(t, tc.want.errorLog, strings.Contains(h.log.String(), "level=ERROR"), h.log.String())
		})
	}
}

func TestBackoff(t *testing.T) {
	cases := map[string]struct {
		attempts int
		wait     time.Duration
	}{
		"three attempts": {3, 240 * time.Second},
		"ten attempts":   {10, time.Hour},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			endpoint := h.push.srv.URL + "/push/fail"
			h.push.answer("/push/fail", answer{status: 500})
			h.queue(t, endpoint, now, tc.attempts)
			require.NoError(t, h.sender.Flush(context.Background()))
			_, early := h.delivery(t, endpoint, now.Add(tc.wait-time.Second))
			assert.False(t, early)
			d, ok := h.delivery(t, endpoint, now.Add(tc.wait))
			require.True(t, ok)
			assert.Equal(t, tc.attempts+1, d.Attempts)
		})
	}
}

func TestDropsAfter24h(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/old"
	h.push.answer("/push/old", answer{status: 500})
	h.queue(t, endpoint, now.Add(-24*time.Hour-time.Second), 0)
	require.NoError(t, h.sender.Flush(context.Background()))
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left)
}

// A push left in the outbox through a day of downtime is stale even when the
// push service would accept it.
func TestDropsUnsentAfter24h(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/stale"
	h.queue(t, endpoint, now.Add(-24*time.Hour-time.Second), 0)
	require.NoError(t, h.sender.Flush(context.Background()))
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left)
	h.push.mu.Lock()
	defer h.push.mu.Unlock()
	assert.Empty(t, h.push.requests)
}

func TestRequestHeaders(t *testing.T) {
	h := newHarness(t)
	h.queue(t, h.push.srv.URL+"/push/ok", now, 0)
	require.NoError(t, h.sender.Flush(context.Background()))
	h.push.mu.Lock()
	defer h.push.mu.Unlock()
	require.Len(t, h.push.requests, 1)
	r := h.push.requests[0]
	assert.Equal(t, "86400", r.Header.Get("TTL"))
	assert.Equal(t, "aes128gcm", r.Header.Get("Content-Encoding"))
	assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "vapid t="), r.Header.Get("Authorization"))
}

// A link within the 1,024-byte render limit must fit one push record even
// when every byte is a character JSON may escape.
func TestFlushSendsLinkFullOfAmpersands(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/amp"
	link := "/?q=" + strings.Repeat("&", 1000)
	h.queuePush(t, queued{endpoint: endpoint, created: now, notification: trigger.Notification{Title: "Your turn", Body: "Game 7", Link: link}})

	require.NoError(t, h.sender.Flush(context.Background()))
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left, h.log.String())
	h.push.mu.Lock()
	defer h.push.mu.Unlock()
	assert.Len(t, h.push.requests, 1)
}

// A push webpush-go refuses before any request, here for a browser key off
// the P-256 curve, fails the same way on every retry.
func TestDropsPushRefusedBeforeSending(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/badkey"
	offCurve := base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...))
	h.queuePush(t, queued{endpoint: endpoint, created: now, p256dh: offCurve})

	require.NoError(t, h.sender.Flush(context.Background()))
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left, "refused push should be dropped")
	assert.True(t, h.subscribed(t, endpoint))
	assert.Contains(t, h.log.String(), "level=ERROR")
	h.push.mu.Lock()
	defer h.push.mu.Unlock()
	assert.Empty(t, h.push.requests)
}

// A push endpoint lets anyone holding it change that browser's opt-ins, so
// it stays out of the logs.
func TestNetworkErrorLogOmitsEndpoint(t *testing.T) {
	h := newHarness(t)
	endpoint := closedEndpoint(t) + "/SECRET-TOKEN"
	h.queue(t, endpoint, now, 0)

	require.NoError(t, h.sender.Flush(context.Background()))
	require.Contains(t, h.log.String(), "push failed")
	assert.NotContains(t, h.log.String(), "SECRET-TOKEN")
}

func TestEncodePayload(t *testing.T) {
	cases := map[string]struct {
		n    trigger.Notification
		want string
	}{
		"markup stays raw": {
			n:    trigger.Notification{Title: "A & B", Body: "<b>7</b>", Link: "/?a=1&b=2"},
			want: `{"title":"A & B","body":"<b>7</b>","link":"/?a=1&b=2"}`,
		},
		"oversized link becomes the root": {
			n:    trigger.Notification{Title: "Your turn", Body: "Game 7", Link: "/" + strings.Repeat("\x01", 1000)},
			want: `{"title":"Your turn","body":"Game 7","link":"/"}`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := encodePayload(tc.n)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
			assert.LessOrEqual(t, len(got), maxPayload)
		})
	}
}

// cancelAfterAccept cancels a context as soon as the push service has
// answered, the way a SIGTERM can land mid-send.
type cancelAfterAccept struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (c cancelAfterAccept) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := c.base.RoundTrip(r)
	c.cancel()
	return resp, err
}

func TestShutdownFinishesInFlightPush(t *testing.T) {
	h := newHarness(t)
	h.queue(t, h.push.srv.URL+"/push/first", now, 0)
	h.queue(t, h.push.srv.URL+"/push/second", now, 0)
	ctx, cancel := context.WithCancel(context.Background())
	client := *h.push.srv.Client()
	client.Transport = cancelAfterAccept{base: client.Transport, cancel: cancel}
	h.sender.cfg.Client = &client

	err := h.sender.Flush(ctx)
	require.ErrorIs(t, err, context.Canceled)
	_, left := h.delivery(t, h.push.srv.URL+"/push/first", now)
	assert.False(t, left, "an accepted push must be recorded as sent")

	h.sender.cfg.Client = h.push.srv.Client()
	require.NoError(t, h.sender.Flush(context.Background()))
	h.push.mu.Lock()
	defer h.push.mu.Unlock()
	paths := make([]string, 0, len(h.push.requests))
	for _, r := range h.push.requests {
		paths = append(paths, r.URL.Path)
	}
	assert.Equal(t, []string{"/push/first", "/push/second"}, paths)
}

func TestRunFlushesOnWake(t *testing.T) {
	h := newHarness(t)
	wake := make(chan struct{}, 1)
	h.sender.cfg.Wake = wake
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.sender.Run(ctx) }()

	h.queue(t, h.push.srv.URL+"/push/ok", now, 0)
	wake <- struct{}{}
	require.Eventually(t, func() bool {
		h.push.mu.Lock()
		defer h.push.mu.Unlock()
		return len(h.push.requests) == 1
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	require.Eventually(t, func() bool {
		select {
		case err := <-done:
			return err == nil
		default:
			return false
		}
	}, 2*time.Second, 10*time.Millisecond)
}
