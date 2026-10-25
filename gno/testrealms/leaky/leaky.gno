// Package leaky is a dapp in alice's namespace that exports its handle and
// the admins value that holds it.
package leaky

import (
	"gno.land/p/dev/gnotif/admins/v0"
	"gno.land/p/nt/testutils/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var (
	// H is the dapp's handle, exported by mistake.
	H *gnotif.Handle
	// A is the dapp's admins value, exported by mistake.
	A *admins.Admins
)

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	H = gnotif.Open(cross(me))
	A = admins.New(H, testutils.TestAddress("alice"))
}

// CopyThenSet sets a trigger through a copy of its own handle.
func CopyThenSet(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	c := *H
	c.Set(cross(me), "copy", gnotif.Trigger{Event: "Copy", Title: "copy", Link: "/"})
}
