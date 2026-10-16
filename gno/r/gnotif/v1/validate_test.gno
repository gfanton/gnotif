package gnotif

import (
	"strings"
	"testing"
)

const chat = "gno.land/r/alice/chat/v0"

func TestValidateRejects(t *testing.T) {
	long := strings.Repeat("a", 300)
	valid := [6]string{"TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}"}
	cases := []struct {
		name, msg string
		field     int // index into valid: event, filter, param, title, body, link
		value     string
	}{
		{"empty event", "invalid event", 0, ""},
		{"event too long", "invalid event", 0, long[:65]},
		{"nine filter pairs", "invalid filter", 1, "a=1,b=2,c=3,d=4,e=5,f=6,g=7,h=8,i=9"},
		{"filter without value", "invalid filter", 1, "a"},
		{"filter value with equals", "invalid filter", 1, "a=b=c"},
		{"filter empty key", "invalid filter", 1, "=b"},
		{"filter key too long", "invalid filter", 1, long[:65] + "=b"},
		{"param too long", "invalid param", 2, long[:65]},
		{"empty title", "invalid title", 3, ""},
		{"title too long", "invalid title", 3, long[:65]},
		{"body too long", "invalid body", 4, long[:256]},
		{"empty link", "invalid link", 5, ""},
		{"relative link", "invalid link", 5, "game"},
		{"protocol-relative link", "invalid link", 5, "//evil.com"},
		{"backslash link", "invalid link", 5, "/\\evil.com"},
		{"absolute link", "invalid link", 5, "https://evil.com"},
		{"link too long", "invalid link", 5, "/" + long},
		{"link with tab", "invalid link", 5, "/\t/evil.example"},
		{"link with newline", "invalid link", 5, "/\n/evil.example"},
		{"link with carriage return", "invalid link", 5, "/\r/evil.example"},
		{"link with DEL", "invalid link", 5, "/x\x7f"},
		{"link with inner backslash", "invalid link", 5, "/a\\b"},
		{"link with trailing tab", "invalid link", 5, "/ok\t"},
		{"event with space", "invalid event", 0, "Turn Played"},
		{"event with dash", "invalid event", 0, "Turn-Played"},
		{"event imitating the verified mark", "invalid event", 0, "TurnPlayed · ✓ verified"},
		{"param with space", "invalid param", 2, "next player"},
		{"filter key with space", "invalid filter", 1, "mode x=ranked"},
		{"filter key with dash", "invalid filter", 1, "mo-de=ranked"},
	}
	for _, c := range cases {
		a := valid
		a[c.field] = c.value
		mustPanic(t, c.msg, func() {
			validate(Trigger{Event: a[0], Filter: a[1], Param: a[2], Title: a[3], Body: a[4], Link: a[5]})
		})
	}
}

func TestValidateReportsFirstBadField(t *testing.T) {
	tr := Trigger{Event: "", Filter: "a", Param: "x y", Title: "", Body: strings.Repeat("a", 256), Link: ""}
	mustPanic(t, "invalid event", func() { validate(tr) })
	tr.Event = "TurnPlayed"
	mustPanic(t, "invalid filter", func() { validate(tr) })
	tr.Filter = ""
	mustPanic(t, "invalid param", func() { validate(tr) })
	tr.Param = ""
	mustPanic(t, "invalid title", func() { validate(tr) })
	tr.Title = "Your turn"
	mustPanic(t, "invalid body", func() { validate(tr) })
	tr.Body = ""
	mustPanic(t, "invalid link", func() { validate(tr) })
	tr.Link = "/"
	validate(tr)
}

