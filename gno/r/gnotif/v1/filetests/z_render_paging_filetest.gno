// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"strings"
	"testing"

	"gno.land/p/nt/testutils/v0"
	forgot "gno.land/r/alice/forgot/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

var (
	alice = testutils.TestAddress("alice")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
)

// With chat's two, 64 more triggers fill the first page with the 50 newest and
// leave the 16 oldest to the second, which has no next page.
func main(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
	testing.SetRealm(testing.NewUserRealm(alice))
	ids := strings.Split(forgot.Many(cross(cur), 64), ",")
	page1 := reg.Render("")
	page2 := reg.Render("page/2")
	println(len(ids), strings.Count(page1, "\n- [**"), strings.Count(page2, "\n- [**"))
	println(strings.HasPrefix(page1, "# gnotif triggers\n\n- [**"+ids[63]+"**]"))
	println(strings.Contains(page1, "**"+ids[14]+"**"), strings.Contains(page1, "**"+ids[13]+"**"))
	println(strings.HasPrefix(page2, "# gnotif triggers\n\n- [**"+ids[13]+"**]"))
	println(strings.Contains(page2, "**"+ids[0]+"**"))
	println(strings.Contains(page1, "\n[Next page](/r/dev/gnotif/v1:page/2)\n"))
	println(strings.Contains(page2, "[Next page]"))
	println(reg.Render("page/3"))
}

// Output:
// 64 50 16
// true
// true false
// true
// true
// true
// false
// # gnotif triggers
//
// No triggers on this page.
