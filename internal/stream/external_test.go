package stream

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func postMessage(t *testing.T, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	MessageHandler(testProtocol()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/message", strings.NewReader(body)))
	return rec.Code, rec.Body.String()
}

func TestMessageReplies(t *testing.T) {
	cases := map[string]struct{ frame, answer string }{
		"ping": {PingFrame, `{"reply":"{\"type\":\"pong\"}"}`},
		"accepted": {
			`{"type":"optins","optins":[{"trigger":"0000001","value":"g1bob"}]}`,
			fmt.Sprintf(`{"reply":"{\"type\":\"optins\"}","keys":[%q]}`, Key("0000001", "g1bob")),
		},
		"empty":           {`{"type":"optins","optins":[]}`, `{"reply":"{\"type\":\"optins\"}","keys":[]}`},
		"unknown trigger": {`{"type":"optins","optins":[{"trigger":"nope","value":""}]}`, `{"reply":"{\"type\":\"error\",\"error\":\"unknown trigger \\\"nope\\\"\"}"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, body := postMessage(t, tc.frame)
			assert.Equal(t, http.StatusOK, code)
			assert.JSONEq(t, tc.answer, body)
		})
	}
}

func TestMessageTooLarge(t *testing.T) {
	code, body := postMessage(t, strings.Repeat(" ", MaxFrame+1))
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
	assert.JSONEq(t, `{"error":"request body too large"}`, body)
}

func TestMessageMethods(t *testing.T) {
	rec := httptest.NewRecorder()
	MessageHandler(testProtocol()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/message", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
