package delivery

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/store/storetest"
	"github.com/gfanton/gnotif/internal/trigger"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type answer struct {
	status int
	header http.Header
}

// request is a request a push service received, with its body.
type request struct {
	*http.Request
	body []byte
}

type pushService struct {
	srv      *httptest.Server
	mu       sync.Mutex
	answers  map[string]answer
	requests []request
	gate     chan struct{} // non-nil: requests wait until it is closed
}

func newPushService(t *testing.T) *pushService {
	t.Helper()
	p := &pushService{answers: map[string]answer{}}
	p.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		p.mu.Lock()
		p.requests = append(p.requests, request{r.Clone(context.Background()), body})
		a, ok := p.answers[r.URL.Path]
		gate := p.gate
		p.mu.Unlock()
		if gate != nil {
			select {
			case <-gate:
			case <-r.Context().Done():
			}
		}
		if !ok {
			a = answer{status: http.StatusCreated}
		}
		maps.Copy(w.Header(), a.header)
		w.WriteHeader(a.status)
	}))
	t.Cleanup(p.srv.Close)
	// Close waits for the requests in progress.
	t.Cleanup(p.unblock)
	return p
}

func (p *pushService) answer(path string, a answer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answers[path] = a
}

// block holds every request from now on until unblock.
func (p *pushService) block() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gate = make(chan struct{})
}

// unblock answers the held requests and lets later ones through.
func (p *pushService) unblock() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.gate != nil {
		close(p.gate)
		p.gate = nil
	}
}

func (p *pushService) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func (p *pushService) paths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.requests))
	for _, r := range p.requests {
		out = append(out, r.URL.Path)
	}
	return out
}

// host is the push service's entry in an allowlist.
func (p *pushService) host() string {
	return strings.TrimPrefix(p.srv.URL, "https://")
}

// logBuffer holds the sender's log, which routines write while a test reads.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// browser holds a subscribed browser's secrets, to read the pushes it gets.
type browser struct {
	key  *ecdh.PrivateKey
	auth []byte
}

type harness struct {
	push     *pushService
	services []*pushService
	store    *store.Store
	sender   *Sender
	log      *logBuffer
	browsers map[string]browser // by endpoint
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessOn(t, filepath.Join(t.TempDir(), "gnotif.db"))
}

