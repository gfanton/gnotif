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

// The admins library refuses a remove from a caller that is not an admin.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	managed.Activate(cross(cur))
	testing.SetRealm(testing.NewUserRealm(carol))
	managed.SetTrigger(cross(cur), "turn", turn)
	testing.SetRealm(testing.NewUserRealm(bob))
	managed.RemoveTrigger(cross(cur), "turn")
}

// Error:
// g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu is not an admin
