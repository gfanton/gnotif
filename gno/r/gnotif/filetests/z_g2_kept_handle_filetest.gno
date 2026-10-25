// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	forgot "gno.land/r/alice/forgot/v0"
	evil "gno.land/r/bob/evil/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

const ftarget = "gno.land/r/alice/forgot/v0"

var (
	alice = testutils.TestAddress("alice")
	bob   = testutils.TestAddress("bob")
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

// A dapp that opens late: G2 in the owner's direct call opens first, and Y
// keeps the handle. Later bob, a stranger, reaches the dapp's G1 bug, and Y
// sets a second fake through the kept handle with no owner signature, until
// the owner revokes.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(bob))
	evil.Keep(cross(cur))
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.RunPlugin(cross(cur))
	testing.SetRealm(testing.NewUserRealm(bob))
	forgot.Lend(cross(cur))
	println(reg.Active(ftarget))
}

// Output:
// G2 set v1-0000003
// kept set v1-0000004
// v1-0000003,v1-0000004