// newHarnessOn is newHarness on the store at dsn.
func newHarnessOn(t *testing.T, dsn string) *harness {
	t.Helper()
	st, err := store.Open(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	h := &harness{store: st, log: &logBuffer{}, browsers: map[string]browser{}}
	h.push = h.newService(t)
	h.sender = New(Config{
		Store:     st,
		Client:    h.push.srv.Client(),
		Keys:      Keys{Public: pub, Private: priv},
		Subject:   "test@gnotif.example",
		TTL:       24 * time.Hour,
		PushHosts: []string{h.push.host()},
		Now:       func() time.Time { return now },
		Log:       slog.New(slog.NewTextHandler(h.log, nil)),
	})
	return h
}

// newService starts another push service. Its requests are let through when
// a running sender stops. httptest servers share one certificate, so the
// sender's client trusts them all.
func (h *harness) newService(t *testing.T) *pushService {
	t.Helper()
	p := newPushService(t)
	h.services = append(h.services, p)
	return p
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

// newPush subscribes q.endpoint and returns a push for it, held nowhere.
func (h *harness) newPush(t *testing.T, q queued) store.Push {
	t.Helper()
	ctx := context.Background()
	b := browser{auth: make([]byte, 16)}
	rand.Read(b.auth)
	if q.p256dh == "" {
		var err error
		b.key, err = ecdh.P256().GenerateKey(rand.Reader)
		require.NoError(t, err)
		q.p256dh = base64.RawURLEncoding.EncodeToString(b.key.PublicKey().Bytes())
	}
	if q.notification == (trigger.Notification{}) {
		q.notification = trigger.Notification{Title: "Your turn", Body: "Game 7", Link: "/?game=7"}
	}
	sub := store.Subscription{Endpoint: q.endpoint, P256dh: q.p256dh, Auth: base64.RawURLEncoding.EncodeToString(b.auth)}
	require.NoError(t, h.store.PutSubscription(ctx, sub))
	res, err := h.store.Exec(ctx, []store.Statement{{SQL: `SELECT id FROM subscriptions WHERE endpoint = ?`, Args: []any{q.endpoint}}})
	require.NoError(t, err)
	id, ok := res[0].Rows[0]["id"].(int64)
	require.True(t, ok)
	h.browsers[q.endpoint] = b
	return store.Push{
		SubscriptionID: id, Subscription: sub, TriggerID: "t1", TxHash: q.endpoint,
		Notification: q.notification, CreatedAt: q.created,
	}
}

// subscribe subscribes endpoint with fresh browser keys and returns a push
// for it, created now.
func (h *harness) subscribe(t *testing.T, endpoint string) store.Push {
	t.Helper()
	return h.newPush(t, queued{endpoint: endpoint, created: now})
}

// queue subscribes endpoint with fresh browser keys and queues one push for it.
func (h *harness) queue(t *testing.T, endpoint string, created time.Time, attempts int) {
	t.Helper()
	h.queuePush(t, queued{endpoint: endpoint, created: created, attempts: attempts})
}

// queuePush writes a push to the outbox, due at q.created.
func (h *harness) queuePush(t *testing.T, q queued) {
	t.Helper()
	p := h.newPush(t, q)
	require.NoError(t, h.store.Queue(context.Background(), []store.Pending{{Push: p, Attempts: q.attempts, NextAttempt: q.created}}))
}

// numbered returns n pushes like p, told apart by their transaction and
// titled with their number, from first.
func numbered(p store.Push, first, n int) []store.Push {
	out := make([]store.Push, n)
	for i := range out {
		num := strconv.Itoa(first + i)
		out[i] = p
		out[i].TxHash = "tx" + num
		out[i].Notification.Title = num
	}
	return out
}

// flush sends every due outbox push once, as a routine does.
func (h *harness) flush(t *testing.T) {
	t.Helper()
	due, err := h.store.Due(context.Background(), now, memoryCap)
	require.NoError(t, err)
	for _, d := range due {
		require.NoError(t, h.sender.deliver(context.Background(), fromRow(d)))
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

// outbox returns the titles of the outbox's pushes, earliest due first.
func (h *harness) outbox(t *testing.T) []string {
	t.Helper()
	due, err := h.store.Due(context.Background(), now.Add(48*time.Hour), memoryCap)
	require.NoError(t, err)
	out := make([]string, 0, len(due))
	for _, d := range due {
		out = append(out, d.Notification.Title)
	}
	return out
}

// outboxWrites returns how many rows were ever written to the outbox: its
// AUTOINCREMENT sequence.
func (h *harness) outboxWrites(t *testing.T) int64 {
	t.Helper()
	res, err := h.store.Exec(context.Background(), []store.Statement{{SQL: `SELECT seq FROM sqlite_sequence WHERE name = 'outbox'`}})
	require.NoError(t, err)
	if len(res[0].Rows) == 0 {
		return 0
	}
	seq, ok := res[0].Rows[0]["seq"].(int64)
	require.True(t, ok)
	return seq
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

// titles returns the title of every push p received for endpoint, in the
// order p received them.
func (h *harness) titles(t *testing.T, p *pushService, endpoint string) []string {
	t.Helper()
	u, err := url.Parse(endpoint)
	require.NoError(t, err)
	p.mu.Lock()
	requests := slices.Clone(p.requests)
	p.mu.Unlock()
	var out []string
	for _, r := range requests {
		if r.URL.Path == u.Path {
			out = append(out, h.browsers[endpoint].title(t, r.body))
		}
	}
	return out
}

// title decrypts an aes128gcm push body (RFC 8291) as b and returns its
// notification's title.
func (b browser) title(t *testing.T, body []byte) string {
	t.Helper()
	require.Greater(t, len(body), 21)
	salt, idLen := body[:16], int(body[20])
	serverKey, sealed := body[21:21+idLen], body[21+idLen:]
	pub, err := ecdh.P256().NewPublicKey(serverKey)
	require.NoError(t, err)
	secret, err := b.key.ECDH(pub)
	require.NoError(t, err)
	ikm, err := hkdf.Key(sha256.New, secret, b.auth, "WebPush: info\x00"+string(b.key.PublicKey().Bytes())+string(serverKey), 32)
	require.NoError(t, err)
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	require.NoError(t, err)
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	require.NoError(t, err)
	block, err := aes.NewCipher(cek)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	require.NoError(t, err)
	var n trigger.Notification
	require.NoError(t, json.Unmarshal(plain[:bytes.LastIndexByte(plain, 2)], &n))
	return n.Title
}

// running is a sender's Run in progress.
type running struct {
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

func (r *running) returned() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// start runs s until ctx ends, stop is called, or the test ends.
func (h *harness) start(t *testing.T, ctx context.Context, s *Sender) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	r := &running{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.err = s.Run(ctx)
	}()
	t.Cleanup(func() { h.stop(t, r) })
	return r
}

// stop cancels r, lets every held request through, and waits for Run to
// return.
func (h *harness) stop(t *testing.T, r *running) {
	t.Helper()
	r.cancel()
	for _, p := range h.services {
		p.unblock()
	}
	h.wait(t, r)
}

// wait waits for Run to return, and checks it returned no error.
func (h *harness) wait(t *testing.T, r *running) {
	t.Helper()
	require.Eventually(t, r.returned, 15*time.Second, 5*time.Millisecond, "Run did not return")
	require.NoError(t, r.err)
}

// drive runs the routines of h.sender without Run, so that only the test
// checks the outbox, one check at a time. The returned check checks it and
// starts the routines of the services it found. The routines stop at the
// test's end.
func (h *harness) drive(t *testing.T) (check func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var routines sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		for _, p := range h.services {
			p.unblock()
		}
		routines.Wait()
	})
	return func() {
		h.sender.check(ctx)
		h.sender.start(ctx, &routines)
	}
}

// outboxProxy stands between a sender and a remote store. It records the
// most rows one Due read returned, and can hold back the answer to one.
type outboxProxy struct {
	url     string
	armed   atomic.Bool
	reached chan struct{} // closed once the held read has its answer
	release chan struct{} // closed to send the held answer
	once    sync.Once
	mu      sync.Mutex
	most    int
}

func newOutboxProxy(t *testing.T) *outboxProxy {
	t.Helper()
	upstream := storetest.NewServer(t)
	p := &outboxProxy{reached: make(chan struct{}), release: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}
		resp, err := http.Post(upstream.URL, "application/json", bytes.NewReader(body))
		if !assert.NoError(t, err) {
			return
		}
		defer resp.Body.Close()
		out, err := io.ReadAll(resp.Body)
		if !assert.NoError(t, err) {
			return
		}
		if bytes.Contains(body, []byte("FROM outbox o JOIN")) {
			var results []struct{ Rows []json.RawMessage }
			if !assert.NoError(t, json.Unmarshal(out, &results)) || !assert.Len(t, results, 1) {
				return
			}
			p.mu.Lock()
			p.most = max(p.most, len(results[0].Rows))
			p.mu.Unlock()
			if p.armed.CompareAndSwap(true, false) {
				close(p.reached)
				<-p.release
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(out)
	}))
	t.Cleanup(srv.Close)
	// Close waits for the held answer.
	t.Cleanup(p.send)
	p.url = srv.URL
	return p
}

// hold holds back the answer to the next Due read.
func (p *outboxProxy) hold() { p.armed.Store(true) }

// send sends the held answer.
func (p *outboxProxy) send() { p.once.Do(func() { close(p.release) }) }

// mostRows returns the most rows one Due read returned.
func (p *outboxProxy) mostRows() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.most
}

// closed reports whether ch is closed.
func closed(ch <-chan struct{}) func() bool {
	return func() bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
}

// held returns how many pushes s holds in memory.
func held(s *Sender) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held
}

