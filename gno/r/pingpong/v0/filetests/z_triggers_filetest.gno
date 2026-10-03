// PKGPATH: gno.land/r/demo/main

package main

import (
	"chain"

	"gno.land/r/dev/pingpong/v0"
)

func main(cur realm) {
	pingpong.DeclareTriggers(cross(cur))
	println(chain.PackageAddress("gno.land/r/dev/pingpong/v0"))
}

// Output:
// g1tw6tyv67zjyf26nskj6g5z9fhtvv4996w50xwf

// Events:
// [
//   {
//     "type": "TriggerDeclared",
//     "attrs": [
//       {
//         "key": "id",
//         "value": "0000001"
//       },
//       {
//         "key": "target",
//         "value": "gno.land/r/dev/pingpong/v0"
//       },
//       {
//         "key": "event",
//         "value": "TurnPlayed"
//       },
//       {
//         "key": "filter",
//         "value": ""
//       },
//       {
//         "key": "param",
//         "value": "next"
//       },
//       {
//         "key": "title",
//         "value": "Your turn"
//       },
//       {
//         "key": "body",
//         "value": "Game {game}, turn {turn}"
//       },
//       {
//         "key": "link",
//         "value": "/?game={game}"
//       },
//       {
//         "key": "declarer",
//         "value": "g1tw6tyv67zjyf26nskj6g5z9fhtvv4996w50xwf"
//       },
//       {
//         "key": "verified",
//         "value": "true"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/gnotif/v0"
//   }
// ]
