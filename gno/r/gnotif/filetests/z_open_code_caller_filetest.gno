// PKGPATH: gno.land/r/test/main

package main

import (
	"chain"
	"testing"

	own "gno.land/r/g1pm0qlcryf0v0fftfcvahwux3mxt8t0rdnlqkqd/own/v0"
	"gno.land/r/sys/names"
	setup "gno.land/r/test/setup/v0"
)

var admin = address("g1skl80cuz8zq3lul9pgz5pc35l2pfzgxgfpsqkx")

// The caller is a code realm whose address is the signer's, as a MsgRun
// script's realm is: not a direct call.
func main(cur realm) {
	self := chain.PackageAddress("gno.land/r/test/main")
	testing.SetHeight(0)
	setup.Names(cross(cur), self)
	testing.SetHeight(123)
	testing.SetRealm(testing.NewUserRealm(admin))
	names.Enable(cross(cur))
	testing.SetOriginCaller(self)
	testing.SetRealm(testing.NewCodeRealm("gno.land/r/test/main"))
	println(own.Activate(cross(cur)))
}

// Error:
// the dapp was not entered directly by its namespace owner; entered from gno.land/r/test/main
