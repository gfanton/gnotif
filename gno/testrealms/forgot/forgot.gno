// Package forgot is a second dapp in alice's namespace. Its init opens
// nothing: it opens from Activate and keeps its handle, and any caller may
// change its triggers.
package forgot

import (
	"strconv"
	"strings"

	pbox "gno.land/p/bob/pbox/v0"
	evil "gno.land/r/bob/evil/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var (
	box = &pbox.Box{N: 1}
	h   *gnotif.Handle
)

// Activate opens the dapp's handle.
func Activate(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h = gnotif.Open(cross(me))
}

// Set sets t under name.
func Set(cur realm, name string, t gnotif.Trigger) string {
	me := cur.Sub(gnotif.Subpath)
	return h.Set(cross(me), name, t)
}

// Remove removes the trigger under name.
func Remove(cur realm, name string) {
	me := cur.Sub(gnotif.Subpath)
	h.Remove(cross(me), name)
}

// Many sets n distinct triggers, t0 to t<n-1>, and returns their ids.
func Many(cur realm, n int) string {
	me := cur.Sub(gnotif.Subpath)
	ids := make([]string, n)
	for i := range ids {
		s := strconv.Itoa(i)
		ids[i] = h.Set(cross(me), "t"+s, gnotif.Trigger{Event: "E" + s, Title: "t", Link: "/"})
	}
	return strings.Join(ids, ",")
}

// Lend has the G1 bug: it lends Y, the attacker, its cur and a /p/ box it
// allocated.
func Lend(cur realm) { evil.ViaBox(0, cur, box) }

// HandMe is G3: it hands its gnotif identity to Y, on purpose (delegation) or
// by mistake.
func HandMe(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	evil.TakeMe(0, me)
}

// RunPlugin has the G2 bug: it runs Y's /p/ plugin with its cur.
func RunPlugin(cur realm) { evil.Plugin().Setup(0, cur) }

// LendThenActivate reaches the G1 bug before its own Open.
func LendThenActivate(cur realm) {
	Lend(cur)
	Activate(cur)
}

// ActivateThenLend reaches the G1 bug after its own Open.
func ActivateThenLend(cur realm) {
	Activate(cur)
	Lend(cur)
}
