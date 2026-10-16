// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	late "gno.land/r/alice/late/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

const target = "gno.land/r/alice/late/v0"

var (
	alice = testutils.TestAddress("alice")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
)

// The owner's direct call, names enforced, reaches G2 before the dapp's own
// Declare. G2 declares a fake, then the dapp's two triggers. The dapp's
// Declare leaves the fake out, so it panics and the call reverts.
func main(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))

	testing.SetRealm(testing.NewUserRealm(alice))
	ids := late.PluginThenActivate(cross(cur))
	println("own ids:", ids)
	println("active:", reg.Active(target))
}

// Output:
// G2 declared v1-0000003,v1-0000004,v1-0000005

// Error:
// the triggers of gno.land/r/alice/late/v0 are set: deploy a new version of the realm to change them
