package pingpong

import (
	"strings"
	"testing"

	"gno.land/p/nt/testutils/v0"
	"gno.land/p/nt/uassert/v0"
)

var (
	alice   = testutils.TestAddress("alice")
	bob     = testutils.TestAddress("bob")
	carol   = testutils.TestAddress("carol")
	noGames = "No games on this page."
)

func stored(t *testing.T, id string) *Game {
	t.Helper()
	if !games.Has(id) {
		t.Fatalf("game %s not stored", id)
	}
	return games.Get(id).(*Game)
}

func clearGames() {
	var ids []string
	games.Iterate("", "", func(id string, _ any) bool {
		ids = append(ids, id)
		return false
	})
	for _, id := range ids {
		games.Remove(id)
	}
}

// unchanged fails the test unless game id still holds exactly want.
func unchanged(t *testing.T, id string, want Game) {
	t.Helper()
	got := *stored(t, id)
	if got != want {
		t.Fatalf("game %s changed: got %+v, want %+v", id, got, want)
	}
}

func TestNewGame(cur realm, t *testing.T) {
	testing.SetRealm(testing.NewUserRealm(alice))
	uassert.AbortsWithMessage(t, cur, "invalid opponent", func() { NewGame(cross(cur), address("not-an-address")) })
	uassert.AbortsWithMessage(t, cur, "cannot play against yourself", func() { NewGame(cross(cur), alice) })
	uassert.AbortsWithMessage(t, cur, "invalid opponent", func() { NewGame(cross(cur), address(strings.ToUpper(alice.String()))) })
	uassert.AbortsWithMessage(t, cur, "invalid opponent", func() { NewGame(cross(cur), address(strings.ToUpper(bob.String()))) })
	uassert.Equal(t, 0, games.Size())

	id := NewGame(cross(cur), bob)
	uassert.Equal(t, "0000001", id)
	g := stored(t, id)
	uassert.Equal(t, alice, g.First)
	uassert.Equal(t, bob, g.Second)
	uassert.Equal(t, alice, g.Next)
	uassert.Equal(t, 0, g.Turn)
	uassert.False(t, g.Started)
}

func TestAccept(cur realm, t *testing.T) {
	clearGames()
	testing.SetRealm(testing.NewUserRealm(alice))
	id := NewGame(cross(cur), bob)
	pending := *stored(t, id)

	for _, who := range []address{carol, alice} {
		testing.SetRealm(testing.NewUserRealm(who))
		uassert.AbortsWithMessage(t, cur, "not invited", func() { Accept(cross(cur), id) })
		unchanged(t, id, pending)
	}

	testing.SetRealm(testing.NewUserRealm(bob))
	Accept(cross(cur), id)
	started := *stored(t, id)
	uassert.True(t, started.Started)
	uassert.Equal(t, alice, started.Next)
	uassert.Equal(t, 0, started.Turn)

	uassert.AbortsWithMessage(t, cur, "game already started", func() { Accept(cross(cur), id) })
	unchanged(t, id, started)

	uassert.AbortsWithMessage(t, cur, "unknown game", func() { Accept(cross(cur), "zzzzzzz") })
}

func TestPlayBeforeAccept(cur realm, t *testing.T) {
	clearGames()
	testing.SetRealm(testing.NewUserRealm(alice))
	id := NewGame(cross(cur), bob)
	pending := *stored(t, id)

	uassert.AbortsWithMessage(t, cur, "game not started", func() { Play(cross(cur), id) })
	testing.SetRealm(testing.NewUserRealm(bob))
	uassert.AbortsWithMessage(t, cur, "game not started", func() { Play(cross(cur), id) })
	unchanged(t, id, pending)
}

func TestPlay(cur realm, t *testing.T) {
	clearGames()
	testing.SetRealm(testing.NewUserRealm(alice))
	id := NewGame(cross(cur), bob)
	testing.SetRealm(testing.NewUserRealm(bob))
	Accept(cross(cur), id)

	uassert.AbortsWithMessage(t, cur, "not your turn", func() { Play(cross(cur), id) })
	testing.SetRealm(testing.NewUserRealm(carol))
	uassert.AbortsWithMessage(t, cur, "not your turn", func() { Play(cross(cur), id) })
	uassert.Equal(t, 0, stored(t, id).Turn)

	testing.SetRealm(testing.NewUserRealm(alice))
	Play(cross(cur), id)
	uassert.Equal(t, bob, stored(t, id).Next)
	uassert.Equal(t, 1, stored(t, id).Turn)

	testing.SetRealm(testing.NewUserRealm(bob))
	Play(cross(cur), id)
	uassert.Equal(t, alice, stored(t, id).Next)
	uassert.Equal(t, 2, stored(t, id).Turn)

	uassert.AbortsWithMessage(t, cur, "unknown game", func() { Play(cross(cur), "zzzzzzz") })
}

