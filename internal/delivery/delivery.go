// Package delivery sends queued pushes through the browsers' push services
// and acts on their answers: delete, retry later, or drop a dead subscription.
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
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

const (
	batchSize   = 100
	baseBackoff = 30 * time.Second
	maxBackoff  = time.Hour
	maxQueueAge = 24 * time.Hour
	flushEvery  = time.Second
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
	Wake    <-chan struct{}
	Now     func() time.Time
	Log     *slog.Logger
}

// Sender drains the outbox. Flush and Run are not safe for concurrent use.
type Sender struct {
	cfg Config
}

// New returns a sender.
func New(cfg Config) *Sender {
	return &Sender{cfg: cfg}
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

// Run flushes the outbox when woken and every second, until ctx ends.
func (s *Sender) Run(ctx context.Context) error {
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()
	for {
		if err := s.Flush(ctx); err != nil && ctx.Err() == nil {
			s.cfg.Log.Warn("delivery flush failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.cfg.Wake:
		case <-ticker.C:
		}
	}
}

// Flush sends every due push once. It returns early on a store error, or
// when ctx ends, after recording the outcome of the push in flight.
func (s *Sender) Flush(ctx context.Context) error {
	for {
		due, err := s.cfg.Store.Due(ctx, s.cfg.Now(), batchSize)
		if err != nil {
			return err
		}
		for _, d := range due {
			if err := ctx.Err(); err != nil {
				return err
			}
			// A push the service accepted must be deleted, or the next start
			// sends it again; the client's timeout bounds the wait.
			if err := s.send(context.WithoutCancel(ctx), d); err != nil {
				return err
			}
		}
		if len(due) < batchSize {
			return nil
		}
	}
}

func (s *Sender) send(ctx context.Context, d store.Delivery) error {
	if s.cfg.Now().Sub(d.CreatedAt) >= maxQueueAge {
		s.cfg.Log.Warn("push dropped after a day unsent", "delivery", d.ID, "attempts", d.Attempts)
		return s.cfg.Store.DeleteDelivery(ctx, d.ID)
	}
	payload, err := encodePayload(d.Notification)
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload,
		&webpush.Subscription{
			Endpoint: d.Subscription.Endpoint,
			Keys:     webpush.Keys{P256dh: d.Subscription.P256dh, Auth: d.Subscription.Auth},
		},
		&webpush.Options{
			HTTPClient:      s.cfg.Client,
			Subscriber:      s.cfg.Subject,
			VAPIDPublicKey:  s.cfg.Keys.Public,
			VAPIDPrivateKey: s.cfg.Keys.Private,
			TTL:             int(s.cfg.TTL.Seconds()),
		})
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		// The URL carries the endpoint, which is a credential; log the cause only.
		s.cfg.Log.Warn("push failed", "delivery", d.ID, "subscription", d.SubscriptionID, "err", urlErr.Err)
		return s.retry(ctx, d, 0)
	}
	if err != nil {
		s.cfg.Log.Error("push refused before sending", "delivery", d.ID, "subscription", d.SubscriptionID, "err", err)
		return s.cfg.Store.DeleteDelivery(ctx, d.ID)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch code := resp.StatusCode; {
	case code >= 200 && code < 300:
		return s.cfg.Store.DeleteDelivery(ctx, d.ID)
	case code == http.StatusNotFound || code == http.StatusGone:
		s.cfg.Log.Info("push subscription expired", "subscription", d.SubscriptionID, "status", code)
		return s.cfg.Store.RemoveSubscription(ctx, d.SubscriptionID)
	case code == http.StatusTooManyRequests || code >= 500:
		return s.retry(ctx, d, retryAfter(resp.Header.Get("Retry-After"), s.cfg.Now()))
	default:
		// Any other 4xx means gnotifd sent a request the push service
		// refuses; retrying it would fail the same way.
		s.cfg.Log.Error("push rejected", "delivery", d.ID, "status", code)
		return s.cfg.Store.DeleteDelivery(ctx, d.ID)
	}
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

// retry reschedules d after its backoff, or after the push service's
// Retry-After when that is later.
func (s *Sender) retry(ctx context.Context, d store.Delivery, after time.Duration) error {
	return s.cfg.Store.Reschedule(ctx, d.ID, s.cfg.Now().Add(max(backoff(d.Attempts), after)))
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
