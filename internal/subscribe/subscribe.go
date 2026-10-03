// Package subscribe serves the HTTP API browsers use to register push
// subscriptions and opt in to triggers.
package subscribe

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

// DefaultPushHosts are the push services browsers use, as ntfy allows them.
var DefaultPushHosts = []string{
	"fcm.googleapis.com",
	"updates.push.services.mozilla.com",
	"web.push.apple.com",
	"*.notify.windows.com",
}

const maxBody = 8192

// Config holds the handler's dependencies.
type Config struct {
	Store     *store.Store
	PublicKey string // VAPID public key browsers subscribe with
	PushHosts []string
	Log       *slog.Logger
}

type triggerJSON struct {
	ID       string `json:"id"`
	Target   string `json:"target"`
	Event    string `json:"event"`
	Filter   string `json:"filter"`
	Param    string `json:"param"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Link     string `json:"link"`
	Declarer string `json:"declarer"`
	Verified bool   `json:"verified"`
}

type subscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	// OldEndpoint names the subscription this one replaces after the
	// browser rotated it; knowing an endpoint already authorizes deleting it.
	OldEndpoint string `json:"oldEndpoint"`
}

type optinJSON struct {
	Trigger string `json:"trigger"`
	Value   string `json:"value"`
}

type optinsJSON struct {
	Endpoint string      `json:"endpoint"`
	Optins   []optinJSON `json:"optins"`
}

type handler struct {
	cfg Config
}

// NewHandler returns the /v1/ API. Every answer allows any origin.
func NewHandler(cfg Config) http.Handler {
	h := &handler{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/vapid", h.vapid)
	mux.HandleFunc("GET /v1/triggers", h.triggers)
	mux.HandleFunc("PUT /v1/subscription", h.putSubscription)
	mux.HandleFunc("PUT /v1/subscription/optins", h.putOptins)
	mux.HandleFunc("DELETE /v1/subscription", h.deleteSubscription)
	mux.HandleFunc("OPTIONS /v1/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		mux.ServeHTTP(w, r)
	})
}

func (h *handler) vapid(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"publicKey": h.cfg.PublicKey})
}

func (h *handler) triggers(w http.ResponseWriter, r *http.Request) {
	ts, err := h.cfg.Store.Triggers(r.Context())
	if err != nil {
		h.internal(w, err)
		return
	}
	out := make([]triggerJSON, len(ts))
	for i, t := range ts {
		out[i] = triggerJSON{
			ID: t.ID, Target: t.Target, Event: t.Event, Filter: trigger.FormatFilter(t.Filter),
			Param: t.Param, Title: t.Title, Body: t.Body, Link: t.Link,
			Declarer: t.Declarer, Verified: t.Verified,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) putSubscription(w http.ResponseWriter, r *http.Request) {
	var sub subscriptionJSON
	if !decode(w, r, &sub) {
		return
	}
	for _, err := range []error{
		checkEndpoint(sub.Endpoint, h.cfg.PushHosts),
		checkKey("p256dh", sub.Keys.P256dh, p256dhSize),
		checkKey("auth", sub.Keys.Auth, authSize),
	} {
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	stored := store.Subscription{Endpoint: sub.Endpoint, P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth}
	var err error
	if sub.OldEndpoint != "" {
		err = h.cfg.Store.MoveSubscription(r.Context(), sub.OldEndpoint, stored)
	} else {
		err = h.cfg.Store.PutSubscription(r.Context(), stored)
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) putOptins(w http.ResponseWriter, r *http.Request) {
	var req optinsJSON
	if !decode(w, r, &req) {
		return
	}
	ts, err := h.cfg.Store.Triggers(r.Context())
	if err != nil {
		h.internal(w, err)
		return
	}
	byID := make(map[string]trigger.Trigger, len(ts))
	for _, t := range ts {
		byID[t.ID] = t
	}
	if err := checkOptins(req.Optins, byID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	optins := make([]store.Optin, len(req.Optins))
	for i, o := range req.Optins {
		optins[i] = store.Optin{TriggerID: o.Trigger, Value: o.Value}
	}
	err = h.cfg.Store.ReplaceOptins(r.Context(), req.Endpoint, optins)
	if errors.Is(err, store.ErrUnknownSubscription) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !decode(w, r, &req) {
		return
	}
	err := h.cfg.Store.DeleteSubscription(r.Context(), req.Endpoint)
	if errors.Is(err, store.ErrUnknownSubscription) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) internal(w http.ResponseWriter, err error) {
	h.cfg.Log.Error("subscription API", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// decode reads a JSON body of at most maxBody bytes, answering the error
// itself when it fails.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v)
	if err == nil {
		return true
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}
	writeError(w, http.StatusBadRequest, "invalid JSON body")
	return false
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
