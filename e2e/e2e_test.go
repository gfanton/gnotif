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
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/coder/websocket"
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

// recorder is a TLS push server and the pushes it received, beside an
// external hub's -publish end and the frames it was sent.
type recorder struct {
	server *httptest.Server
	mu     sync.Mutex
	pushes []pushed
	hooked []string
}

// hookedFor returns the frames the external hub was sent for value.
func (r *recorder) hookedFor(value string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, f := range r.hooked {
		var n struct{ Value string }
		if json.Unmarshal([]byte(f), &n) == nil && n.Value == value {
			out = append(out, f)
		}
	}
	return out
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

	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch []struct{ Key, Frame string }
		if json.NewDecoder(r.Body).Decode(&batch) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rec.mu.Lock()
		for _, item := range batch {
			rec.hooked = append(rec.hooked, item.Frame)
		}
		rec.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(hook.Close)

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
			StreamMax:    10,
			Publish:      hook.URL,
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

	// The registry opens a handle only for a realm's gnotif identity; a user's
	// direct call is not one.
	out, err := s.tryCallPackage("devtest", registryPath, "Open")
	require.Error(t, err)
	assert.Contains(t, out, "call with this realm's gnotif identity")

	endpoint := push.URL + "/push/1"
	subscribe(t, base, endpoint)
	put(t, base+"/v1/subscription/optins", map[string]any{
		"endpoint": endpoint,
		"optins":   []map[string]string{{"trigger": triggerID, "value": s.addrs["player2"]}},
	})

	liveCtx, liveCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(liveCancel)
	live, _, err := websocket.Dial(liveCtx, base+"/v1/stream", nil)
	require.NoError(t, err)
	t.Cleanup(func() { live.CloseNow() })
	set, err := json.Marshal(map[string]any{"type": "optins", "optins": []map[string]string{{"trigger": triggerID, "value": s.addrs["player2"]}}})
	require.NoError(t, err)
	require.NoError(t, live.Write(liveCtx, websocket.MessageText, set))
	_, reply, err := live.Read(liveCtx)
	require.NoError(t, err)
	require.Equal(t, `{"type":"optins"}`, string(reply))

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
	for _, p := range rec.pushes {
		assert.Equal(t, "/push/1", p.path)
		assert.Equal(t, "86400", p.header.Get("TTL"))
		assert.Equal(t, "aes128gcm", p.header.Get("Content-Encoding"))
		assert.True(t, strings.HasPrefix(p.header.Get("Authorization"), "vapid t="), p.header.Get("Authorization"))
	}
	rec.mu.Unlock()

	// The live connection hears the same two turns, frame for frame what the
	// external hub was sent for player2.
	var frames []string
	for range 2 {
		_, frame, err := live.Read(liveCtx)
		require.NoError(t, err)
		frames = append(frames, string(frame))
		var n struct {
			Type, ID, Trigger, Value string
			Attrs                    []struct{ Key, Value string }
		}
		require.NoError(t, json.Unmarshal(frame, &n))
		assert.Equal(t, "notification", n.Type)
		assert.Regexp(t, "^[0-9a-f]{32}$", n.ID)
		assert.Equal(t, triggerID, n.Trigger)
		assert.Equal(t, s.addrs["player2"], n.Value)
		assert.Contains(t, n.Attrs, struct{ Key, Value string }{"game", "0000001"})
	}
	player2 := func() []string { return rec.hookedFor(s.addrs["player2"]) }
	require.Eventually(t, func() bool { return len(player2()) == 2 }, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, frames, player2())

	conformance := exec.Command("node", "--test", "conformance/stream.test.ts")
	conformance.Env = append(os.Environ(), "GNOTIF_SERVER="+base, "GNOTIF_TARGET="+pingpongPath)
	report, err := conformance.CombinedOutput()
	require.NoError(t, err, "conformance suite:\n%s", report)

	// gnodev does not enforce namespaces, so the registry lets no key drop.
	out, err = s.tryCallPackage("devtest", registryPath, "Drop", pingpongPath, triggerID)
	require.Error(t, err)
	assert.Contains(t, out, "r/sys/names is not enabled, so no namespace has an owner")
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
		registryV2 = "gno.land/r/dev/gnotif/v2"
		registryV3 = "gno.land/r/dev/gnotif/v3"
	)
	s.addpkg("devtest", filepath.Join(s.root, "gno", "r", "gnotif"), registryV2,
		registryPath, registryV2)
	s.addpkg("devtest", filepath.Join(s.root, "demo", "gno.land", "r", "pingpong"), pingpongV1,
		registryPath, registryV2,
		pingpongPath, pingpongV1)
	s.addpkg("devtest", filepath.Join(s.root, "e2e", "testdata", "gnotif-v3"), registryV3)
	s.callPackage("devtest", registryV3, "Spoof", pingpongPath)

	base, rec := startGnotifd(t, s)

	require.Eventually(t, func() bool {
		v0, err0 := listTriggers(base, pingpongPath)
		v1, err1 := listTriggers(base, pingpongV1)
		return err0 == nil && err1 == nil &&
			slices.Equal(v0, []listedTrigger{{ID: "v1-0000001", Title: "Your turn"}}) &&
			len(v1) == 1 && v1[0].ID == "v2-0000001"
	}, 30*time.Second, 250*time.Millisecond, "gnotifd never listed both pingpong triggers")

	player2 := s.addrs["player2"]
	for _, c := range []struct{ endpoint, trigger string }{
		{rec.server.URL + "/push/v1", "v1-0000001"},
		{rec.server.URL + "/push/v2", "v2-0000001"},
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
		return slices.Equal(paths, []string{"/push/v1", "/push/v2"})
	}, 30*time.Second, 100*time.Millisecond, "expected one push per registry, got %v", rec.paths())
	assert.Never(t, func() bool { return rec.count() > 2 }, 3*time.Second, 100*time.Millisecond)

	// The v3 realm's events carry v1's id, and gnotifd ignored them.
	v0, err := listTriggers(base, pingpongPath)
	require.NoError(t, err)
	assert.Equal(t, []listedTrigger{{ID: "v1-0000001", Title: "Your turn"}}, v0)
}