func TestValidateAccepts(t *testing.T) {
	name := strings.Repeat("a", 64)
	cases := []struct {
		name string
		t    Trigger
	}{
		{"empty filter, param and body", Trigger{Event: "TurnPlayed", Title: "Your turn", Link: "/"}},
		{"link with placeholder path", Trigger{Event: "TurnPlayed", Title: "Your turn", Link: "/{game}"}},
		{"link with query", Trigger{Event: "TurnPlayed", Title: "Your turn", Link: "/?game={game}&x=1"}},
		{"event with underscore and digit", Trigger{Event: "Turn_Played2", Title: "Your turn", Link: "/"}},
		{"eight filter pairs", Trigger{Event: "TurnPlayed", Filter: "a=1,b=2,c=3,d=4,e=5,f=6,g=7,h=8", Title: "Your turn", Link: "/"}},
		{"64-byte names", Trigger{Event: name, Filter: name + "=" + name, Param: name, Title: name, Link: "/"}},
		{"255-byte body", Trigger{Event: "TurnPlayed", Title: "Your turn", Body: strings.Repeat("a", 255), Link: "/"}},
		{"256-byte link", Trigger{Event: "TurnPlayed", Title: "Your turn", Link: "/" + strings.Repeat("a", 255)}},
	}
	for _, c := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: validate panicked: %v", c.name, r)
				}
			}()
			validate(c.t)
		}()
	}
}

func TestValidTarget(t *testing.T) {
	long := strings.Repeat("a", 300)
	rejected := []struct{ name, target string }{
		{"target outside r", "gno.land/p/demo/x"},
		{"target too long", "gno.land/r/" + long[:246]},
		{"target imitating the verified mark", "gno.land/r/dev/pingpong/v0 · TurnPlayed · param next · ✓ verified · by g1tw6t…"},
		{"target with space", "gno.land/r/dev/ping pong"},
		{"target with middle dot", "gno.land/r/dev/ping·pong"},
		{"target with asterisk", "gno.land/r/dev/**pingpong**"},
		{"target with bracket", "gno.land/r/dev/[pingpong"},
		{"target with newline", "gno.land/r/dev/pingpong\n- ✓ verified"},
		{"target with sub-realm mark", "gno.land/r/demo/game#x"},
		{"target with uppercase", "gno.land/r/dev/PingPong"},
		{"target with dot", "gno.land/r/dev/ping.pong"},
		{"target with no path", "gno.land/r/"},
		{"target with empty segment", "gno.land/r//x"},
		{"target with trailing slash", "gno.land/r/x/"},
		{"target segment starting with digit", "gno.land/r/9x"},
		{"target with doubled underscore", "gno.land/r/a__b"},
		{"target segment ending with underscore", "gno.land/r/a_"},
		{"target segment starting with dash", "gno.land/r/-a"},
		{"target subsegment starting with dash", "gno.land/r/a/-b"},
		{"target with mixed separators", "gno.land/r/a-_b"},
	}
	for _, c := range rejected {
		if validTarget(c.target) {
			t.Errorf("%s: validTarget(%q) = true, want false", c.name, c.target)
		}
	}
	accepted := []struct{ name, target string }{
		{"versioned target", chat},
		{"target in an address namespace", "gno.land/r/g1tw6tyv67zjyf26nskj6g5z9fhtvv4996w50xwf/pingpong"},
		{"target with dash", "gno.land/r/dev/ping-pong"},
		{"target of 256 bytes", "gno.land/r/" + strings.Repeat("a", 245)},
		{"target in a full address namespace", "gno.land/r/g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5/gnotif/v0"},
		{"target with mixed separators between alphanumerics", "gno.land/r/a-b_c/d"},
		{"target with a single segment", "gno.land/r/user"},
	}
	for _, c := range accepted {
		if !validTarget(c.target) {
			t.Errorf("%s: validTarget(%q) = false, want true", c.name, c.target)
		}
	}
}

func TestNamespaceOf(t *testing.T) {
	cases := []struct{ target, want string }{
		{chat, "alice"},
		{"gno.land/r/g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh/pa/v0", "g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh"},
		{"gno.land/r/user", "user"},
	}
	for _, c := range cases {
		if got := namespaceOf(c.target); got != c.want {
			t.Errorf("namespaceOf(%q) = %q, want %q", c.target, got, c.want)
		}
	}
	mustPanic(t, "invalid target gno.land/p/x/y", func() { namespaceOf("gno.land/p/x/y") })
}
