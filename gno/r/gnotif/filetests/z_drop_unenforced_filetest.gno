// PKGPATH: gno.land/r/test/main

package main

import (
	"testing"

	"gno.land/p/nt/testutils/v0"
	chat "gno.land/r/alice/chat/v0"
	reg "gno.land/r/dev/gnotif/v1"
)

var _ = chat.Again

// Without namespace enforcement r/sys/names would authorize every key, so the
// registry lets no key drop, alice included.
func main(cur realm) {
	testing.SetRealm(testing.NewUserRealm(testutils.TestAddress("alice")))
	reg.Drop(cross(cur), "gno.land/r/alice/chat/v0", "v1-0000001")
}

// Error:
// r/sys/names is not enabled, so no namespace has an owner
