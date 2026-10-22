// Package managed is a dapp in alice's namespace whose admins change its
// triggers through the admins library. It opens from Activate, never from
// init, so a filetest opens it once names are enforced.
package managed

import (
	probe "gno.land/p/bob/probe/v0"
	"gno.land/p/dev/gnotif/admins/v0"
	"gno.land/p/nt/testutils/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var triggers *admins.Admins

// Activate opens the dapp's handle and gives it to its admins, alice and carol.
func Activate(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	triggers = admins.New(gnotif.Open(cross(me)), testutils.TestAddress("alice"), testutils.TestAddress("carol"))
}

func SetTrigger(cur realm, name string, t gnotif.Trigger) string {
	return triggers.Set(0, cur.Sub(gnotif.Subpath), name, t)
}

func RemoveTrigger(cur realm, name string) {
	triggers.Remove(0, cur.Sub(gnotif.Subpath), name)
}

// SetWithStaleMe mints its gnotif identity here, then uses it one crossing
// frame down, where it is no longer current.
func SetWithStaleMe(cur realm, name string, t gnotif.Trigger) string {
	me := cur.Sub(gnotif.Subpath)
	return setWith(cross(cur), me, name, t)
}

func setWith(cur realm, me realm, name string, t gnotif.Trigger) string {
	return triggers.Set(0, me, name, t)
}

// Probe reports what /p/ code sees of the dapp's cur and of its gnotif
// identity, minted here or in a helper the dapp calls.
func Probe(cur realm) string {
	me := cur.Sub(gnotif.Subpath)
	return probe.Who(0, cur) + "\n" + probe.Who(0, me) + "\n" + viaHelper(cur)
}

func viaHelper(cur realm) string { return probe.Who(0, cur.Sub(gnotif.Subpath)) }
