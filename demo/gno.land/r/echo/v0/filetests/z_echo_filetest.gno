// PKGPATH: gno.land/r/demo/main

package main

import (
	"testing"

	"gno.land/p/nt/testutils/v0"

	"gno.land/r/dev/echo/v0"
)

// Each message names its own caller, whatever the caller writes.
func main(cur realm) {
	testing.SetRealm(testing.NewUserRealm(testutils.TestAddress("alice")))
	echo.Echo(cross(cur), "hello")

	testing.SetRealm(testing.NewUserRealm(testutils.TestAddress("bob")))
	echo.Echo(cross(cur), "hi, it's bob")
}

// Events:
// [
//   {
//     "type": "Echo",
//     "attrs": [
//       {
//         "key": "to",
//         "value": "g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh"
//       },
//       {
//         "key": "msg",
//         "value": "hello"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/echo/v0"
//   },
//   {
//     "type": "Echo",
//     "attrs": [
//       {
//         "key": "to",
//         "value": "g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu"
//       },
//       {
//         "key": "msg",
//         "value": "hi, it's bob"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/echo/v0"
//   }
// ]
