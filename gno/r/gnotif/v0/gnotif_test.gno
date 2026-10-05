package gnotif

import (
	"strings"
	"testing"

	"gno.land/p/nt/markdown/sanitize/v0"
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
		{"target imitating the verified mark", "invalid target", 0, "gno.land/r/dev/pingpong/v0 · TurnPlayed · param next · ✓ verified · by g1tw6t…"},
		{"target with space", "invalid target", 0, "gno.land/r/dev/ping pong"},
		{"target with middle dot", "invalid target", 0, "gno.land/r/dev/ping·pong"},
		{"target with asterisk", "invalid target", 0, "gno.land/r/dev/**pingpong**"},
		{"target with bracket", "invalid target", 0, "gno.land/r/dev/[pingpong"},
		{"target with newline", "invalid target", 0, "gno.land/r/dev/pingpong\n- ✓ verified"},
		{"target with uppercase", "invalid target", 0, "gno.land/r/dev/PingPong"},
		{"target with dot", "invalid target", 0, "gno.land/r/dev/ping.pong"},
		{"target with no path", "invalid target", 0, "gno.land/r/"},
		{"target with empty segment", "invalid target", 0, "gno.land/r//x"},
		{"target with trailing slash", "invalid target", 0, "gno.land/r/x/"},
		{"target segment starting with digit", "invalid target", 0, "gno.land/r/9x"},
		{"target with doubled underscore", "invalid target", 0, "gno.land/r/a__b"},
		{"target segment ending with underscore", "invalid target", 0, "gno.land/r/a_"},
		{"target segment starting with dash", "invalid target", 0, "gno.land/r/-a"},
		{"target subsegment starting with dash", "invalid target", 0, "gno.land/r/a/-b"},
		{"target with mixed separators", "invalid target", 0, "gno.land/r/a-_b"},
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
		{"versioned target", 0, "gno.land/r/dev/pingpong/v0"},
		{"target in an address namespace", 0, "gno.land/r/g1tw6tyv67zjyf26nskj6g5z9fhtvv4996w50xwf/pingpong"},
		{"target with dash", 0, "gno.land/r/dev/ping-pong"},
		{"target with three segments", 0, "gno.land/r/dev/pingpong/v0"},
		{"target in a full address namespace", 0, "gno.land/r/g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5/gnotif/v0"},
		{"target with mixed separators between alphanumerics", 0, "gno.land/r/a-b_c/d"},
		{"target with a single segment", 0, "gno.land/r/user"},
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
	id := Declare(cross(cur), game, "TurnPlayed", "", "", "Hi", "[x](javascript:alert(1))", "/")
	out := Render("trigger/" + id)
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

func TestRenderPages(cur realm, t *testing.T) {
	var old []string
	triggers.Iterate("", "", func(id string, _ any) bool {
		old = append(old, id)
		return false
	})
	for _, id := range old {
		triggers.Remove(id)
	}

	testing.SetRealm(testing.NewCodeRealm(game))
	ids := make([]string, 51) // oldest first
	for i := range ids {
		ids[i] = Declare(cross(cur), game, "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}")
	}
	oldest, newest := ids[0], ids[50]

	page1 := Render("")
	uassert.Equal(t, page1, Render("page/1"))
	uassert.True(t, strings.Contains(page1, newest))
	uassert.True(t, strings.Contains(page1, ids[1]))
	uassert.False(t, strings.Contains(page1, oldest))
	uassert.True(t, strings.Index(page1, newest) < strings.Index(page1, ids[49]), "newest first")
	uassert.True(t, strings.Contains(page1, "(/r/dev/gnotif/v0:page/2)"))
	uassert.True(t, strings.Contains(page1, "(/r/dev/gnotif/v0:trigger/"+newest+")"))

	page2 := Render("page/2")
	uassert.True(t, strings.Contains(page2, oldest))
	uassert.False(t, strings.Contains(page2, ids[1]))
	uassert.False(t, strings.Contains(page2, ":page/3)"))

	uassert.True(t, strings.Contains(Render("page/3"), "No triggers on this page."))
	for _, p := range []string{"page/3", "page/9223372036854775807", "page/184467440737095518"} {
		out := Render(p)
		uassert.True(t, strings.Contains(out, "No triggers on this page."), p)
		uassert.False(t, strings.Contains(out, ":page/"), p)
	}
	for _, p := range []string{"page/0", "page/-1", "page/x", "nope", "trigger/zzz"} {
		uassert.Equal(t, "Not found.", Render(p), p)
	}

	triggers.Remove(newest)
	uassert.False(t, strings.Contains(Render(""), ":page/2)"))

	detail := Render("trigger/" + oldest)
	for _, want := range []string{oldest, game, "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}", "✓ verified"} {
		uassert.True(t, strings.Contains(detail, sanitize.InlineText(want)), want)
	}

	testing.SetRealm(testing.NewUserRealm(alice))
	unverified := Declare(cross(cur), game, "TurnPlayed", "", "", "Hi", "[x](javascript:alert(1))", "/")
	detail = Render("trigger/" + unverified)
	uassert.False(t, strings.Contains(detail, "[x](javascript"))
	uassert.False(t, strings.Contains(detail, "✓ verified"))
}
