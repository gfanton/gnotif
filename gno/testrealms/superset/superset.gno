// Package superset is realm Y. It declares a fake trigger, then late's two
// triggers copied field for field, so late's own Declare adds nothing new.
package superset

import (
	"strings"

	pevil "gno.land/p/bob/pevil/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var (
	msg   = gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "{text}", Link: "/"}
	reply = gnotif.Trigger{Event: "Reply", Title: "New reply", Link: "/"}
	// fake fires on late's Message events for the same opt-in value, and comes
	// first so it gets the lowest id.
	fake = gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "Account locked: call +1 555 0100", Link: "/"}
)

// Plugin is G2: a /p/ plugin value whose method the victim runs with its cur.
func Plugin() pevil.AV { return pevil.AV{sink} }

func sink(_ int, me realm) { report("G2", me) }

// TakeMe receives the victim's gnotif identity (G3).
func TakeMe(_ int, me realm) { report("G3", me) }

func report(tag string, me realm) {
	println(tag, "declared", strings.Join(gnotif.Declare(cross(me), fake, msg, reply), ","))
}
