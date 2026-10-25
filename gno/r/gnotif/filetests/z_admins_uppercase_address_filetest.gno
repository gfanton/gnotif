// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"strings"
	"testing"

	"gno.land/p/nt/testutils/v0"
	managed "gno.land/r/alice/managed/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

var (
	alice = testutils.TestAddress("alice")
	bob   = testutils.TestAddress("bob")
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

// An admin listed in upper case, as bech32 also allows, is the same key: bob
// sets a trigger.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	managed.ActivateWith(cross(cur), address(strings.ToUpper(string(bob))))
	testing.SetRealm(testing.NewUserRealm(bob))
	println(managed.SetTrigger(cross(cur), "turn", turn))
}

// Output:
// v1-0000003