func closedEndpoint(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "https://" + addr + "/push/closed"
}

// A push answered the same way follows the same rule whether it was held in
// memory or read from the outbox.
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
		for _, origin := range []string{"outbox", "memory"} {
			t.Run(name+" from "+origin, func(t *testing.T) {
				h := newHarness(t)
				endpoint := h.push.srv.URL + "/push/" + strings.ReplaceAll(name, " ", "-")
				if tc.endpoint != nil {
					endpoint = tc.endpoint(h)
				} else {
					h.push.answer("/push/"+name, answer{status: tc.status, header: tc.header})
				}
				if origin == "outbox" {
					h.queue(t, endpoint, now, 0)
					h.flush(t)
				} else {
					require.NoError(t, h.sender.deliver(context.Background(), item{Push: h.subscribe(t, endpoint)}))
				}

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
			h.flush(t)
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
	h.flush(t)
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left)
}

// A push left in the outbox through a day of downtime is stale even when the
// push service would accept it.
func TestDropsUnsentAfter24h(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/stale"
	h.queue(t, endpoint, now.Add(-24*time.Hour-time.Second), 0)
	h.flush(t)
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left)
	assert.Zero(t, h.push.count())
}

func TestRequestHeaders(t *testing.T) {
	h := newHarness(t)
	h.queue(t, h.push.srv.URL+"/push/ok", now, 0)
	h.flush(t)
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

	h.flush(t)
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left, h.log.String())
	assert.Equal(t, 1, h.push.count())
}

