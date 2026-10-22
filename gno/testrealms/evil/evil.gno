// Package evil is realm Y, the attacker. Most paths end in report, which
// opens a handle with the identity it obtained and sets a fake trigger.
package evil

import (
	pbox "gno.land/p/bob/pbox/v0"
	pevil "gno.land/p/bob/pevil/v0"
	pmint "gno.land/p/bob/pmint/v0"
	leaky "gno.land/r/alice/leaky/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

var fake = gnotif.Trigger{Event: "Fake", Title: "Claim your prize", Link: "/claim"}

// ViaBox is G1: a lent cur plus a lender-stamped /p/ box.
func ViaBox(_ int, rlm realm, b *pbox.Box) {
	f := b.Do(pmint.MakeMinter).(func(int, realm) realm)
	report("G1", f(0, rlm))
}

// UseLent holds a lent cur and nothing else.
func UseLent(_ int, rlm realm, try string) {
	switch try {
	case "open":
		gnotif.Open(cross(rlm))
	case "sub":
		report("lent", rlm.Sub(gnotif.Subpath))
	}
}

// TakeMe receives the victim's gnotif identity.
func TakeMe(_ int, me realm) { report("G3", me) }

// Plugin is G2: a /p/ plugin value whose method the victim runs with its cur.
func Plugin() pevil.AV { return pevil.AV{sink} }

func sink(_ int, me realm) { report("G2", me) }

// ReSub tries to turn a sub of the victim into its gnotif identity.
func ReSub(_ int, v realm) { report("resub", v.Sub(gnotif.Subpath)) }

// UseLeaked sets a trigger through leaky's exported handle, with Y's gnotif
// identity or with Y's own cur.
func UseLeaked(cur realm, as string) {
	switch as {
	case "me":
		me := cur.Sub(gnotif.Subpath)
		println(leaky.H.Set(cross(me), "fake", fake))
	case "cur":
		println(leaky.H.Set(cross(cur), "fake", fake))
	}
}

// UseLeakedAdmins sets a trigger through leaky's exported admins value with
// Y's own gnotif identity.
func UseLeakedAdmins(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	println(leaky.A.Set(0, me, "fake", fake))
}

func report(tag string, me realm) {
	h := gnotif.Open(cross(me))
	println(tag, "set", h.Set(cross(me), "fake", fake))
}
