// PKGPATH: gno.land/r/test/main

package main

import (
	"testing"

	"gno.land/p/nt/testutils/v0"
	forgot "gno.land/r/alice/forgot/v0"
)

var alice = testutils.TestAddress("alice")

// A call with no triggers panics.
func main(cur realm) {
	testing.SetRealm(testing.NewUserRealm(alice))
	println(forgot.List(cross(cur)))
}

// Error:
// declare at least one trigger
