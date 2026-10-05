package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gfanton/gnotif/internal/trigger"
)

const publishQueue = 64

// PublisherConfig names the external hub that receives each window's matches.
type PublisherConfig struct {
	URL     string
	Token   string // sent as a bearer token when set
	Client  *http.Client
	Timeout time.Duration // per request
	Log     *slog.Logger
}

type publishedJSON struct {
	Key   string `json:"key"`
	Frame string `json:"frame"`
}

// Publisher sends each window's matches to an external hub, at most once.
type Publisher struct {
	cfg     PublisherConfig
	batches chan []publishedJSON
}

// NewPublisher returns a publisher; Run sends what Publish queues.
func NewPublisher(cfg PublisherConfig) *Publisher {
	return &Publisher{cfg: cfg, batches: make(chan []publishedJSON, publishQueue)}
}

// Publish queues one window's matches. It never blocks: with the queue full,
// the batch is dropped.
func (p *Publisher) Publish(matches []trigger.Match) {
	batch := make([]publishedJSON, len(matches))
	for i, m := range matches {
		batch[i] = publishedJSON{Key: Key(m.TriggerID, m.Value), Frame: string(NotificationFrame(m))}
	}
	select {
	case p.batches <- batch:
	default:
		p.cfg.Log.Warn("live publish queue full", "matches", len(matches))
	}
}

// Run sends the queued batches in order until ctx ends. A batch that fails is
// logged and never retried.
func (p *Publisher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case batch := <-p.batches:
			if err := p.send(ctx, batch); err != nil {
				p.cfg.Log.Warn("live publish failed", "matches", len(batch), "err", err)
			}
		}
	}
}

func (p *Publisher) send(ctx context.Context, batch []publishedJSON) error {
	body, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
	}
	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("hub answered %d", resp.StatusCode)
	}
	return nil
}
