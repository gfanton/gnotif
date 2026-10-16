// Package forgot is a second dapp in alice's namespace. Its init declares
// nothing; every declaration comes from a later function.
package forgot

import (
	"strconv"
	"strings"

	pbox "gno.land/p/bob/pbox/v0"
	evil "gno.land/r/bob/evil/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var box = &pbox.Box{N: 1}

// Activate declares the dapp's trigger.
func Activate(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	return strings.Join(gnotif.Declare(cross(me), gnotif.Trigger{Event: "Message", Title: "New message", Link: "/"}), ",")
}

// Many declares n distinct triggers in one call.
func Many(cur realm, n int) string {
	me := cur.Sub(gnotif.Subpath)
	ts := make([]gnotif.Trigger, n)
	for i := range ts {
		ts[i] = gnotif.Trigger{Event: "E" + strconv.Itoa(i), Title: "t", Link: "/"}
	}
	return strings.Join(gnotif.Declare(cross(me), ts...), ",")
}

// List declares the given triggers.
func List(cur realm, ts ...gnotif.Trigger) string {
	me := cur.Sub(gnotif.Subpath)
	return strings.Join(gnotif.Declare(cross(me), ts...), ",")
}

// Lend has the G1 bug.
func Lend(cur realm) { evil.ViaBox(0, cur, box) }

// HandMe hands its gnotif identity to Y (delegation, or the G3 mistake).
func HandMe(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	evil.TakeMe(0, me)
}

// RunPlugin has the G2 bug.
func RunPlugin(cur realm) { evil.Plugin().Setup(0, cur) }

// LendThenActivate reaches the G1 bug before its own Declare.
func LendThenActivate(cur realm) string {
	Lend(cur)
	return Activate(cur)
}

// ActivateThenLend reaches the G1 bug after its own Declare.
func ActivateThenLend(cur realm) string {
	ids := Activate(cur)
	Lend(cur)
	return ids
}
