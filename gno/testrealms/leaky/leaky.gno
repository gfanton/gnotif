// Package leaky is a dapp in alice's namespace that exports its handle.
package leaky

import gnotif "gno.land/r/dev/gnotif/v1"

// H is the dapp's handle, exported by mistake.
var H *gnotif.Handle

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	H = gnotif.Open(cross(me))
}

// CopyThenSet sets a trigger through a copy of its own handle.
func CopyThenSet(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	c := *H
	c.Set(cross(me), "copy", gnotif.Trigger{Event: "Copy", Title: "copy", Link: "/"})
}
