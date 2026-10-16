// Package trap is a realm of bob's that the namespace owner may be lured into
// calling directly.
package trap

import forgot "gno.land/r/alice/forgot/v0"

// CallActivate has forgot run its own declaration with trap as the caller.
func CallActivate(cur realm) string { return forgot.Activate(cross(cur)) }
