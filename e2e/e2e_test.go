//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// recorder is a TLS push server and the pushes it received.
type recorder struct {
	server *httptest.Server
	mu     sync.Mutex
	pushes []pushed
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pushes)
}

func (r *recorder) paths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := make([]string, len(r.pushes))
	for i, p := range r.pushes {
		paths[i] = p.path
	}
	return paths
}

// startGnotifd runs gnotifd in-process against the stack, with its registry
// at registryPath and its pushes going to the returned recorder, until the
// test ends. base is its HTTP URL.
func startGnotifd(t *testing.T, s *stack) (base string, rec *recorder) {
	t.Helper()
	rec = &recorder{}
	rec.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.pushes = append(rec.pushes, pushed{path: r.URL.Path, header: r.Header.Clone()})
		rec.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(rec.server.Close)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	base = "http://" + ln.Addr().String()
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx, server.Config{
			Listener:     ln,
			Indexer:      s.indexer,
			Registry:     registryPath,
			StartHeight:  1,
			DB:           filepath.Join(t.TempDir(), "gnotif.db"),
			Poll:         500 * time.Millisecond,
			MaxAge:       10 * time.Minute,
			VAPID:        delivery.Keys{Public: pub, Private: priv},
			VAPIDSubject: "e2e@gnotif.example",
			PushHosts:    []string{rec.server.Listener.Addr().String()},
			PushClient:   rec.server.Client(),
			Log:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		})
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
	return base, rec
}

func TestTurnNotifiesOpponent(t *testing.T) {
	s := newStack(t)
	base, rec := startGnotifd(t, s)
	push := rec.server
	count := rec.count

	listed := func() ([]string, error) {
		ts, err := listTriggers(base, pingpongPath)
		ids := make([]string, len(ts))
		for i, tr := range ts {
			ids[i] = tr.ID
		}
		return ids, err
	}
	var triggerID string
	require.Eventually(t, func() bool {
		ids, err := listed()
		if err != nil || len(ids) != 1 {
			return false
		}
		triggerID = ids[0]
		return true
	}, 30*time.Second, 250*time.Millisecond, "gnotifd never listed the verified pingpong trigger")

	// The registry refuses a declaration from anyone but the target realm.
	out, err := s.tryCallPackage("devtest", registryPath, "Declare", pingpongPath, "TurnPlayed", "", "", "Spoof", "spoofed", "/")
	require.Error(t, err)
	assert.Contains(t, out, "caller is not the target")

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

	ids, err := listed()
	require.NoError(t, err)
	assert.Equal(t, []string{triggerID}, ids)

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

	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, p := range rec.pushes {
		assert.Equal(t, "/push/1", p.path)
		assert.Equal(t, "86400", p.header.Get("TTL"))
		assert.Equal(t, "aes128gcm", p.header.Get("Content-Encoding"))
		assert.True(t, strings.HasPrefix(p.header.Get("Authorization"), "vapid t="), p.header.Get("Authorization"))
	}
}

type listedTrigger struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// listTriggers returns the triggers gnotifd lists for target, and an error
// if any is unverified or aimed at another target.
func listTriggers(base, target string) ([]listedTrigger, error) {
	resp, err := http.Get(base + "/v1/triggers?target=" + url.QueryEscape(target))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var ts []struct {
		listedTrigger
		Target   string `json:"target"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		return nil, err
	}
	out := make([]listedTrigger, 0, len(ts))
	for _, tr := range ts {
		if tr.Target != target || !tr.Verified {
			return nil, fmt.Errorf("listing carries %+v", tr)
		}
		out = append(out, tr.listedTrigger)
	}
	return out, nil
}

func TestRegistryVersions(t *testing.T) {
	s := newStack(t)
	const (
		pingpongV1 = "gno.land/r/dev/pingpong/v1"
		registryV1 = "gno.land/r/dev/gnotif/v1"
		registryV2 = "gno.land/r/dev/gnotif/v2"
	)
	s.addpkg("devtest", filepath.Join(s.root, "gno", "r", "gnotif", "v0"), registryV1,
		"lastID.Next().String()", `"v1-" + lastID.Next().String()`,
		registryPath, registryV1)
	s.addpkg("devtest", filepath.Join(s.root, "demo", "gno.land", "r", "pingpong", "v0"), pingpongV1,
		registryPath, registryV1,
		pingpongPath, pingpongV1)
	s.addpkg("devtest", filepath.Join(s.root, "e2e", "testdata", "gnotif-v2"), registryV2)
	s.callPackage("devtest", registryV2, "Spoof", pingpongPath)

	base, rec := startGnotifd(t, s)

	require.Eventually(t, func() bool {
		v0, err0 := listTriggers(base, pingpongPath)
		v1, err1 := listTriggers(base, pingpongV1)
		return err0 == nil && err1 == nil &&
			slices.Equal(v0, []listedTrigger{{ID: "0000001", Title: "Your turn"}}) &&
			len(v1) == 1 && v1[0].ID == "v1-0000001"
	}, 30*time.Second, 250*time.Millisecond, "gnotifd never listed both pingpong triggers")

	player2 := s.addrs["player2"]
	for _, c := range []struct{ endpoint, trigger string }{
		{rec.server.URL + "/push/v0", "0000001"},
		{rec.server.URL + "/push/v1", "v1-0000001"},
	} {
		key, err := ecdh.P256().GenerateKey(rand.Reader)
		require.NoError(t, err)
		auth := make([]byte, 16)
		rand.Read(auth)
		put(t, base+"/v1/subscription", map[string]any{
			"endpoint": c.endpoint,
			"keys": map[string]string{
				"p256dh": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
				"auth":   base64.RawURLEncoding.EncodeToString(auth),
			},
		})
		put(t, base+"/v1/subscription/optins", map[string]any{
			"endpoint": c.endpoint,
			"optins":   []map[string]string{{"trigger": c.trigger, "value": player2}},
		})
	}

	for _, pkg := range []string{pingpongPath, pingpongV1} {
		s.callPackage("devtest", pkg, "NewGame", player2)
		s.callPackage("player2", pkg, "Accept", "0000001")
		s.callPackage("devtest", pkg, "Play", "0000001")
	}

	require.Eventually(t, func() bool {
		paths := rec.paths()
		slices.Sort(paths)
		return slices.Equal(paths, []string{"/push/v0", "/push/v1"})
	}, 30*time.Second, 100*time.Millisecond, "expected one push per registry, got %v", rec.paths())
	assert.Never(t, func() bool { return rec.count() > 2 }, 3*time.Second, 100*time.Millisecond)

	// The v2 realm's events carry v0's id, and gnotifd ignored them.
	v0, err := listTriggers(base, pingpongPath)
	require.NoError(t, err)
	assert.Equal(t, []listedTrigger{{ID: "0000001", Title: "Your turn"}}, v0)
}

func put(t *testing.T, url string, body any) {
	t.Helper()
	status, answer := putRaw(t, url, body)
	require.Equal(t, http.StatusNoContent, status, "PUT %s: %s", url, answer)
}

// putRaw sends a JSON PUT and returns the status and the answer's body.
func putRaw(t *testing.T, url string, body any) (int, string) {
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
	return resp.StatusCode, answer.String()
}
