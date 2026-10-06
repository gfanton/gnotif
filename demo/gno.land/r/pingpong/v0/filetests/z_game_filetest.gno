// PKGPATH: gno.land/r/demo/main

package main

import (
	"testing"

	"gno.land/p/nt/testutils/v0"

	"gno.land/r/dev/pingpong/v0"
)

func main(cur realm) {
	alice := testutils.TestAddress("alice")
	bob := testutils.TestAddress("bob")

	testing.SetRealm(testing.NewUserRealm(alice))
	id := pingpong.NewGame(cross(cur), bob)
	println(id)

	testing.SetRealm(testing.NewUserRealm(bob))
	pingpong.Accept(cross(cur), id)

	testing.SetRealm(testing.NewUserRealm(alice))
	pingpong.Play(cross(cur), id)

	testing.SetRealm(testing.NewUserRealm(bob))
	pingpong.Leave(cross(cur), id)
}

// Output:
// 0000001

// Events:
// [
//   {
//     "type": "GameInvited",
//     "attrs": [
//       {
//         "key": "game",
//         "value": "0000001"
//       },
//       {
//         "key": "first",
//         "value": "g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh"
//       },
//       {
//         "key": "second",
//         "value": "g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/pingpong/v0"
//   },
//   {
//     "type": "GameStarted",
//     "attrs": [
//       {
//         "key": "game",
//         "value": "0000001"
//       },
//       {
//         "key": "first",
//         "value": "g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh"
//       },
//       {
//         "key": "second",
//         "value": "g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/pingpong/v0"
//   },
//   {
//     "type": "TurnPlayed",
//     "attrs": [
//       {
//         "key": "game",
//         "value": "0000001"
//       },
//       {
//         "key": "next",
//         "value": "g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu"
//       },
//       {
//         "key": "turn",
//         "value": "1"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/pingpong/v0"
//   },
//   {
//     "type": "GameEnded",
//     "attrs": [
//       {
//         "key": "game",
//         "value": "0000001"
//       },
//       {
//         "key": "by",
//         "value": "g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/pingpong/v0"
//   }
// ]
