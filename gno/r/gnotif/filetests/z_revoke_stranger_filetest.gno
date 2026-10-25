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

// Only the namespace owner revokes.
func main(cur realm) {
	enableNames(cur)
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.Activate(cross(cur))
	testing.SetRealm(testing.NewUserRealm(bob))
	reg.Revoke(cross(cur), ftarget)
}

// Error:
// g1vfhkyh6lta047h6lta047h6lta047h6l03vdhu is not authorized for namespace alice
