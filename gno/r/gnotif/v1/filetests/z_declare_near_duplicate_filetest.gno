// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	chat "gno.land/r/alice/chat/v0"
	forgot "gno.land/r/alice/forgot/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

const ftarget = "gno.land/r/alice/forgot/v0"

var (
	alice = testutils.TestAddress("alice")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
)

// chat's init holds v1-0000001 and v1-0000002.
var _ = chat.Again

// Two triggers that differ in their link only are two triggers.
func main(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
	testing.SetRealm(testing.NewUserRealm(alice))
	a := reg.Trigger{Event: "A", Title: "a", Link: "/"}
	b := a
	b.Link = "/b"
	println(forgot.List(cross(cur), a, b))
	println(reg.Active(ftarget))
}

// Output:
// v1-0000003,v1-0000004
// v1-0000003,v1-0000004

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
//         "key": "event",
//         "value": "A"
//       },
//       {
//         "key": "filter",
//         "value": ""
//       },
//       {
//         "key": "param",
//         "value": ""
//       },
//       {
//         "key": "title",
//         "value": "a"
//       },
//       {
//         "key": "body",
//         "value": ""
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
//         "value": "v1-0000004"
//       },
//       {
//         "key": "target",
//         "value": "gno.land/r/alice/forgot/v0"
//       },
//       {
//         "key": "event",
//         "value": "A"
//       },
//       {
//         "key": "filter",
//         "value": ""
//       },
//       {
//         "key": "param",
//         "value": ""
//       },
//       {
//         "key": "title",
//         "value": "a"
//       },
//       {
//         "key": "body",
//         "value": ""
//       },
//       {
//         "key": "link",
//         "value": "/b"
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
