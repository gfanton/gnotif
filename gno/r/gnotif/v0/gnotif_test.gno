package gnotif

import (
	"strings"
	"testing"

	"gno.land/p/nt/testutils/v0"
	"gno.land/p/nt/uassert/v0"
)

const game = "gno.land/r/demo/game"

var (
	alice = testutils.TestAddress("alice")
	bob   = testutils.TestAddress("bob")
)

func stored(t *testing.T, id string) *Trigger {
	t.Helper()
	if !triggers.Has(id) {
		t.Fatalf("trigger %s not stored", id)
	}
	return triggers.Get(id).(*Trigger)
}

func TestDeclareRejects(cur realm, t *testing.T) {
	long := strings.Repeat("a", 300)
	valid := [7]string{game, "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}"}
	cases := []struct {
		name, msg string
		field     int // index into valid: target, event, filter, param, title, body, link
		value     string
	}{
		{"target outside r", "invalid target", 0, "gno.land/p/demo/x"},
		{"target too long", "invalid target", 0, "gno.land/r/" + long[:246]},
		{"empty event", "invalid event", 1, ""},
		{"event too long", "invalid event", 1, long[:65]},
		{"nine filter pairs", "invalid filter", 2, "a=1,b=2,c=3,d=4,e=5,f=6,g=7,h=8,i=9"},
		{"filter without value", "invalid filter", 2, "a"},
		{"filter value with equals", "invalid filter", 2, "a=b=c"},
		{"filter empty key", "invalid filter", 2, "=b"},
		{"filter key too long", "invalid filter", 2, long[:65] + "=b"},
		{"param too long", "invalid param", 3, long[:65]},
		{"empty title", "invalid title", 4, ""},
		{"title too long", "invalid title", 4, long[:65]},
		{"body too long", "invalid body", 5, long[:256]},
		{"empty link", "invalid link", 6, ""},
		{"relative link", "invalid link", 6, "game"},
		{"protocol-relative link", "invalid link", 6, "//evil.com"},
		{"backslash link", "invalid link", 6, "/\\evil.com"},
		{"absolute link", "invalid link", 6, "https://evil.com"},
		{"link too long", "invalid link", 6, "/" + long},
		{"link with tab", "invalid link", 6, "/\t/evil.example"},
		{"link with newline", "invalid link", 6, "/\n/evil.example"},
		{"link with carriage return", "invalid link", 6, "/\r/evil.example"},
		{"link with DEL", "invalid link", 6, "/x\x7f"},
		{"link with inner backslash", "invalid link", 6, "/a\\b"},
		{"link with trailing tab", "invalid link", 6, "/ok\t"},
		{"event with space", "invalid event", 1, "Turn Played"},
		{"event with dash", "invalid event", 1, "Turn-Played"},
		{"event imitating the verified mark", "invalid event", 1, "TurnPlayed · ✓ verified"},
		{"param with space", "invalid param", 3, "next player"},
		{"filter key with space", "invalid filter", 2, "mode x=ranked"},
		{"filter key with dash", "invalid filter", 2, "mo-de=ranked"},
	}
	for _, c := range cases {
		a := valid
		a[c.field] = c.value
		uassert.AbortsWithMessage(t, cur, c.msg, func() {
			Declare(cross(cur), a[0], a[1], a[2], a[3], a[4], a[5], a[6])
		}, c.name)
	}
}

func TestDeclareAccepts(cur realm, t *testing.T) {
	valid := [7]string{game, "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}"}
	cases := []struct {
		name  string
		field int // index into valid: target, event, filter, param, title, body, link
		value string
	}{
		{"link with placeholder path", 6, "/{game}"},
		{"link with query", 6, "/?game={game}&x=1"},
		{"event with underscore and digit", 1, "Turn_Played2"},
		{"empty param for a broadcast", 3, ""},
	}
	for _, c := range cases {
		a := valid
		a[c.field] = c.value
		id := Declare(cross(cur), a[0], a[1], a[2], a[3], a[4], a[5], a[6])
		uassert.True(t, triggers.Has(id), c.name)
	}
}

func TestDeclareByUser(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	id := Declare(cross(cur), game, "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}")
	tr := stored(t, id)
	uassert.Equal(t, alice, tr.Declarer)
	uassert.False(t, tr.Verified)
	uassert.Equal(t, "mode=ranked", tr.Filter)
	uassert.Equal(t, game, tr.Target)
}

func TestDeclareVerified(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewCodeRealm(game))
	own := Declare(cross(cur), game, "TurnPlayed", "", "next", "Your turn", "Game {game}", "/")
	other := Declare(cross(cur), "gno.land/r/demo/other", "TurnPlayed", "", "next", "Your turn", "Game {game}", "/")
	uassert.True(t, stored(t, own).Verified)
	uassert.False(t, stored(t, other).Verified)
}

func TestRemove(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	id := Declare(cross(cur), game, "TurnPlayed", "", "next", "Your turn", "", "/")
	testing.SetRealm(testing.NewUserRealm(bob))
	uassert.AbortsWithMessage(t, cur, "not the declarer", func() { Remove(cross(cur), id) })
	testing.SetRealm(testing.NewUserRealm(alice))
	Remove(cross(cur), id)
	uassert.False(t, triggers.Has(id))
	uassert.AbortsWithMessage(t, cur, "unknown trigger", func() { Remove(cross(cur), id) })
}

func TestRenderEscapes(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	id := Declare(cross(cur), "gno.land/r/[x](javascript:alert(1))", "TurnPlayed", "", "", "Hi", "", "/")
	out := Render("")
	uassert.True(t, strings.Contains(out, id))
	uassert.False(t, strings.Contains(out, "[x](javascript"))

	testing.SetRealm(testing.NewCodeRealm(game))
	verified := Declare(cross(cur), game, "TurnPlayed", "", "next", "Your turn", "", "/")
	for _, line := range strings.Split(Render(""), "\n") {
		if strings.Contains(line, verified) {
			uassert.True(t, strings.Contains(line, "✓ verified"), line)
			return
		}
	}
	t.Fatalf("no line for trigger %s", verified)
}
