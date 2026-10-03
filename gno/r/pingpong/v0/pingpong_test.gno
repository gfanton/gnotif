package pingpong

import (
	"strings"
	"testing"

	"gno.land/p/nt/testutils/v0"
	"gno.land/p/nt/uassert/v0"
)

var (
	alice = testutils.TestAddress("alice")
	bob   = testutils.TestAddress("bob")
)

func stored(t *testing.T, id string) *Game {
	t.Helper()
	if !games.Has(id) {
		t.Fatalf("game %s not stored", id)
	}
	return games.Get(id).(*Game)
}

func TestTurns(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	id := NewGame(cross(cur), bob)
	g := stored(t, id)
	uassert.Equal(t, alice, g.First)
	uassert.Equal(t, bob, g.Second)
	uassert.Equal(t, alice, g.Next)
	uassert.Equal(t, 0, g.Turn)

	testing.SetRealm(testing.NewUserRealm(bob))
	uassert.AbortsWithMessage(t, cur, "not your turn", func() { Play(cross(cur), id) })

	testing.SetRealm(testing.NewUserRealm(alice))
	Play(cross(cur), id)
	uassert.Equal(t, bob, stored(t, id).Next)
	uassert.Equal(t, 1, stored(t, id).Turn)

	testing.SetRealm(testing.NewUserRealm(bob))
	Play(cross(cur), id)
	uassert.Equal(t, alice, stored(t, id).Next)
	uassert.Equal(t, 2, stored(t, id).Turn)
}

func TestSelfGame(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	uassert.AbortsWithMessage(t, cur, "cannot play against yourself", func() { NewGame(cross(cur), alice) })
}

func TestUnknownGame(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	uassert.AbortsWithMessage(t, cur, "unknown game", func() { Play(cross(cur), "zzzzzzz") })
}

func TestDeclareTriggersOnce(cur realm, t *testing.T) {
	DeclareTriggers(cross(cur))
	uassert.AbortsWithMessage(t, cur, "triggers already declared", func() { DeclareTriggers(cross(cur)) })
}

func TestRender(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	id := NewGame(cross(cur), bob)
	out := Render("")
	uassert.True(t, strings.Contains(out, id))
	uassert.True(t, strings.Contains(out, alice.String()))
}