func TestLeave(cur realm, t *testing.T) {
	clearGames()
	testing.SetRealm(testing.NewUserRealm(alice))
	pendingID := NewGame(cross(cur), bob)
	startedID := NewGame(cross(cur), bob)
	testing.SetRealm(testing.NewUserRealm(bob))
	Accept(cross(cur), startedID)
	pending, started := *stored(t, pendingID), *stored(t, startedID)

	testing.SetRealm(testing.NewUserRealm(carol))
	uassert.AbortsWithMessage(t, cur, "not a player", func() { Leave(cross(cur), pendingID) })
	uassert.AbortsWithMessage(t, cur, "not a player", func() { Leave(cross(cur), startedID) })
	unchanged(t, pendingID, pending)
	unchanged(t, startedID, started)
	uassert.AbortsWithMessage(t, cur, "unknown game", func() { Leave(cross(cur), "zzzzzzz") })

	testing.SetRealm(testing.NewUserRealm(bob))
	Leave(cross(cur), pendingID)
	uassert.False(t, games.Has(pendingID))
	testing.SetRealm(testing.NewUserRealm(alice))
	Leave(cross(cur), startedID)
	uassert.False(t, games.Has(startedID))

	uassert.AbortsWithMessage(t, cur, "unknown game", func() { Play(cross(cur), startedID) })
	uassert.AbortsWithMessage(t, cur, "unknown game", func() { Accept(cross(cur), pendingID) })
	uassert.AbortsWithMessage(t, cur, "unknown game", func() { Leave(cross(cur), startedID) })
}

func TestDeclareTriggersOnce(cur realm, t *testing.T) {
	DeclareTriggers(cross(cur))
	uassert.AbortsWithMessage(t, cur, "triggers already declared", func() { DeclareTriggers(cross(cur)) })
}

func TestRenderGame(cur realm, t *testing.T) {
	clearGames()
	testing.SetRealm(testing.NewUserRealm(alice))
	id := NewGame(cross(cur), bob)

	out := Render("game/" + id)
	for _, want := range []string{id, alice.String(), bob.String(), "pending", "(/r/dev/pingpong/v0)"} {
		uassert.True(t, strings.Contains(out, want), want)
	}
	testing.SetRealm(testing.NewUserRealm(bob))
	Accept(cross(cur), id)
	out = Render("game/" + id)
	uassert.True(t, strings.Contains(out, "started"))
	uassert.False(t, strings.Contains(out, "pending"))

	for _, p := range []string{"game/zzz", "game/", "nope"} {
		uassert.Equal(t, "Not found.", Render(p), p)
	}
}

func TestRenderPages(cur realm, t *testing.T) {
	clearGames()
	uassert.True(t, strings.Contains(Render(""), noGames))

	testing.SetRealm(testing.NewUserRealm(alice))
	ids := make([]string, 51) // oldest first
	for i := range ids {
		ids[i] = NewGame(cross(cur), bob)
	}
	oldest, newest := ids[0], ids[50]

	testing.SetRealm(testing.NewCodeRealm("gno.land/r/demo/other"))
	page1 := Render("")
	uassert.Equal(t, page1, Render("page/1"))
	uassert.True(t, strings.Contains(page1, newest))
	uassert.True(t, strings.Contains(page1, ids[1]))
	uassert.False(t, strings.Contains(page1, oldest))
	uassert.True(t, strings.Index(page1, newest) < strings.Index(page1, ids[49]), "newest first")
	uassert.True(t, strings.Contains(page1, "(/r/dev/pingpong/v0:page/2)"))
	uassert.True(t, strings.Contains(page1, "(/r/dev/pingpong/v0:game/"+newest+")"))

	page2 := Render("page/2")
	uassert.True(t, strings.Contains(page2, oldest))
	uassert.False(t, strings.Contains(page2, ids[1]))
	uassert.False(t, strings.Contains(page2, ":page/3)"))

	for _, p := range []string{"page/3", "page/9223372036854775807", "page/184467440737095518"} {
		out := Render(p)
		uassert.True(t, strings.Contains(out, noGames), p)
		uassert.False(t, strings.Contains(out, ":page/"), p)
	}
	for _, p := range []string{"page/0", "page/-1", "page/x", "page/", "nope"} {
		uassert.Equal(t, "Not found.", Render(p), p)
	}

	// exactly 50 games fit on one page
	games.Remove(newest)
	exact := Render("")
	uassert.False(t, strings.Contains(exact, ":page/2)"))
	uassert.True(t, strings.Contains(exact, ids[49]))
	uassert.True(t, strings.Contains(exact, oldest))
	uassert.True(t, strings.Contains(Render("page/2"), noGames))
}
