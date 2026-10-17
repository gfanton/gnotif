package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/trigger"
)

var testTriggers = map[string]trigger.Trigger{
	"0000001": {ID: "0000001", Param: "next"},
	"0000002": {ID: "0000002"},
}

func testLookup(_ context.Context, ids []string) (map[string]trigger.Trigger, error) {
	out := map[string]trigger.Trigger{}
	for _, id := range ids {
		if t, ok := testTriggers[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

func testProtocol() Protocol {
	return Protocol{TriggersByID: testLookup, Log: slog.New(slog.DiscardHandler)}
}

func TestHandleFrames(t *testing.T) {
	data, err := os.ReadFile("testdata/frames.json")
	require.NoError(t, err)
	var cases []struct {
		Name  string
		Frame string
		Reply string
		Keys  *[][2]string // nil: the set is unchanged
	}
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases)

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			r := testProtocol().Handle(context.Background(), []byte(tc.Frame))
			assert.Equal(t, tc.Reply, string(r.Frame))
			if tc.Keys == nil {
				assert.False(t, r.Replaced)
				assert.Nil(t, r.Keys)
				return
			}
			want := []string{}
			for _, pair := range *tc.Keys {
				want = append(want, Key(pair[0], pair[1]))
			}
			assert.True(t, r.Replaced)
			assert.Equal(t, want, r.Keys)
		})
	}
}

func TestHandleInternalError(t *testing.T) {
	var log bytes.Buffer
	p := Protocol{
		TriggersByID: func(context.Context, []string) (map[string]trigger.Trigger, error) {
			return nil, errors.New("database is locked")
		},
		Log: slog.New(slog.NewTextHandler(&log, nil)),
	}
	r := p.Handle(context.Background(), []byte(`{"type":"optins","optins":[]}`))
	assert.Equal(t, `{"type":"error","error":"internal error"}`, string(r.Frame))
	assert.False(t, r.Replaced)
	assert.Contains(t, log.String(), "database is locked")

	r = p.Handle(context.Background(), []byte(PingFrame))
	assert.Equal(t, PongFrame, string(r.Frame))
}

func TestHandleLogsRefusedOptins(t *testing.T) {
	var log bytes.Buffer
	p := Protocol{TriggersByID: testLookup, Log: slog.New(slog.NewTextHandler(&log, nil))}
	r := p.Handle(context.Background(), []byte(`{"type":"optins","optins":[{"trigger":"9999999","value":""}]}`))
	assert.Equal(t, `{"type":"error","error":"unknown trigger \"9999999\""}`, string(r.Frame))
	assert.Contains(t, log.String(), `level=INFO msg="live opt-ins refused" reason="unknown trigger \"9999999\"" optins=1`)
}

func TestKey(t *testing.T) {
	k := Key("0000001", "g1bob")
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), k)
	assert.Equal(t, k, Key("0000001", "g1bob"))
	assert.NotEqual(t, Key("a", "bc"), Key("ab", "c"))
}

func TestNotificationFrame(t *testing.T) {
	m := trigger.Match{
		TriggerID:    "0000001",
		Value:        "g1bob",
		Notification: trigger.Notification{Title: "Your turn", Body: "Game 7, turn 1", Link: "/?game=7"},
		Event: trigger.Event{TxHash: "tx1", Height: 7, Attrs: []trigger.Pair{
			{Key: "game", Value: "7"}, {Key: "next", Value: "g1bob"}, {Key: "turn", Value: "1"},
		}},
	}
	// A zone other than UTC: the frame still gives the time in UTC.
	m.Event.Time = time.Date(2026, 10, 16, 23, 46, 8, 500_000_000, time.FixedZone("CEST", 2*60*60))
	assert.Equal(t,
		`{"type":"notification","id":"b8f1e52e04913c7b1e77d46c3a9bfa98","trigger":"0000001","value":"g1bob","title":"Your turn","body":"Game 7, turn 1","link":"/?game=7","height":7,"tx":"tx1","time":"2026-10-16T21:46:08.5Z","attrs":[{"key":"game","value":"7"},{"key":"next","value":"g1bob"},{"key":"turn","value":"1"}]}`,
		string(NotificationFrame(m)))
}
