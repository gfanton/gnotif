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

// enableNames registers alice and bob, then turns namespace enforcement on.
func enableNames(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
}

var _ = chat.Again

// Dropping every trigger of a target emits one TriggerRemoved per id, in the
// order listed, and leaves the target with no active ids.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	reg.Drop(cross(cur), ctarget, "v1-0000002,v1-0000001")
	println("active:", reg.Active(ctarget))
}

// Output:
// active:

// Events:
// [
//   {
//     "type": "Registered",
//     "attrs": [
//       {
//         "key": "name",
//         "value": "alice"
//       },
//       {
//         "key": "address",
//         "value": "g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh"
//       }
//     ],
//     "pkg_path": "gno.land/r/sys/users"
//   },
//   {
//     "type": "Registered",
//     "attrs": [
//       {
//         "key": "name",
//         "value": "bob"
//       },
//       {
//         "key": "address",
//         "value": "g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu"
//       }
//     ],
//     "pkg_path": "gno.land/r/sys/users"
//   },
//   {
//     "type": "TriggerRemoved",
//     "attrs": [
//       {
//         "key": "id",
//         "value": "v1-0000002"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/gnotif/v1"
//   },
//   {
//     "type": "TriggerRemoved",
//     "attrs": [
//       {
//         "key": "id",
//         "value": "v1-0000001"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/gnotif/v1"
//   }
// ]