// A session key that an admin approved for the realm cannot change the
// realm's triggers through the admins library; the admin's own key can.
func TestAdminsRefuseSessionKey(t *testing.T) {
	s := newStack(t)
	const managedPath = "gno.land/r/dev/managed/v0"
	s.addpkg("devtest", filepath.Join(s.root, "e2e", "testdata", "managed"), managedPath)
	s.addSession(managedPath)

	out, err := s.trySessionCall(managedPath, "SetTitle", "Phished")
	require.Error(t, err, "a session-signed SetTitle went through:\n%s", out)
	assert.Contains(t, out, "sign with your own key, not a session key")

	s.callPackage("devtest", managedPath, "SetTitle", "First")
}

// The registry refuses a session key where it needs the namespace owner's own:
// in the call that opens a realm's handle, and in Drop.
func TestRegistryRefusesSessionKey(t *testing.T) {
	s := newStack(t)
	const latePath = "gno.land/r/dev/late/v0"
	// managed, with its init renamed, opens its handle only when called.
	s.addpkg("devtest", filepath.Join(s.root, "e2e", "testdata", "managed"), latePath,
		"gno.land/r/dev/managed/v0", latePath,
		"package managed", "package late",
		"func init(cur realm)", "func Activate(cur realm)")
	s.addSession(latePath, registryPath)

	out, err := s.trySessionCall(latePath, "Activate")
	require.Error(t, err, "a session-signed Open went through:\n%s", out)
	assert.Contains(t, out, "signed with a session key: sign with the namespace owner's own key")

	out, err = s.trySessionCall(registryPath, "Drop", pingpongPath, "v1-0000001")
	require.Error(t, err, "a session-signed Drop went through:\n%s", out)
	assert.Contains(t, out, "sign with the namespace owner's own key, not a session key")

	s.callPackage("devtest", latePath, "Activate")
}

// A realm whose admins library holds its handle changes its trigger in
// transactions after its deploy, and the edit keeps the user's opt-in.
func TestManagedTriggerAcrossTransactions(t *testing.T) {
	s := newStack(t)
	const managedPath = "gno.land/r/dev/managed/v0"
	s.addpkg("devtest", filepath.Join(s.root, "e2e", "testdata", "managed"), managedPath)
	base, rec := startGnotifd(t, s)

	// listed returns the id of the realm's one trigger when its title is title.
	listed := func(title string) (string, bool) {
		ts, err := listTriggers(base, managedPath)
		if err != nil || len(ts) != 1 || ts[0].Title != title {
			return "", false
		}
		return ts[0].ID, true
	}

	s.callPackage("devtest", managedPath, "SetTitle", "First")
	var id string
	require.Eventually(t, func() bool {
		var ok bool
		id, ok = listed("First")
		return ok
	}, 30*time.Second, 250*time.Millisecond, "gnotifd never listed the managed trigger")

	endpoint := rec.server.URL + "/push/1"
	subscribe(t, base, endpoint)
	put(t, base+"/v1/subscription/optins", map[string]any{
		"endpoint": endpoint,
		"optins":   []map[string]string{{"trigger": id, "value": s.addrs["devtest"]}},
	})

	s.callPackage("devtest", managedPath, "SetTitle", "Second")
	require.Eventually(t, func() bool {
		edited, ok := listed("Second")
		return ok && edited == id
	}, 30*time.Second, 250*time.Millisecond, "gnotifd never listed the edited trigger under the same id")

	s.callPackage("devtest", managedPath, "Ping", "hi")
	require.Eventually(t, func() bool { return rec.count() == 1 }, 30*time.Second, 100*time.Millisecond, "expected 1 push, got %d", rec.count())
	assert.Never(t, func() bool { return rec.count() > 1 }, 3*time.Second, 100*time.Millisecond)

	s.callPackage("devtest", managedPath, "Remove")
	require.Eventually(t, func() bool {
		ts, err := listTriggers(base, managedPath)
		return err == nil && len(ts) == 0
	}, 30*time.Second, 250*time.Millisecond, "gnotifd still lists the removed managed trigger")
}

// subscribe registers a browser subscription at endpoint, with fresh keys.
func subscribe(t *testing.T, base, endpoint string) {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	auth := make([]byte, 16)
	rand.Read(auth)
	put(t, base+"/v1/subscription", map[string]any{
		"endpoint": endpoint,
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString(auth),
		},
	})
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
