// Package delivery sends pushes through the browsers' push services and acts
// on their answers: done, retry later, or drop a dead subscription. Pushes go
// out from memory; the outbox holds the ones that could not.
package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/gfanton/gnotif/internal/pushhost"
	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

const (
	baseBackoff = 30 * time.Second
	maxBackoff  = time.Hour
	maxQueueAge = 24 * time.Hour
	checkEvery  = time.Second
	checkPage   = 100
	// memoryCap is the most pushes held in memory, queued or in flight, across
	// every push service.
	// ceiling: one shared cap; a hung push service under sustained load fills it and slows the others; upgrade: a cap per service
	memoryCap = 10_000
)

// Keys is a VAPID key pair, as webpush.GenerateVAPIDKeys returns it.
type Keys struct{ Public, Private string }

// Config holds the sender's dependencies and settings.
type Config struct {
	Store   *store.Store
	Client  *http.Client
	Keys    Keys
	Subject string // an email address or an https URL; webpush-go adds "mailto:" itself
	TTL     time.Duration
	// PushHosts is the allowlist of push services, as pushhost.Match reads
	// it. Each entry gets its own routine.
	PushHosts []string
	Now       func() time.Time
	Log       *slog.Logger
}

// Sender sends pushes on one routine per push service, each in the order it
// got them. Send is safe for concurrent use; Run is called once.
type Sender struct {
	cfg   Config
	added chan struct{} // a service waits for its routine

	mu sync.Mutex
	// services are keyed by allowlist entry, or by host for an endpoint
	// that matches none.
	services map[string]*service
	rows     map[int64]bool // ids of the outbox rows held
	// settled holds the outbox rows whose outcome was stored since the
	// current check started, which that check's read may still list as due.
	settled map[int64]bool
	after   store.DueKey // where the next check's page starts
	held    int          // pushes queued or in flight
	stopped bool
}

// service is one push service's queue.
type service struct {
	queue   []item
	wake    chan struct{} // the queue grew
	running bool          // its routine was started
}

// item is a push held in memory: new from Send, or read from the outbox.
type item struct {
	store.Push
	row      int64 // outbox id; 0 for a push not in the outbox
	attempts int
}

func fromRow(d store.Delivery) item {
	return item{
		Push: store.Push{
			SubscriptionID: d.SubscriptionID, Subscription: d.Subscription,
			Notification: d.Notification, CreatedAt: d.CreatedAt,
		},
		row:      d.ID,
		attempts: d.Attempts,
	}
}

// New returns a sender.
func New(cfg Config) *Sender {
	return &Sender{
		cfg:      cfg,
		added:    make(chan struct{}, 1),
		services: map[string]*service{},
		rows:     map[int64]bool{},
		settled:  map[int64]bool{},
	}
}

// NewClient returns the HTTP client for push services: a 10 s timeout and
// no redirects, since a push endpoint that redirects is not one to follow.
func NewClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Send queues pushes on their push services' routines and returns without
// waiting for them. A push past memoryCap, or sent once Run has stopped, is
// written to the outbox, due now.
func (s *Sender) Send(ctx context.Context, pushes []store.Push) {
	var spill []store.Pending
	s.mu.Lock()
	for _, p := range pushes {
		if s.stopped || s.held >= memoryCap {
			// A push spilled at the cap, like a retried one, can be sent after
			// a later push to the same subscription; accepted under overload.
			spill = append(spill, store.Pending{Push: p, NextAttempt: s.cfg.Now()})
			continue
		}
		s.hold(item{Push: p})
	}
	s.mu.Unlock()
	// A push handed over while the caller stops still reaches the outbox.
	s.write(context.WithoutCancel(ctx), spill)
}

// hold queues it on its push service's routine. s.mu is held.
func (s *Sender) hold(it item) {
	key := s.serviceOf(it.Subscription.Endpoint)
	svc := s.services[key]
	if svc == nil {
		svc = &service{wake: make(chan struct{}, 1)}
		s.services[key] = svc
		signal(s.added)
	}
	svc.queue = append(svc.queue, it)
	if it.row != 0 {
		s.rows[it.row] = true
	}
	s.held++
	signal(svc.wake)
}

// serviceOf names the routine that sends to endpoint: its allowlist entry, or
// its host when the allowlist no longer has it.
func (s *Sender) serviceOf(endpoint string) string {
	if entry, ok := pushhost.Match(endpoint, s.cfg.PushHosts); ok {
		return entry
	}
	if u, err := url.Parse(endpoint); err == nil {
		return u.Host
	}
	return ""
}

