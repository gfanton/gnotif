// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	chat "gno.land/r/alice/chat/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

const ctarget = "gno.land/r/alice/chat/v0"

var (
	alice = testutils.TestAddress("alice")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
)

// enableNames registers alice and bob, then turns namespace enforcement on.
func enableNames(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
}

// After the owner drops one of a dapp's triggers, running the dapp's
// declaration again counts the dropped one as new and panics.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	reg.Drop(cross(cur), ctarget, "v1-0000002")
	println(chat.Again(cross(cur)))
}

// Error:
// the triggers of gno.land/r/alice/chat/v0 are set: deploy a new version of the realm to change them
