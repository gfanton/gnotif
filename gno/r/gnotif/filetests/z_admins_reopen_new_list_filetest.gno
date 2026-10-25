// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	managed "gno.land/r/alice/managed/v0"
	evil "gno.land/r/bob/evil/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

var (
	alice = testutils.TestAddress("alice")
	bob   = testutils.TestAddress("bob")
	carol = testutils.TestAddress("carol")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
	turn  = reg.Trigger{Event: "Turn", Param: "player", Title: "Your turn", Body: "Game {game}", Link: "/"}
)

// enableNames registers alice and bob, then turns namespace enforcement on.
func enableNames(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
}

var (
	_       = managed.Activate
	_       = evil.UseLeakedAdmins
	_, _, _ = alice, bob, carol
	_       = turn
	_       = enableNames
)


// The owner revokes, then reopens with a new list, as the docs' Reopen does:
// bob, the new admin, sets a trigger, and carol, an old admin, is refused.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	managed.Activate(cross(cur))
	reg.Revoke(cross(cur), "gno.land/r/alice/managed/v0")
	managed.ActivateWith(cross(cur), bob)
	testing.SetRealm(testing.NewUserRealm(bob))
	println(managed.SetTrigger(cross(cur), "turn", turn))
	testing.SetRealm(testing.NewUserRealm(carol))
	managed.SetTrigger(cross(cur), "turn", turn)
}

// Output:
// v1-0000003

// Error:
// g1vdshymmvta047h6lta047h6lta047h6l2asz94 is not an admin