// signal wakes ch's reader without blocking; ch holds one signal.
func signal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// Run starts each push service's routine on its first push, and hands the
// outbox's due pushes to the routines at once and every second. When ctx
// ends, it waits for each routine's send in flight, writes the pushes still
// queued to the outbox, and returns. A push read from the outbox keeps its
// row and is not written again.
func (s *Sender) Run(ctx context.Context) error {
	var routines sync.WaitGroup
	ticker := time.NewTicker(checkEvery)
	defer ticker.Stop()
	s.check(ctx)
	for {
		s.start(ctx, &routines)
		select {
		case <-ctx.Done():
			routines.Wait()
			s.stop(context.WithoutCancel(ctx))
			return nil
		case <-s.added:
		case <-ticker.C:
			s.check(ctx)
		}
	}
}

// start runs a routine for each service that has none.
func (s *Sender) start(ctx context.Context, routines *sync.WaitGroup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, svc := range s.services {
		if !svc.running {
			svc.running = true
			routines.Go(func() { s.serve(ctx, svc) })
		}
	}
}

// serve sends svc's pushes in order until ctx ends.
func (s *Sender) serve(ctx context.Context, svc *service) {
	for {
		it, ok := s.next(ctx, svc)
		if !ok {
			return
		}
		// A push in flight at stop still records its outcome, or the next
		// start sends it again; the client's timeout bounds the wait.
		if err := s.deliver(context.WithoutCancel(ctx), it); err != nil {
			s.cfg.Log.Warn("push outcome not stored", "delivery", it.row, "err", err)
		}
		s.mu.Lock()
		s.held--
		if it.row != 0 {
			delete(s.rows, it.row)
			s.settled[it.row] = true
		}
		s.mu.Unlock()
	}
}

// next takes svc's next push, waiting for one. It reports false once ctx
// ends, and leaves the rest of the queue to stop.
func (s *Sender) next(ctx context.Context, svc *service) (item, bool) {
	for ctx.Err() == nil {
		s.mu.Lock()
		if len(svc.queue) > 0 {
			it := svc.queue[0]
			svc.queue = svc.queue[1:]
			s.mu.Unlock()
			return it, true
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-svc.wake:
		}
	}
	return item{}, false
}

