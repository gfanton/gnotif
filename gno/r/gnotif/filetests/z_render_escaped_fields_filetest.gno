// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	"gno.land/p/nt/testutils/v0"
	chat "gno.land/r/alice/chat/v0"
	forgot "gno.land/r/alice/forgot/v0"
	reg "gno.land/r/dev/gnotif/v1"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

var (
	alice = testutils.TestAddress("alice")
	admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")
)

// chat's init holds v1-0000001 and v1-0000002, so the new trigger is v1-0000003.
var _ = chat.Again

// Every field a dapp supplies is escaped on the trigger page and on the
// target page, and each lands on its own line.
func main(cur realm) {
	testing.SetHeight(0)
	setup.Names(cross(cur), chain.PackageAddress("gno.land/r/test/main"))
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
	testing.SetRealm(testing.NewUserRealm(alice))
	forgot.Activate(cross(cur))
	id := forgot.Set(cross(cur), "hot_sale", reg.Trigger{
		Event:  "Ev_ent_1",
		Filter: "k_1=v*1,k_2=a.b",
		Param:  "pa_ram",
		Title:  "Hot *sale* [now]!",
		Body:   "(a) ~b~ > c + d - e # f <g> & h `i` j_k a\\b",
		Link:   "/p_q?x=1&y=(2)#z",
	})
	println(id)
	println(reg.Render("trigger/" + id))
	println(reg.Render("target/gno.land/r/alice/forgot/v0"))
}

// Output:
// v1-0000003
// # Trigger v1-0000003
//
// - Target: gno\.land/r/alice/forgot/v0
// - Name: hot\_sale
// - Event: Ev\_ent\_1
// - Filter: k\_1=v\*1,k\_2=a\.b
// - Param: pa\_ram
// - Title: Hot \*sale\* \[now\]\!
// - Body: \(a\) \~b\~ \> c \+ d \- e \# f \<g\> \& h \`i\` j\_k a\\b
// - Link: /p\_q?x=1\&y=\(2\)\#z
// - Declarer: g10pd4hatxfy7hzf9uh9pvjfsh8c0f3084sfdmq7
// - ✓ verified
//
// [All triggers](/r/dev/gnotif/v1)
//
// # gnotif triggers for gno\.land/r/alice/forgot/v0
//
// - [**v1-0000003**](/r/dev/gnotif/v1:trigger/v1-0000003) gno\.land/r/alice/forgot/v0 · hot\_sale · Ev\_ent\_1 · param pa\_ram · ✓ verified · by g10pd4hatxfy7hzf9uh9pvjfsh8c0f3084sfdmq7
//
// [All triggers](/r/dev/gnotif/v1)
