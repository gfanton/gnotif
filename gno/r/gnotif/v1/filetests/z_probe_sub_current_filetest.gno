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

// What /p/ code sees of a dapp's cur and of its gnotif identity: each is
// current, and its Previous is the dapp's caller.
func main(cur realm) {
	testing.SetRealm(testing.NewUserRealm(bob))
	println(managed.Probe(cross(cur)))
}

// Output:
// gno.land/r/alice/managed/v0 current=true previous=@g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu
// gno.land/r/alice/managed/v0#gnotif current=true previous=@g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu
// gno.land/r/alice/managed/v0#gnotif current=true previous=@g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu
