// Package pa is a dapp in alice's personal-address namespace.
package pa

import (
	"strings"

	gnotif "gno.land/r/dev/gnotif/v1"
)

// Setup declares the dapp's trigger.
func Setup(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	return strings.Join(gnotif.Declare(cross(me), gnotif.Trigger{Event: "Message", Title: "New message", Link: "/"}), ",")
}
