// Package managed is a dapp whose deployer changes its trigger through the
// admins library, in transactions after the deploy.
package managed

import (
	"chain"

	"gno.land/p/dev/gnotif/admins/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var triggers *admins.Admins

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	triggers = admins.New(gnotif.Open(cross(me)), cur.Previous().Address())
}

// SetTitle sets the Ping trigger with title, and returns its id.
func SetTitle(cur realm, title string) string {
	return triggers.Set(0, cur.Sub(gnotif.Subpath), "ping", gnotif.Trigger{Event: "Ping", Param: "to", Title: title, Body: "{msg}", Link: "/"})
}

// Remove removes the Ping trigger.
func Remove(cur realm) {
	triggers.Remove(0, cur.Sub(gnotif.Subpath), "ping")
}

// Ping sends msg to the browsers that opted in with the caller's address.
func Ping(cur realm, msg string) {
	chain.Emit("Ping", "to", cur.Previous().Address().String(), "msg", msg)
}
