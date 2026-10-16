// Package evil is realm Y, the attacker. Every path ends in report, which
// declares a fake trigger with the identity it obtained.
package evil

import (
	"strings"

	pbox "gno.land/p/bob/pbox/v0"
	pevil "gno.land/p/bob/pevil/v0"
	pmint "gno.land/p/bob/pmint/v0"
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
	case "declare":
		gnotif.Declare(cross(rlm), fake)
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

func report(tag string, me realm) {
	println(tag, "declared", strings.Join(gnotif.Declare(cross(me), fake), ","))
}
