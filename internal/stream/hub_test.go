package stream

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/trigger"
)

func newTestHub(t *testing.T, max int) (*Hub, string) {
	t.Helper()
	h := NewHub(HubConfig{Protocol: testProtocol(), Max: max, Log: slog.New(slog.DiscardHandler)})
	srv := httptest.NewServer(h)
	t.Cleanup(func() {
		h.Close()
		srv.Close()
	})
	return h, srv.URL
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, frame string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, c.Write(ctx, websocket.MessageText, []byte(frame)))
}

func recv(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	typ, b, err := c.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, typ)
	return string(b)
}

// closeStatus reads until the server closes c, and returns its code.
func closeStatus(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func optinsFrame(t *testing.T, pairs ...[2]string) string {
	t.Helper()
	type o struct {
		Trigger string `json:"trigger"`
		Value   string `json:"value"`
	}
	optins := []o{}
	for _, p := range pairs {
		optins = append(optins, o{p[0], p[1]})
	}
	b, err := json.Marshal(map[string]any{"type": "optins", "optins": optins})
	require.NoError(t, err)
	return string(b)
}

func testMatch(value string) trigger.Match {
	return trigger.Match{
		TriggerID:    "0000001",
		Value:        value,
		Notification: trigger.Notification{Title: "Your turn", Body: "Game 7, turn 1", Link: "/?game=7"},
		Event:        trigger.Event{TxHash: "tx-" + value, Height: 7},
	}
}

func subscribed(t *testing.T, url, value string) *websocket.Conn {
	t.Helper()
	c := dial(t, url)
	send(t, c, optinsFrame(t, [2]string{"0000001", value}))
	require.Equal(t, `{"type":"optins"}`, recv(t, c))
	return c
}

func TestHubFanOutByKey(t *testing.T) {
	h, url := newTestHub(t, 10)
	a := subscribed(t, url, "g1bob")
	b := subscribed(t, url, "g1alice")

	bob, alice := testMatch("g1bob"), testMatch("g1alice")
	h.Publish([]trigger.Match{bob, alice})
	assert.Equal(t, string(NotificationFrame(bob)), recv(t, a))
	assert.Equal(t, string(NotificationFrame(alice)), recv(t, b))

	// The next frame each one gets answers its ping: nothing else was queued.
	for _, c := range []*websocket.Conn{a, b} {
		send(t, c, PingFrame)
		assert.Equal(t, PongFrame, recv(t, c))
	}
}

func TestHubDuplicateOptinGivesOneFrame(t *testing.T) {
	h, url := newTestHub(t, 10)
	c := dial(t, url)
	send(t, c, optinsFrame(t, [2]string{"0000001", "g1bob"}, [2]string{"0000001", "g1bob"}))
	require.Equal(t, `{"type":"optins"}`, recv(t, c))

	bob := testMatch("g1bob")
	h.Publish([]trigger.Match{bob})
	assert.Equal(t, string(NotificationFrame(bob)), recv(t, c))
	send(t, c, PingFrame)
	assert.Equal(t, PongFrame, recv(t, c))
}

func TestHubNoUpgradeAnswers426(t *testing.T) {
	_, url := newTestHub(t, 10)
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUpgradeRequired, resp.StatusCode)
	assert.JSONEq(t, `{"error":"websocket upgrade required"}`, string(body))
}

func TestHubCapAnswers503(t *testing.T) {
	refused := func(t *testing.T, url string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, resp, err := websocket.Dial(ctx, url, nil)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.JSONEq(t, `{"error":"too many live connections"}`, string(body))
	}

	t.Run("max 1", func(t *testing.T) {
		_, url := newTestHub(t, 1)
		dial(t, url)
		refused(t, url)
	})
	t.Run("max 0", func(t *testing.T) {
		_, url := newTestHub(t, 0)
		refused(t, url)
	})
}

func TestHubFrameSizeBoundary(t *testing.T) {
	_, url := newTestHub(t, 10)
	frame := func(size int) string {
		head, tail := `{"type":"optins","optins":[]`, `}`
		return head + strings.Repeat(" ", size-len(head)-len(tail)) + tail
	}

	c := dial(t, url)
	send(t, c, frame(MaxFrame))
	assert.Equal(t, `{"type":"optins"}`, recv(t, c))

	c = dial(t, url)
	send(t, c, frame(MaxFrame+1))
	assert.Equal(t, websocket.StatusMessageTooBig, closeStatus(t, c))
}

func TestHubBinaryCloses1003(t *testing.T) {
	_, url := newTestHub(t, 10)
	c := dial(t, url)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, c.Write(ctx, websocket.MessageBinary, []byte{1}))
	assert.Equal(t, websocket.StatusUnsupportedData, closeStatus(t, c))
}

func TestHubSlowClientDropped(t *testing.T) {
	h, url := newTestHub(t, 1)
	subscribed(t, url, "g1bob") // and never reads again

	m := testMatch("g1bob")
	m.Notification.Body = strings.Repeat("b", trigger.MaxBody)
	matches := make([]trigger.Match, 10000)
	for i := range matches {
		matches[i] = m
	}
	h.Publish(matches)

	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			return false
		}
		c.CloseNow()
		return true
	}, 20*time.Second, 200*time.Millisecond, "the slow client still holds the only slot")
}

func TestHubCloseSends1001(t *testing.T) {
	h, url := newTestHub(t, 10)
	c := dial(t, url)

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		h.Close()
	}()
	assert.Equal(t, websocket.StatusGoingAway, closeStatus(t, c))
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, url, nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestHubOutlivesServerTimeouts(t *testing.T) {
	h := NewHub(HubConfig{Protocol: testProtocol(), Max: 10, Log: slog.New(slog.DiscardHandler)})
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ReadTimeout = 200 * time.Millisecond
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	t.Cleanup(func() {
		h.Close()
		srv.Close()
	})

	c := dial(t, srv.URL)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range 10 {
		<-ticker.C
		send(t, c, PingFrame)
		require.Equal(t, PongFrame, recv(t, c))
	}
}
