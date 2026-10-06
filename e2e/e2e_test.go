//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/delivery"
	"github.com/gfanton/gnotif/internal/indexer"
	"github.com/gfanton/gnotif/internal/server"
	"github.com/gfanton/gnotif/internal/trigger"
)

type pushed struct {
	path   string
	header http.Header
}

func TestTurnNotifiesOpponent(t *testing.T) {
	s := newStack(t)

	var mu sync.Mutex
	var pushes []pushed
	push := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pushes = append(pushes, pushed{path: r.URL.Path, header: r.Header.Clone()})
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(push.Close)
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(pushes)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	base := "http://" + ln.Addr().String()
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx, server.Config{
			Listener:     ln,
			Indexer:      s.indexer,
			Registry:     "gno.land/r/dev/gnotif/v0",
			StartHeight:  1,
			DB:           filepath.Join(t.TempDir(), "gnotif.db"),
			Poll:         500 * time.Millisecond,
			MaxAge:       10 * time.Minute,
			VAPID:        delivery.Keys{Public: pub, Private: priv},
			VAPIDSubject: "e2e@gnotif.example",
			PushHosts:    []string{push.Listener.Addr().String()},
			PushClient:   push.Client(),
			Log:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		})
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})

	var triggerID string
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/v1/triggers")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var ts []struct {
			ID       string `json:"id"`
			Target   string `json:"target"`
			Verified bool   `json:"verified"`
		}
		if json.NewDecoder(resp.Body).Decode(&ts) != nil {
			return false
		}
		for _, tr := range ts {
			if tr.Target == pingpongPath && tr.Verified {
				triggerID = tr.ID
				return true
			}
		}
		return false
	}, 30*time.Second, 250*time.Millisecond, "gnotifd never listed the verified pingpong trigger")

	key, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	auth := make([]byte, 16)
	rand.Read(auth)
	endpoint := push.URL + "/push/1"
	put(t, base+"/v1/subscription", map[string]any{
		"endpoint": endpoint,
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString(auth),
		},
	})
	put(t, base+"/v1/subscription/optins", map[string]any{
		"endpoint": endpoint,
		"optins":   []map[string]string{{"trigger": triggerID, "value": s.addrs["player2"]}},
	})

	s.call("devtest", "NewGame", s.addrs["player2"])
	s.call("player2", "Accept", "0000001")
	require.Never(t, func() bool { return count() > 0 }, 3*time.Second, 100*time.Millisecond, "a push arrived before anyone played")

	playHeight, playHash := committed(t, s.call("devtest", "Play", "0000001")) // player2's turn: push 1
	s.call("player2", "Play", "0000001")                                       // devtest's turn: no push
	s.call("devtest", "Play", "0000001")                                       // player2's turn: push 2

	require.Eventually(t, func() bool { return count() == 2 }, 30*time.Second, 100*time.Millisecond, "expected 2 pushes, got %d", count())
	assert.Never(t, func() bool { return count() > 2 }, 3*time.Second, 100*time.Millisecond)

	// The pushes prove the indexer already holds the first Play's block.
	ix := indexer.New(s.indexer, http.DefaultClient)
	refs, _, err := ix.BlockTxs(context.Background(), playHeight)
	require.NoError(t, err)
	i := slices.IndexFunc(refs, func(r indexer.TxRef) bool { return r.Hash == playHash })
	require.NotEqual(t, -1, i, "BlockTxs(%d) = %v lacks %s", playHeight, refs, playHash)
	batch, err := ix.FetchTx(context.Background(), playHeight, refs[i].Index)
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(batch.Events, func(e trigger.Event) bool {
		return e.PkgPath == pingpongPath && e.Type == "TurnPlayed" &&
			e.TxHash == playHash && e.Height == playHeight && e.TxIndex == refs[i].Index
	}), "FetchTx(%d, %d) = %+v", playHeight, refs[i].Index, batch.Events)

	mu.Lock()
	defer mu.Unlock()
	for _, p := range pushes {
		assert.Equal(t, "/push/1", p.path)
		assert.Equal(t, "86400", p.header.Get("TTL"))
		assert.Equal(t, "aes128gcm", p.header.Get("Content-Encoding"))
		assert.True(t, strings.HasPrefix(p.header.Get("Authorization"), "vapid t="), p.header.Get("Authorization"))
	}
}

func put(t *testing.T, url string, body any) {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var answer bytes.Buffer
	answer.ReadFrom(resp.Body)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "PUT %s: %s", url, answer.String())
}
