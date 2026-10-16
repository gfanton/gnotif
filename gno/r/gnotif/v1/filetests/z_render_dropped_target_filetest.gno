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

// chat's init holds v1-0000001 and v1-0000002.
var _ = chat.Again

// A target whose triggers were all dropped is as unknown as one that never
// declared, and its triggers are gone from the trigger and list pages.
func main(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
	testing.SetRealm(testing.NewUserRealm(alice))
	reg.Drop(cross(cur), ctarget, "v1-0000001,v1-0000002")
	println(reg.Render("target/" + ctarget))
	println(reg.Render("trigger/v1-0000001"))
	println(reg.Render(""))
}

// Output:
// Not found.
// Not found.
// # gnotif triggers
//
// No triggers on this page.
