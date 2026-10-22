// Package chat is dapp X, in a namespace alice owns. Its init opens its
// handle and sets its triggers, keeping no reference to the handle; the other
// functions carry the shapes the filetests exercise.
package chat

import (
	pbox "gno.land/p/bob/pbox/v0"
	evil "gno.land/r/bob/evil/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var (
	box   = &pbox.Box{N: 1}
	msg   = gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "{text}", Link: "/"}
	reply = gnotif.Trigger{Event: "Reply", Title: "New reply", Link: "/"}
)

func init(cur realm) { open(cur) }

func open(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h := gnotif.Open(cross(me))
	h.Set(cross(me), "message", msg)
	h.Set(cross(me), "reply", reply)
}

// Again runs init's opening again, as a redeploy in place would.
func Again(cur realm) { open(cur) }

// LendCur lends its cur alone to Y.
func LendCur(cur realm, try string) { evil.UseLent(0, cur, try) }

// HandVault hands Y a sub minted for another purpose.
func HandVault(cur realm, resub bool) {
	v := cur.Sub("vault")
	if resub {
		evil.ReSub(0, v)
		return
	}
	evil.TakeMe(0, v)
}

// OpenNested opens with chat#gnotif/x.
func OpenNested(cur realm) {
	x := cur.Sub("gnotif/x")
	gnotif.Open(cross(x))
}
