// PKGPATH: gno.land/r/test/main

package main

import (
	"testing"

	"gno.land/p/nt/testutils/v0"
	forgot "gno.land/r/alice/forgot/v0"
	reg "gno.land/r/dev/gnotif/v1"
)

// After activation, a Declare that names one of the realm's two triggers and
// nothing new panics, as code that looks up one trigger's id would.
func main(cur realm) {
	testing.SetRealm(testing.NewUserRealm(testutils.TestAddress("alice")))
	a := reg.Trigger{Event: "A", Title: "a", Link: "/"}
	b := reg.Trigger{Event: "B", Title: "b", Link: "/"}
	println(forgot.List(cross(cur), a, b))
	println(forgot.List(cross(cur), a))
}

// Output:
// v1-0000003,v1-0000004

// Error:
// the triggers of gno.land/r/alice/forgot/v0 are set: deploy a new version of the realm to change them