// A push webpush-go refuses before any request, here for a browser key off
// the P-256 curve, fails the same way on every retry.
func TestDropsPushRefusedBeforeSending(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/badkey"
	offCurve := base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...))
	h.queuePush(t, queued{endpoint: endpoint, created: now, p256dh: offCurve})

	h.flush(t)
	_, left := h.delivery(t, endpoint, now.Add(48*time.Hour))
	assert.False(t, left, "refused push should be dropped")
	assert.True(t, h.subscribed(t, endpoint))
	assert.Contains(t, h.log.String(), "level=ERROR")
	assert.Zero(t, h.push.count())
}

// A push endpoint lets anyone holding it change that browser's opt-ins, so
// it stays out of the logs.
func TestNetworkErrorLogOmitsEndpoint(t *testing.T) {
	h := newHarness(t)
	endpoint := closedEndpoint(t) + "/SECRET-TOKEN"
	h.queue(t, endpoint, now, 0)

	h.flush(t)
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
	first, second := h.push.srv.URL+"/push/first", h.push.srv.URL+"/push/second"
	h.queue(t, first, now, 0)
	h.queue(t, second, now, 0)
	ctx, cancel := context.WithCancel(context.Background())
	client := *h.push.srv.Client()
	client.Transport = cancelAfterAccept{base: client.Transport, cancel: cancel}
	cfg := h.sender.cfg
	h.sender.cfg.Client = &client

	h.wait(t, h.start(t, ctx, h.sender))
	_, left := h.delivery(t, first, now)
	assert.False(t, left, "an accepted push must be recorded as sent")
	_, left = h.delivery(t, second, now)
	assert.True(t, left, "a push from the outbox stays there at stop")
	assert.Len(t, h.outbox(t), 1, "a push from the outbox is not written again at stop")

	h.start(t, context.Background(), New(cfg))
	require.Eventually(t, func() bool { return h.push.count() == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"/push/first", "/push/second"}, h.push.paths())
}

func TestSuccessWritesNothing(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/ok"
	p := h.subscribe(t, endpoint)
	r := h.start(t, context.Background(), h.sender)

	h.sender.Send(context.Background(), []store.Push{p})
	require.Eventually(t, func() bool { return h.push.count() == 1 }, 5*time.Second, 5*time.Millisecond)
	h.stop(t, r)

	assert.Equal(t, []string{"Your turn"}, h.titles(t, h.push, endpoint))
	assert.Zero(t, h.outboxWrites(t), "a sent push must never reach the outbox")
	assert.Zero(t, held(h.sender))
}

func TestRetryableFailureWritesOneRow(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/busy"
	h.push.answer("/push/busy", answer{status: 503, header: http.Header{"Retry-After": {"120"}}})
	p := h.subscribe(t, endpoint)
	r := h.start(t, context.Background(), h.sender)

	h.sender.Send(context.Background(), []store.Push{p})
	require.Eventually(t, func() bool {
		_, ok := h.delivery(t, endpoint, now.Add(120*time.Second))
		return ok
	}, 5*time.Second, 5*time.Millisecond)
	h.stop(t, r)

	_, early := h.delivery(t, endpoint, now.Add(119*time.Second))
	assert.False(t, early, "due before Retry-After")
	d, _ := h.delivery(t, endpoint, now.Add(120*time.Second))
	assert.Equal(t, 1, d.Attempts)
	assert.Equal(t, []string{"Your turn"}, h.outbox(t))
	assert.Equal(t, 1, h.push.count())
}

