package trigger

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const game = "gno.land/r/demo/game"

func pairs(kv ...string) []Pair {
	out := make([]Pair, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, Pair{Key: kv[i], Value: kv[i+1]})
	}
	return out
}

func TestParseFilter(t *testing.T) {
	cases := map[string]struct {
		in      string
		want    []Pair
		wantErr bool
	}{
		"empty":           {in: "", want: nil},
		"two pairs":       {in: "a=1,b=2", want: pairs("a", "1", "b", "2")},
		"no value":        {in: "a", wantErr: true},
		"empty key":       {in: "=b", wantErr: true},
		"empty value":     {in: "a=", wantErr: true},
		"equals in value": {in: "a=b=c", wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseFilter(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.in, FormatFilter(got))
		})
	}
}

func declared(overrides ...string) []Pair {
	base := map[string]string{
		"id": "0000001", "target": game, "event": "TurnPlayed", "filter": "mode=ranked",
		"param": "next", "title": "Your turn", "body": "Game {game}", "link": "/?game={game}",
		"declarer": "g1alice", "verified": "true",
	}
	for i := 0; i+1 < len(overrides); i += 2 {
		base[overrides[i]] = overrides[i+1]
	}
	var out []Pair
	for _, k := range []string{"id", "target", "event", "filter", "param", "title", "body", "link", "declarer", "verified"} {
		if v, ok := base[k]; ok {
			out = append(out, Pair{Key: k, Value: v})
		}
	}
	return out
}

func TestFromDeclared(t *testing.T) {
	got, err := FromDeclared(declared())
	require.NoError(t, err)
	assert.Equal(t, Trigger{
		ID: "0000001", Target: game, Event: "TurnPlayed", Param: "next",
		Title: "Your turn", Body: "Game {game}", Link: "/?game={game}",
		Filter: pairs("mode", "ranked"), Declarer: "g1alice", Verified: true,
	}, got)

	withoutID := declared()[1:]
	cases := map[string][]Pair{
		"missing id":       withoutID,
		"malformed filter": declared("filter", "a"),
		"unknown verified": declared("verified", "maybe"),
	}
	for name, attrs := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := FromDeclared(attrs)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrMalformed), err)
		})
	}
}

func TestMatches(t *testing.T) {
	tr := Trigger{Target: game, Event: "TurnPlayed", Filter: pairs("mode", "ranked")}
	cases := map[string]struct {
		event Event
		want  bool
	}{
		"all match":                    {Event{PkgPath: game, Type: "TurnPlayed", Attrs: pairs("mode", "ranked")}, true},
		"other realm":                  {Event{PkgPath: "gno.land/r/demo/other", Type: "TurnPlayed", Attrs: pairs("mode", "ranked")}, false},
		"other type":                   {Event{PkgPath: game, Type: "GameCreated", Attrs: pairs("mode", "ranked")}, false},
		"filter value differs":         {Event{PkgPath: game, Type: "TurnPlayed", Attrs: pairs("mode", "casual")}, false},
		"filter attribute missing":     {Event{PkgPath: game, Type: "TurnPlayed", Attrs: pairs("next", "g1bob")}, false},
		"duplicate key, first matches": {Event{PkgPath: game, Type: "TurnPlayed", Attrs: pairs("mode", "ranked", "mode", "casual")}, true},
		"duplicate key, first differs": {Event{PkgPath: game, Type: "TurnPlayed", Attrs: pairs("mode", "casual", "mode", "ranked")}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tr.Matches(tc.event))
		})
	}
}

func TestRender(t *testing.T) {
	base := Trigger{Title: "Your turn", Body: "Game {game}, turn {turn}", Link: "/?game={game}"}
	with := func(edit func(*Trigger)) Trigger {
		tr := base
		edit(&tr)
		return tr
	}
	cases := map[string]struct {
		trigger Trigger
		attrs   []Pair
		check   func(t *testing.T, n Notification)
	}{
		"plain": {base, pairs("game", "7", "turn", "2"), func(t *testing.T, n Notification) {
			assert.Equal(t, Notification{Title: "Your turn", Body: "Game 7, turn 2", Link: "/?game=7"}, n)
		}},
		"missing attribute": {base, pairs("turn", "2"), func(t *testing.T, n Notification) {
			assert.Equal(t, "Game , turn 2", n.Body)
		}},
		"unclosed brace": {with(func(tr *Trigger) { tr.Body = "Hi {game" }), pairs("game", "7"), func(t *testing.T, n Notification) {
			assert.Equal(t, "Hi {game", n.Body)
		}},
		"escaped link value": {base, pairs("game", "a b/c"), func(t *testing.T, n Notification) {
			assert.Equal(t, "/?game=a%20b%2Fc", n.Link)
		}},
		"duplicate key": {base, pairs("game", "7", "game", "8"), func(t *testing.T, n Notification) {
			assert.True(t, strings.HasPrefix(n.Body, "Game 7"), n.Body)
		}},
		"long title": {with(func(tr *Trigger) { tr.Title = "{x}" }), pairs("x", strings.Repeat("é", 100)), func(t *testing.T, n Notification) {
			assert.LessOrEqual(t, len(n.Title), MaxTitle)
			assert.True(t, utf8.ValidString(n.Title))
			assert.NotEmpty(t, n.Title)
		}},
		"long body": {with(func(tr *Trigger) { tr.Body = "{x}" }), pairs("x", strings.Repeat("a", 300)), func(t *testing.T, n Notification) {
			assert.Len(t, n.Body, MaxBody)
		}},
		"long link": {with(func(tr *Trigger) { tr.Link = "/{x}" }), pairs("x", strings.Repeat("a", 2000)), func(t *testing.T, n Notification) {
			assert.Equal(t, "/", n.Link)
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.check(t, tc.trigger.Render(Event{Attrs: tc.attrs}))
		})
	}
}
