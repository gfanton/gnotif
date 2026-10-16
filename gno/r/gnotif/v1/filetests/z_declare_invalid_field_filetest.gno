// PKGPATH: gno.land/r/test/main

package main

import (
	"testing"

	"gno.land/p/nt/testutils/v0"
	forgot "gno.land/r/alice/forgot/v0"
	reg "gno.land/r/dev/gnotif/v1"
)

var alice = testutils.TestAddress("alice")

// A list whose second trigger breaks a field rule panics.
func main(cur realm) {
	testing.SetRealm(testing.NewUserRealm(alice))
	ok := reg.Trigger{Event: "A", Title: "a", Link: "/"}
	bad := reg.Trigger{Event: "B", Title: "b", Link: "//evil.example"}
	println(forgot.List(cross(cur), ok, bad))
}

// Error:
// invalid link
