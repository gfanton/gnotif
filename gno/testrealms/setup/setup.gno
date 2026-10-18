// Package setup registers test names. Call it at height 0.
package setup

import (
	"gno.land/p/nt/testutils/v0"
	"gno.land/r/sys/users"
)

// Names registers alice and bob, and makes caller a users controller.
func Names(cur realm, controller address) {
	must(users.RegisterUser(cross(cur), "alice", testutils.TestAddress("alice")))
	must(users.RegisterUser(cross(cur), "bob", testutils.TestAddress("bob")))
	users.AddControllerAtGenesis(cross(cur), controller)
}

func must(err error) {
	if err != nil {
		panic(err.Error())
	}
}
