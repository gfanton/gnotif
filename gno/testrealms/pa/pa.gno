// Package pa is a dapp in alice's personal-address namespace.
package pa

import gnotif "gno.land/r/dev/gnotif/v1"

// Setup opens the dapp's handle and sets its trigger.
func Setup(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	h := gnotif.Open(cross(me))
	return h.Set(cross(me), "message", gnotif.Trigger{Event: "Message", Title: "New message", Link: "/"})
}
