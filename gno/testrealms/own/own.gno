// Package own is a dapp in the address namespace of gno.land/r/test/main, the
// filetests' realm. Its init declares nothing.
package own

import (
	"strings"

	gnotif "gno.land/r/dev/gnotif/v1"
)

// Activate declares the dapp's trigger.
func Activate(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	return strings.Join(gnotif.Declare(cross(me), gnotif.Trigger{Event: "Message", Title: "New message", Link: "/"}), ",")
}
