// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	forgot "gno.land/r/alice/forgot/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

const ftarget = "gno.land/r/alice/forgot/v0"

var (
	alice = testutils.TestAddress("alice")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
	turn  = reg.Trigger{Event: "Turn", Title: "Your turn", Link: "/"}
)

// enableNames registers alice and bob, then turns namespace enforcement on.
func enableNames(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
}

// Revoke leaves the dapp without a handle until it reopens. The owner's next
// direct call into the dapp reaches its G1 bug: Y opens first, and the dapp's
// own reopen panics.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.Activate(cross(cur))
	forgot.Set(cross(cur), "turn", turn)
	reg.Revoke(cross(cur), ftarget)
	reg.Drop(cross(cur), ftarget, reg.Active(ftarget))
	forgot.Lend(cross(cur))
	forgot.Activate(cross(cur))
}

// Output:
// G1 set v1-0000004

// Error:
// gno.land/r/alice/forgot/v0 already holds a handle: its namespace owner can Revoke it
