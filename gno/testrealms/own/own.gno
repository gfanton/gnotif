// Package own is a dapp in the address namespace of gno.land/r/test/main, the
// filetests' realm. Its init opens nothing.
package own

import gnotif "gno.land/r/dev/gnotif/v1"

// Activate opens the dapp's handle and sets its trigger.
func Activate(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	h := gnotif.Open(cross(me))
	return h.Set(cross(me), "message", gnotif.Trigger{Event: "Message", Title: "New message", Link: "/"})
}
