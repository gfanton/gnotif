// Package late is a dapp in alice's namespace with chat's two triggers. It
// declares from functions, never from init, so a filetest makes its first
// Declare once names are enforced.
package late

import (
	"strings"

	superset "gno.land/r/bob/superset/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var (
	msg   = gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "{text}", Link: "/"}
	reply = gnotif.Trigger{Event: "Reply", Title: "New reply", Link: "/"}
)

// Activate declares the dapp's triggers.
func Activate(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	return strings.Join(gnotif.Declare(cross(me), msg, reply), ",")
}

// RunPlugin has the G2 bug.
func RunPlugin(cur realm) { superset.Plugin().Setup(0, cur) }

// PluginThenActivate reaches the G2 bug before its own Declare.
func PluginThenActivate(cur realm) string {
	RunPlugin(cur)
	return Activate(cur)
}

// HandMeThenActivate hands its gnotif identity to Y (G3), then declares.
func HandMeThenActivate(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	superset.TakeMe(0, me)
	return strings.Join(gnotif.Declare(cross(me), msg, reply), ",")
}
