// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"strings"
	"testing"

	"gno.land/p/nt/testutils/v0"
	chat "gno.land/r/alice/chat/v0"
	forgot "gno.land/r/alice/forgot/v0"
	leaky "gno.land/r/alice/leaky/v0"
	evil "gno.land/r/bob/evil/v0"
	trap "gno.land/r/bob/trap/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

const ftarget = "gno.land/r/alice/forgot/v0"

var (
	alice = testutils.TestAddress("alice")
	bob   = testutils.TestAddress("bob")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
	turn  = reg.Trigger{Event: "Turn", Param: "player", Title: "Your turn", Body: "Game {game}", Link: "/"}
)

// enableNames registers alice and bob, then turns namespace enforcement on.
func enableNames(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
}

var (
	_    = chat.Again
	_    = forgot.Activate
	_    = leaky.H
	_    = evil.UseLeaked
	_    = trap.CallRevoke
	_    = strings.Repeat
	_, _ = alice, bob
	_    = turn
	_    = enableNames
)

// Set on a name in use keeps the id when only the text changes, and an
// identical Set emits nothing.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.Activate(cross(cur))
	a := forgot.Set(cross(cur), "turn", turn)
	edited := turn
	edited.Title = "Your move"
	b := forgot.Set(cross(cur), "turn", edited)
	c := forgot.Set(cross(cur), "turn", edited)
	println(a, b, c)
	println(reg.Render("trigger/" + a))
}

// Output:
// v1-0000003 v1-0000003 v1-0000003
// # Trigger v1-0000003
//
// - Target: gno\.land/r/alice/forgot/v0
// - Name: turn
// - Event: Turn
// - Filter:
// - Param: player
// - Title: Your move
// - Body: Game {game}
// - Link: /
// - Declarer: g10pd4hatxfy7hzf9uh9pvjfsh8c0f3084sfdmq7
// - ✓ verified
//
// [All triggers](/r/dev/gnotif/v1)

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
//     "type": "TriggerDeclared",
//     "attrs": [
//       {
//         "key": "id",
//         "value": "v1-0000003"
//       },
//       {
//         "key": "target",
//         "value": "gno.land/r/alice/forgot/v0"
//       },
//       {
//         "key": "name",
//         "value": "turn"
//       },
//       {
//         "key": "event",
//         "value": "Turn"
//       },
//       {
//         "key": "filter",
//         "value": ""
//       },
//       {
//         "key": "param",
//         "value": "player"
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
//         "value": "/"
//       },
//       {
//         "key": "declarer",
//         "value": "g10pd4hatxfy7hzf9uh9pvjfsh8c0f3084sfdmq7"
//       },
//       {
//         "key": "verified",
//         "value": "true"
//       },
//       {
//         "key": "approver",
//         "value": "g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/gnotif/v1"
//   },
//   {
//     "type": "TriggerDeclared",
//     "attrs": [
//       {
//         "key": "id",
//         "value": "v1-0000003"
//       },
//       {
//         "key": "target",
//         "value": "gno.land/r/alice/forgot/v0"
//       },
//       {
//         "key": "name",
//         "value": "turn"
//       },
//       {
//         "key": "event",
//         "value": "Turn"
//       },
//       {
//         "key": "filter",
//         "value": ""
//       },
//       {
//         "key": "param",
//         "value": "player"
//       },
//       {
//         "key": "title",
//         "value": "Your move"
//       },
//       {
//         "key": "body",
//         "value": "Game {game}"
//       },
//       {
//         "key": "link",
//         "value": "/"
//       },
//       {
//         "key": "declarer",
//         "value": "g10pd4hatxfy7hzf9uh9pvjfsh8c0f3084sfdmq7"
//       },
//       {
//         "key": "verified",
//         "value": "true"
//       },
//       {
//         "key": "approver",
//         "value": "g1v9kxjcm9ta047h6lta047h6lta047h6lzd40gh"
//       }
//     ],
//     "pkg_path": "gno.land/r/dev/gnotif/v1"
//   }
// ]
