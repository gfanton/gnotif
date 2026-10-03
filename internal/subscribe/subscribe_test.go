package subscribe

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

var (
	validP256dh = newP256dh()
	validAuth   = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
)

func newP256dh() string {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
}

const fcm = "https://fcm.googleapis.com/fcm/send/abc"

type harness struct {
	store   *store.Store
	handler http.Handler
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "gnotif.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	require.NoError(t, st.Update(ctx, func(tx *store.Tx) error {
		if err := tx.PutTrigger(trigger.Trigger{
			ID: "t1", Target: "gno.land/r/demo/game", Event: "TurnPlayed", Param: "next",
			Title: "Your turn", Body: "Game {game}", Link: "/?game={game}", Declarer: "g1game", Verified: true,
			Filter: []trigger.Pair{{Key: "mode", Value: "ranked"}},
		}); err != nil {
			return err
		}
		return tx.PutTrigger(trigger.Trigger{
			ID: "t2", Target: "gno.land/r/gov/dao", Event: "ProposalCreated",
			Title: "New proposal", Link: "/", Declarer: "g1someone",
		})
	}))
	h := NewHandler(Config{Store: st, PublicKey: "PK", PushHosts: DefaultPushHosts, Log: slog.New(slog.DiscardHandler)})
	return &harness{store: st, handler: h}
}

func (h *harness) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func (h *harness) subscribers(t *testing.T, triggerID, value string) []int64 {
	t.Helper()
	var ids []int64
	require.NoError(t, h.store.Update(context.Background(), func(tx *store.Tx) error {
		var err error
		ids, err = tx.Subscribers(triggerID, value)
		return err
	}))
	return ids
}

func subscription(endpoint, p256dh, auth string) string {
	b, _ := json.Marshal(map[string]any{"endpoint": endpoint, "keys": map[string]string{"p256dh": p256dh, "auth": auth}})
	return string(b)
}

func optins(endpoint string, pairs ...string) string {
	list := []map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		list = append(list, map[string]string{"trigger": pairs[i], "value": pairs[i+1]})
	}
	b, _ := json.Marshal(map[string]any{"endpoint": endpoint, "optins": list})
	return string(b)
}

func TestVapid(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/v1/vapid", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"publicKey":"PK"}`, rec.Body.String())
}

