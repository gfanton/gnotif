// Package stream serves gnotif's live notifications: the frames of the
// /v1/stream protocol, the hub that holds live connections, and the
// interface an external hub uses to hold them instead.
package stream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"

	"github.com/gfanton/gnotif/internal/optin"
	"github.com/gfanton/gnotif/internal/trigger"
)

// MaxFrame is the size of the largest client frame, in bytes.
const MaxFrame = 8192

// The heartbeat frames. A ping is recognised by its exact text, so a hub can
// answer it without parsing.
const (
	PingFrame = `{"type":"ping"}`
	PongFrame = `{"type":"pong"}`
)

const acceptedFrame = `{"type":"optins"}`

// Key identifies an opt-in among a connection's set. It never reaches a
// client.
func Key(triggerID, value string) string {
	sum := sha256.Sum256([]byte(triggerID + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

// NotificationFrame is the frame a connection receives for m.
func NotificationFrame(m trigger.Match) []byte {
	// Strings and integers only: Marshal cannot fail.
	b, _ := json.Marshal(struct {
		Type string `json:"type"`
		trigger.BrowserNotification
	}{"notification", m.ForBrowser()})
	return b
}

// Protocol answers client frames.
type Protocol struct {
	TriggersByID optin.Lookup
	Log          *slog.Logger
}

// Reply is the answer to one client frame.
type Reply struct {
	Frame []byte
	// Keys is the connection's new set, without duplicates, when Replaced.
	Keys     []string
	Replaced bool
}

// Handle answers one client text frame.
func (p Protocol) Handle(ctx context.Context, frame []byte) Reply {
	if string(frame) == PingFrame {
		return Reply{Frame: []byte(PongFrame)}
	}
	var msg struct {
		Type   string        `json:"type"`
		Optins []optin.Optin `json:"optins"`
	}
	if err := json.Unmarshal(frame, &msg); err != nil {
		return errorReply("invalid JSON message")
	}
	if msg.Type != "optins" {
		return errorReply("unknown message type")
	}
	err := optin.Check(ctx, msg.Optins, p.TriggersByID)
	if rule, ok := errors.AsType[optin.Error](err); ok {
		p.Log.Info("live opt-ins refused", "reason", rule.Error(), "optins", len(msg.Optins))
		return errorReply(rule.Error())
	}
	if err != nil {
		p.Log.Error("live opt-ins", "err", err)
		return errorReply("internal error")
	}
	keys := make([]string, 0, len(msg.Optins))
	for _, o := range msg.Optins {
		if k := Key(o.Trigger, o.Value); !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return Reply{Frame: []byte(acceptedFrame), Keys: keys, Replaced: true}
}

func errorReply(msg string) Reply {
	// Strings only: Marshal cannot fail.
	b, _ := json.Marshal(struct {
		Type  string `json:"type"`
		Error string `json:"error"`
	}{"error", msg})
	return Reply{Frame: b}
}
