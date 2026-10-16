// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"

	chat "gno.land/r/alice/chat/v0"
	reg "gno.land/r/dev/gnotif/v1"
)

// chat's init holds v1-0000001 (Message, param to) and v1-0000002 (Reply).
var _ = chat.Again

// Every page, in v0's format, with this registry's links.
func main(cur realm) {
	println(chain.PackageAddress("gno.land/r/alice/chat/v0"))
	println(reg.Render(""))
	println(reg.Render("target/gno.land/r/alice/chat/v0"))
	println(reg.Render("trigger/v1-0000002"))
	println(reg.Render("page/2"))
	println(reg.Render("page/0"))
	println(reg.Render("trigger/v1-9999999"))
}

// Output:
// g1gg5tndezmk4g3m7lh6t0w94psd28ruw5t9hjhc
// # gnotif triggers
//
// - [**v1-0000002**](/r/dev/gnotif/v1:trigger/v1-0000002) gno\.land/r/alice/chat/v0 · Reply · ✓ verified · by g1gg5tndezmk4g3m7lh6t0w94psd28ruw5t9hjhc
// - [**v1-0000001**](/r/dev/gnotif/v1:trigger/v1-0000001) gno\.land/r/alice/chat/v0 · Message · param to · ✓ verified · by g1gg5tndezmk4g3m7lh6t0w94psd28ruw5t9hjhc
//
// # gnotif triggers for gno\.land/r/alice/chat/v0
//
// - [**v1-0000002**](/r/dev/gnotif/v1:trigger/v1-0000002) gno\.land/r/alice/chat/v0 · Reply · ✓ verified · by g1gg5tndezmk4g3m7lh6t0w94psd28ruw5t9hjhc
// - [**v1-0000001**](/r/dev/gnotif/v1:trigger/v1-0000001) gno\.land/r/alice/chat/v0 · Message · param to · ✓ verified · by g1gg5tndezmk4g3m7lh6t0w94psd28ruw5t9hjhc
//
// [All triggers](/r/dev/gnotif/v1)
//
// # Trigger v1-0000002
//
// - Target: gno\.land/r/alice/chat/v0
// - Event: Reply
// - Filter:
// - Param:
// - Title: New reply
// - Body:
// - Link: /
// - Declarer: g1gg5tndezmk4g3m7lh6t0w94psd28ruw5t9hjhc
// - ✓ verified
//
// [All triggers](/r/dev/gnotif/v1)
//
// # gnotif triggers
//
// No triggers on this page.
//
// Not found.
// Not found.