// check hands one page of the outbox's due pushes to their routines: at most
// checkPage, and no more than memory has room for. A page starts after the
// last row the previous one read, and the reads start over after a short
// page, so rows held by a stalled service cannot keep the others out of
// reach. A row already held stays with the routine holding it, and a row
// settled while Due ran is skipped. Only one check runs at a time.
func (s *Sender) check(ctx context.Context) {
	s.mu.Lock()
	clear(s.settled)
	limit := min(checkPage, memoryCap-s.held)
	after := s.after
	s.mu.Unlock()
	if limit <= 0 {
		return
	}
	page, err := s.cfg.Store.Due(ctx, s.cfg.Now(), after, limit)
	if err != nil {
		if ctx.Err() == nil {
			s.cfg.Log.Warn("outbox check failed", "err", err)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.after = store.DueKey{}
	if len(page) == limit {
		last := page[len(page)-1]
		s.after = store.DueKey{At: last.NextAttempt, ID: last.ID}
	}
	for _, d := range page {
		// Send may have filled the room while Due ran.
		if s.held >= memoryCap {
			return
		}
		if !s.rows[d.ID] && !s.settled[d.ID] {
			s.hold(fromRow(d))
		}
	}
}

// stop writes the pushes left in the queues to the outbox, once no routine
// runs, and sends every later push there.
func (s *Sender) stop(ctx context.Context) {
	s.mu.Lock()
	s.stopped = true
	var left []store.Pending
	for _, svc := range s.services {
		for _, it := range svc.queue {
			if it.row == 0 {
				left = append(left, store.Pending{Push: it.Push, NextAttempt: s.cfg.Now()})
			}
		}
		s.held -= len(svc.queue)
		svc.queue = nil
	}
	clear(s.rows)
	s.mu.Unlock()
	s.write(ctx, left)
}

// write queues pending in the outbox, and logs them as lost when it fails.
func (s *Sender) write(ctx context.Context, pending []store.Pending) {
	if len(pending) == 0 {
		return
	}
	if err := s.cfg.Store.Queue(ctx, pending); err != nil {
		s.cfg.Log.Error("drop pushes", "count", len(pending), "err", err)
	}
}

// deliver sends it and acts on the answer. It returns the error of the store
// call that records the outcome.
func (s *Sender) deliver(ctx context.Context, it item) error {
	if s.cfg.Now().Sub(it.CreatedAt) >= maxQueueAge {
		s.cfg.Log.Warn("push dropped after a day unsent", "delivery", it.row, "attempts", it.attempts)
		return s.forget(ctx, it)
	}
	payload, err := encodePayload(it.Notification)
	var resp *http.Response
	if err == nil {
		resp, err = webpush.SendNotificationWithContext(ctx, payload,
			&webpush.Subscription{
				Endpoint: it.Subscription.Endpoint,
				Keys:     webpush.Keys{P256dh: it.Subscription.P256dh, Auth: it.Subscription.Auth},
			},
			&webpush.Options{
				HTTPClient:      s.cfg.Client,
				Subscriber:      s.cfg.Subject,
				VAPIDPublicKey:  s.cfg.Keys.Public,
				VAPIDPrivateKey: s.cfg.Keys.Private,
				TTL:             int(s.cfg.TTL.Seconds()),
			})
	}
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		// The URL carries the endpoint, which is a credential; log the cause only.
		s.cfg.Log.Warn("push failed", "delivery", it.row, "subscription", it.SubscriptionID, "err", urlErr.Err)
		return s.retry(ctx, it, 0)
	}
	if err != nil {
		s.cfg.Log.Error("push refused before sending", "delivery", it.row, "subscription", it.SubscriptionID, "err", err)
		return s.forget(ctx, it)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch code := resp.StatusCode; {
	case code >= 200 && code < 300:
		return s.forget(ctx, it)
	case code == http.StatusNotFound || code == http.StatusGone:
		s.cfg.Log.Info("push subscription expired", "subscription", it.SubscriptionID, "status", code)
		return s.cfg.Store.RemoveSubscription(ctx, it.SubscriptionID)
	case code == http.StatusTooManyRequests || code >= 500:
		return s.retry(ctx, it, retryAfter(resp.Header.Get("Retry-After"), s.cfg.Now()))
	default:
		// Any other 4xx means gnotifd sent a request the push service
		// refuses; retrying it would fail the same way.
		s.cfg.Log.Error("push rejected", "delivery", it.row, "status", code)
		return s.forget(ctx, it)
	}
}

// forget deletes it's outbox row, if it has one.
func (s *Sender) forget(ctx context.Context, it item) error {
	if it.row == 0 {
		return nil
	}
	return s.cfg.Store.DeleteDelivery(ctx, it.row)
}

// maxPayload is the largest message webpush-go fits in one aes128gcm record:
// the record less the GCM tag, the salt, size and key header, and the
// padding delimiter.
const maxPayload = int(webpush.MaxRecordSize) - 16 - 86 - 1

// encodePayload returns n as the JSON the service worker reads, at most
// maxPayload bytes. A link that would overflow becomes "/": title and body
// fit even with every byte escaped.
func encodePayload(n trigger.Notification) ([]byte, error) {
	encode := func(link string) ([]byte, error) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		err := enc.Encode(struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			Link  string `json:"link"`
		}{n.Title, n.Body, link})
		return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), err
	}
	b, err := encode(n.Link)
	if err != nil || len(b) <= maxPayload {
		return b, err
	}
	return encode("/")
}

// retry puts it in the outbox for its next attempt, after its backoff or
// after the push service's Retry-After when that is later.
func (s *Sender) retry(ctx context.Context, it item, after time.Duration) error {
	at := s.cfg.Now().Add(max(backoff(it.attempts), after))
	if it.row != 0 {
		return s.cfg.Store.Reschedule(ctx, it.row, at)
	}
	s.write(ctx, []store.Pending{{Push: it.Push, Attempts: it.attempts + 1, NextAttempt: at}})
	return nil
}

func backoff(attempts int) time.Duration {
	wait := baseBackoff
	for range attempts {
		wait *= 2
		if wait >= maxBackoff {
			return maxBackoff
		}
	}
	return wait
}

// retryAfter reads a Retry-After header in seconds or as an HTTP date.
func retryAfter(header string, now time.Time) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		return time.Duration(max(secs, 0)) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		return max(t.Sub(now), 0)
	}
	return 0
}
