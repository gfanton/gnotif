// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"strings"
	"testing"

	"gno.land/p/nt/markdown/sanitize/v0"
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

// A title and a body that hold the record codec's separator and digits come
// back from the trigger page exactly.
func main(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.Activate(cross(cur))
	id := forgot.Set(cross(cur), "score", reg.Trigger{Event: "Score", Title: "Score: 12:3", Body: "{a}:{b}", Link: "/"})
	page := reg.Render("trigger/" + id)
	println(strings.Contains(page, "- Title: "+sanitize.InlineText("Score: 12:3")+"\n"))
	println(strings.Contains(page, "- Body: "+sanitize.InlineText("{a}:{b}")+"\n"))
}

// Output:
// true
// true