func TestRoutedPerPushService(t *testing.T) {
	h := newHarness(t)
	other := h.newService(t)
	h.sender.cfg.PushHosts = []string{h.push.host(), other.host()}
	a, b := h.push.srv.URL+"/push/a", other.srv.URL+"/push/b"
	pushes := []store.Push{h.subscribe(t, a), h.subscribe(t, b)}
	h.start(t, context.Background(), h.sender)

	h.sender.Send(context.Background(), pushes)
	require.Eventually(t, func() bool { return h.push.count() == 1 && other.count() == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"/push/a"}, h.push.paths())
	assert.Equal(t, []string{"/push/b"}, other.paths())
	h.sender.mu.Lock()
	defer h.sender.mu.Unlock()
	assert.ElementsMatch(t, []string{h.push.host(), other.host()}, slices.Collect(maps.Keys(h.sender.services)))
}

func TestBlockedServiceDoesNotDelayOther(t *testing.T) {
	h := newHarness(t)
	other := h.newService(t)
	h.sender.cfg.PushHosts = []string{h.push.host(), other.host()}
	pushes := []store.Push{h.subscribe(t, h.push.srv.URL+"/push/a"), h.subscribe(t, other.srv.URL+"/push/b")}
	h.push.block()
	h.start(t, context.Background(), h.sender)

	h.sender.Send(context.Background(), pushes)
	require.Eventually(t, func() bool { return other.count() == 1 }, 5*time.Second, 5*time.Millisecond,
		"the push to the free service waits for the blocked one")
	assert.Equal(t, 1, h.push.count())
}

func TestSubscriptionPushesKeepOrder(t *testing.T) {
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/ordered"
	p := h.subscribe(t, endpoint)
	h.start(t, context.Background(), h.sender)

	h.sender.Send(context.Background(), numbered(p, 1, 3))
	require.Eventually(t, func() bool { return h.push.count() == 3 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"1", "2", "3"}, h.titles(t, h.push, endpoint))
}

// A push service dropped from the allowlist after a browser subscribed on it
// still gets that browser's pushes.
func TestUnmatchedEndpointStillSent(t *testing.T) {
	h := newHarness(t)
	h.sender.cfg.PushHosts = []string{"fcm.googleapis.com"}
	p := h.subscribe(t, h.push.srv.URL+"/push/unlisted")
	h.start(t, context.Background(), h.sender)

	h.sender.Send(context.Background(), []store.Push{p})
	require.Eventually(t, func() bool { return h.push.count() == 1 }, 5*time.Second, 5*time.Millisecond)
}

func TestOutboxRowQueuedOnce(t *testing.T) {
	h := newHarness(t)
	h.queue(t, h.push.srv.URL+"/push/row", now, 0)
	h.push.block()
	check := h.drive(t)
	check()
	require.Eventually(t, func() bool { return h.push.count() == 1 }, 5*time.Second, 5*time.Millisecond)

	for range 3 {
		check()
	}
	h.push.unblock()
	require.Eventually(t, func() bool { return held(h.sender) == 0 }, 5*time.Second, 5*time.Millisecond)

	assert.Equal(t, 1, h.push.count())
	assert.Empty(t, h.outbox(t))
}

// A check's read can list a row whose outcome a routine stores before the
// read returns; on a remote store the window is a round trip.
func TestRowSettledDuringCheckNotQueuedAgain(t *testing.T) {
	proxy := newOutboxProxy(t)
	h := newHarnessOn(t, proxy.url)
	h.queue(t, h.push.srv.URL+"/push/row", now, 0)
	h.push.block()
	check := h.drive(t)
	check()
	require.Eventually(t, func() bool { return h.push.count() == 1 }, 5*time.Second, 5*time.Millisecond)

	proxy.hold()
	checked := make(chan struct{})
	go func() {
		defer close(checked)
		h.sender.check(context.Background())
	}()
	require.Eventually(t, closed(proxy.reached), 5*time.Second, 5*time.Millisecond, "the check never read the row")
	h.push.unblock()
	require.Eventually(t, func() bool { return held(h.sender) == 0 }, 5*time.Second, 5*time.Millisecond)
	proxy.send()
	require.Eventually(t, closed(checked), 5*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return held(h.sender) == 0 }, 5*time.Second, 5*time.Millisecond)

	assert.Equal(t, 1, h.push.count(), "the delivered row was sent again")
	assert.Empty(t, h.outbox(t))
}