func TestTriggers(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/v1/triggers", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[
		{"id":"t1","target":"gno.land/r/demo/game","event":"TurnPlayed","filter":"mode=ranked","param":"next",
		 "title":"Your turn","body":"Game {game}","link":"/?game={game}","declarer":"g1game","verified":true},
		{"id":"t2","target":"gno.land/r/gov/dao","event":"ProposalCreated","filter":"","param":"",
		 "title":"New proposal","body":"","link":"/","declarer":"g1someone","verified":false}
	]`, rec.Body.String())
}

func TestPutSubscription(t *testing.T) {
	cases := map[string]struct {
		body string
		want int
	}{
		"FCM":                  {subscription(fcm, validP256dh, validAuth), http.StatusNoContent},
		"WNS wildcard":         {subscription("https://wns2-par02p.notify.windows.com/w/?token=x", validP256dh, validAuth), http.StatusNoContent},
		"browser JSON":         {fmt.Sprintf(`{"endpoint":%q,"expirationTime":null,"keys":{"p256dh":%q,"auth":%q}}`, fcm, validP256dh, validAuth), http.StatusNoContent},
		"http scheme":          {subscription("http://fcm.googleapis.com/x", validP256dh, validAuth), http.StatusBadRequest},
		"unlisted host":        {subscription("https://push.evil.example/x", validP256dh, validAuth), http.StatusBadRequest},
		"userinfo trick":       {subscription("https://fcm.googleapis.com@evil.example/x", validP256dh, validAuth), http.StatusBadRequest},
		"unlisted port":        {subscription("https://fcm.googleapis.com:8443/x", validP256dh, validAuth), http.StatusBadRequest},
		"bare wildcard suffix": {subscription("https://notify.windows.com/x", validP256dh, validAuth), http.StatusBadRequest},
		"endpoint too long":    {subscription(fcm+strings.Repeat("a", 1001-len(fcm)), validP256dh, validAuth), http.StatusBadRequest},
		"short p256dh":         {subscription(fcm, base64.RawURLEncoding.EncodeToString(make([]byte, 64)), validAuth), http.StatusBadRequest},
		"p256dh off the curve": {subscription(fcm, base64.RawURLEncoding.EncodeToString(append([]byte{0x04}, make([]byte, 64)...)), validAuth), http.StatusBadRequest},
		"short auth":           {subscription(fcm, validP256dh, base64.RawURLEncoding.EncodeToString(make([]byte, 15))), http.StatusBadRequest},
		"body too large":       {subscription(fcm+strings.Repeat("a", 9000), validP256dh, validAuth), http.StatusRequestEntityTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			rec := h.do(t, http.MethodPut, "/v1/subscription", tc.body)
			assert.Equal(t, tc.want, rec.Code, rec.Body.String())
			if tc.want != http.StatusNoContent {
				var body struct{ Error string }
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.NotEmpty(t, body.Error)
			}
		})
	}
}

// A browser that rotates its push subscription names the old endpoint, and
// the opt-ins move to the new one.
func TestPutSubscriptionMovesOldEndpoint(t *testing.T) {
	h := newHarness(t)
	old, renewed := fcm+"/old", fcm+"/new"
	require.Equal(t, http.StatusNoContent, h.do(t, http.MethodPut, "/v1/subscription", subscription(old, validP256dh, validAuth)).Code)
	require.Equal(t, http.StatusNoContent, h.do(t, http.MethodPut, "/v1/subscription/optins", optins(old, "t1", "g1bob")).Code)

	body := fmt.Sprintf(`{"endpoint":%q,"keys":{"p256dh":%q,"auth":%q},"oldEndpoint":%q}`, renewed, validP256dh, validAuth, old)
	rec := h.do(t, http.MethodPut, "/v1/subscription", body)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	assert.Equal(t, http.StatusNotFound, h.do(t, http.MethodDelete, "/v1/subscription", fmt.Sprintf(`{"endpoint":%q}`, old)).Code)
	assert.Len(t, h.subscribers(t, "t1", "g1bob"), 1)
	require.Equal(t, http.StatusNoContent, h.do(t, http.MethodDelete, "/v1/subscription", fmt.Sprintf(`{"endpoint":%q}`, renewed)).Code)
	assert.Empty(t, h.subscribers(t, "t1", "g1bob"), "the opt-in should belong to the new endpoint")
}

func TestPutOptins(t *testing.T) {
	many := make([]string, 0, 102)
	for i := range 51 {
		many = append(many, "t1", fmt.Sprintf("g1bob%d", i))
	}
	cases := map[string]struct {
		body string
		want int
	}{
		"unknown endpoint":   {optins("https://fcm.googleapis.com/other", "t1", "g1bob"), http.StatusNotFound},
		"unknown trigger":    {optins(fcm, "zz", "g1bob"), http.StatusBadRequest},
		"missing value":      {optins(fcm, "t1", ""), http.StatusBadRequest},
		"value on broadcast": {optins(fcm, "t2", "g1bob"), http.StatusBadRequest},
		"leading space":      {optins(fcm, "t1", " g1bob"), http.StatusBadRequest},
		"trailing newline":   {optins(fcm, "t1", "g1bob\n"), http.StatusBadRequest},
		"51 entries":         {optins(fcm, many...), http.StatusBadRequest},
		"valid":              {optins(fcm, "t1", "g1bob", "t2", ""), http.StatusNoContent},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			require.Equal(t, http.StatusNoContent, h.do(t, http.MethodPut, "/v1/subscription", subscription(fcm, validP256dh, validAuth)).Code)
			rec := h.do(t, http.MethodPut, "/v1/subscription/optins", tc.body)
			assert.Equal(t, tc.want, rec.Code, rec.Body.String())
		})
	}

	t.Run("replaces the set", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusNoContent, h.do(t, http.MethodPut, "/v1/subscription", subscription(fcm, validP256dh, validAuth)).Code)
		require.Equal(t, http.StatusNoContent, h.do(t, http.MethodPut, "/v1/subscription/optins", optins(fcm, "t1", "g1bob", "t2", "")).Code)
		require.Equal(t, http.StatusNoContent, h.do(t, http.MethodPut, "/v1/subscription/optins", optins(fcm, "t2", "")).Code)
		var t1, t2 []int64
		require.NoError(t, h.store.Update(context.Background(), func(tx *store.Tx) error {
			var err error
			if t1, err = tx.Subscribers("t1", "g1bob"); err != nil {
				return err
			}
			t2, err = tx.Subscribers("t2", "")
			return err
		}))
		assert.Empty(t, t1)
		assert.Len(t, t2, 1)
	})
}

func TestDeleteSubscription(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusNoContent, h.do(t, http.MethodPut, "/v1/subscription", subscription(fcm, validP256dh, validAuth)).Code)
	body := fmt.Sprintf(`{"endpoint":%q}`, fcm)
	assert.Equal(t, http.StatusNoContent, h.do(t, http.MethodDelete, "/v1/subscription", body).Code)
	assert.Equal(t, http.StatusNotFound, h.do(t, http.MethodDelete, "/v1/subscription", body).Code)
}

func TestCORS(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodOptions, "/v1/subscription", "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, PUT, DELETE, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", rec.Header().Get("Access-Control-Allow-Headers"))

	rec = h.do(t, http.MethodGet, "/v1/vapid", "")
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}
