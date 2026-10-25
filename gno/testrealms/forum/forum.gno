// Package forum is a dapp in alice's namespace with two shapes: it runs a
// plugin with its cur, and it exports a helper that mints a gnotif identity
// from the realm it is given.
package forum

import gnotif "gno.land/r/dev/gnotif/v1"

// Hooker is a plugin the forum runs with its cur.
type Hooker interface{ Hook(_ int, rlm realm) }

// RunHook runs a plugin with the forum's cur.
func RunHook(cur realm, h Hooker) { h.Hook(0, cur) }

// Identity returns the gnotif identity of rlm.
func Identity(_ int, rlm realm) realm { return rlm.Sub(gnotif.Subpath) }