// The outbox check fills memory up to the cap and leaves the other due rows
// where they are.
func TestOutboxCheckKeepsCap(t *testing.T) {
	h := newHarness(t)
	h.sender.Send(context.Background(), numbered(h.subscribe(t, h.push.srv.URL+"/push/full"), 1, memoryCap-1))
	for _, name := range []string{"x", "y", "z"} {
		h.queue(t, h.push.srv.URL+"/push/"+name, now, 0)
	}

	h.sender.check(context.Background())

	assert.Equal(t, memoryCap, held(h.sender))
	assert.Len(t, h.outbox(t), 3)
}

func TestOverCapGoesToOutbox(t *testing.T) {
	h := newHarness(t)
	p := h.subscribe(t, h.push.srv.URL+"/push/flood")
	h.push.block()
	r := h.start(t, context.Background(), h.sender)
	h.sender.Send(context.Background(), numbered(p, 1, memoryCap))
	require.Eventually(t, func() bool { return h.push.count() == 1 }, 5*time.Second, 5*time.Millisecond)

	h.sender.Send(context.Background(), numbered(p, memoryCap+1, 5))

	assert.Len(t, h.outbox(t), 5, "the push in flight counts against the cap")
	assert.Equal(t, memoryCap, held(h.sender))

	h.stop(t, r)
	res, err := h.store.Exec(context.Background(), []store.Statement{{SQL: `SELECT count(*) AS n FROM outbox`}})
	require.NoError(t, err)
	assert.EqualValues(t, memoryCap-1+5, res[0].Rows[0]["n"], "every queued push is written at stop")
}

func TestDropLogged(t *testing.T) {
	h := newHarness(t)
	p := h.subscribe(t, h.push.srv.URL+"/push/lost")
	h.sender.Send(context.Background(), numbered(p, 1, memoryCap))
	require.NoError(t, h.store.Close())

	h.sender.Send(context.Background(), numbered(p, memoryCap+1, 1))

	assert.Contains(t, h.log.String(), `level=ERROR msg="drop pushes" count=1`)
}

// stopWithQueued leaves a stopped sender that sent push "1" of three to
// endpoint while "2" and "3" waited behind it.
func stopWithQueued(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness(t)
	endpoint := h.push.srv.URL + "/push/stop"
	p := h.subscribe(t, endpoint)
	h.push.block()
	r := h.start(t, context.Background(), h.sender)
	h.sender.Send(context.Background(), numbered(p, 1, 3))
	require.Eventually(t, func() bool { return h.push.count() == 1 }, 5*time.Second, 5*time.Millisecond)

	r.cancel()
	require.Never(t, r.returned, 100*time.Millisecond, 5*time.Millisecond, "Run returned with a push in flight")
	h.push.unblock()
	h.wait(t, r)
	return h, endpoint
}

func TestStopWritesQueuedPushes(t *testing.T) {
	h, endpoint := stopWithQueued(t)

	assert.Equal(t, []string{"1"}, h.titles(t, h.push, endpoint))
	assert.Equal(t, []string{"2", "3"}, h.outbox(t))
	assert.Zero(t, held(h.sender))
}

func TestStartSendsWhatStopLeft(t *testing.T) {
	h, endpoint := stopWithQueued(t)

	r := h.start(t, context.Background(), New(h.sender.cfg))
	require.Eventually(t, func() bool { return len(h.outbox(t)) == 0 }, 5*time.Second, 5*time.Millisecond)
	h.stop(t, r)

	assert.Equal(t, []string{"1", "2", "3"}, h.titles(t, h.push, endpoint))
}

// The watcher's last window can reach Send after the sender stopped, on a
// context that has ended too.
func TestSendAfterStopGoesToOutbox(t *testing.T) {
	h := newHarness(t)
	p := h.subscribe(t, h.push.srv.URL+"/push/late")
	h.stop(t, h.start(t, context.Background(), h.sender))
	ended, cancel := context.WithCancel(context.Background())
	cancel()

	h.sender.Send(ended, []store.Push{p})

	assert.Equal(t, []string{"Your turn"}, h.outbox(t))
	assert.Zero(t, h.push.count())
	assert.Zero(t, held(h.sender))
}
