// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	chat "gno.land/r/alice/chat/v0"
	forgot "gno.land/r/alice/forgot/v0"
	forum "gno.land/r/alice/forum/v0"
	evil2 "gno.land/r/bob/evil2/v0"
	trap "gno.land/r/bob/trap/v0"
	reg "gno.land/r/dev/gnotif/v1"
	pa "gno.land/r/g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh/pa/v0"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

const (
	ctarget = "gno.land/r/alice/chat/v0"
	ftarget = "gno.land/r/alice/forgot/v0"
)

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

func as(a address) { testing.SetRealm(testing.NewUserRealm(a)) }

var (
	_    = chat.Again
	_    = forgot.Activate
	_    = forum.RunHook
	_    = evil2.Plug
	_    = trap.CallActivate
	_    = pa.Setup
	_    = reg.Active
	_, _ = alice, bob
	_    = enableNames
	_    = as
)

// forgot hands me to Y inside the owner's direct call, before any activation:
// Y's declaration is the first call and activates (delegation).
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.HandMe(cross(cur))
	println(reg.Active(ftarget))
}

// Output:
// G3 declared v1-0000003
// v1-0000003
