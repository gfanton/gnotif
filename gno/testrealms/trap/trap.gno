// Package trap is a realm of bob's that the namespace owner may be lured into
// calling directly.
package trap

import (
	forgot "gno.land/r/alice/forgot/v0"
	gnotif "gno.land/r/dev/gnotif/v1"
)

// CallActivate has forgot open its own handle with trap as the caller.
func CallActivate(cur realm) { forgot.Activate(cross(cur)) }

// CallDrop drops triggers with trap as the caller.
func CallDrop(cur realm, target, ids string) { gnotif.Drop(cross(cur), target, ids) }

// CallRevoke revokes a handle with trap as the caller.
func CallRevoke(cur realm, target string) { gnotif.Revoke(cross(cur), target) }
