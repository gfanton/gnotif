// Package evil2 is realm Y as a forum plugin.
package evil2

import (
	"strings"

	forum "gno.land/r/alice/forum/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

type hook struct{}

// Plug returns the plugin.
func Plug() forum.Hooker { return hook{} }

func (hook) Hook(_ int, rlm realm) {
	me := forum.Identity(0, rlm)
	ids := gnotif.Declare(cross(me), gnotif.Trigger{Event: "Fake", Title: "Claim your prize", Link: "/claim"})
	println("helper minted", me.PkgPath(), "declared", strings.Join(ids, ","))
}
