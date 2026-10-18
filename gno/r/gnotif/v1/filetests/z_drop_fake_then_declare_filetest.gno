// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"strings"
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

// G2 in an earlier owner call declares a fake, then the dapp's two triggers.
// Once the owner drops the fake, the dapp's own Declare names every active
// trigger and returns their ids.
func main(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))

	testing.SetRealm(testing.NewUserRealm(alice))
	late.RunPlugin(cross(cur))
	fake, _, _ := strings.Cut(reg.Active(target), ",")
	reg.Drop(cross(cur), target, fake)
	println("active:", reg.Active(target))
	println("own ids:", late.Activate(cross(cur)))
}

// Output:
// G2 declared v1-0000003,v1-0000004,v1-0000005
// active: v1-0000004,v1-0000005
// own ids: v1-0000004,v1-0000005
