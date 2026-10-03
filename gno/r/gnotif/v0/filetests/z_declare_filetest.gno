// PKGPATH: gno.land/r/demo/main

package main

import "gno.land/r/dev/gnotif/v0"

func main(cur realm) {
	id := gnotif.Declare(cross(cur), "gno.land/r/demo/main", "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}")
	println(id)
	gnotif.Remove(cross(cur), id)
}

// Output:
// 0000001

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
//         "value": "gno.land/r/demo/main"
//       },
//       {
//         "key": "event",
//         "value": "TurnPlayed"
//       },
//       {
//         "key": "filter",
//         "value": "mode=ranked"
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
//         "value": "Game {game}"
//       },
//       {
//         "key": "link",
//         "value": "/?game={game}"
//       },
//       {
//         "key": "declarer",
//         "value": "g1e8cq4xmeza97f78c2kd6lzwf8hnyet34me44mu"
//       },
//       {
//         "key": "verified",
//         "value": "true"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/gnotif/v0"
//   },
//   {
//     "type": "TriggerRemoved",
//     "attrs": [
//       {
//         "key": "id",
//         "value": "0000001"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/gnotif/v0"
//   }
// ]
