// Package chat is dapp X, in a namespace alice owns. Its init declares its
// triggers; the other functions carry the shapes the filetests exercise.
package chat

import (
	"strings"

	pbox "gno.land/p/bob/pbox/v0"
	evil "gno.land/r/bob/evil/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var (
	box   = &pbox.Box{N: 1}
	msg   = gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "{text}", Link: "/"}
	reply = gnotif.Trigger{Event: "Reply", Title: "New reply", Link: "/"}
)

func init(cur realm) { declare(cur) }

func declare(cur realm) []string {
	me := cur.Sub(gnotif.Subpath)
	return gnotif.Declare(cross(me), msg, reply)
}

// Again runs init's declaration again, as a redeploy in place would.
func Again(cur realm) string { return strings.Join(declare(cur), ",") }

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

// DeclareNested declares with chat#gnotif/x.
func DeclareNested(cur realm) {
	x := cur.Sub("gnotif/x")
	gnotif.Declare(cross(x), msg)
}
