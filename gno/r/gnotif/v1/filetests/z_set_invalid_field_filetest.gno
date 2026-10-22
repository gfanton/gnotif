// PKGPATH: gno.land/r/test/main

package main

import (
	"testing"

	"gno.land/p/nt/testutils/v0"
	forgot "gno.land/r/alice/forgot/v0"
	reg "gno.land/r/dev/gnotif/v1"
)

var alice = testutils.TestAddress("alice")

// A trigger that breaks a field rule panics.
func main(cur realm) {
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.Activate(cross(cur))
	bad := reg.Trigger{Event: "B", Title: "b", Link: "//evil.example"}
	println(forgot.Set(cross(cur), "bad", bad))
}

// Error:
// invalid link
