// PKGPATH: gno.land/r/demo/main

package main

import "gno.land/r/dev/gnotif/v0"

func main(cur realm) {
	gnotif.Declare(cross(cur), "gno.land/r/demo/main", "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}")
	gnotif.Declare(cross(cur), "gno.land/r/demo/other", "Moved", "", "", "Moved", "", "/")
	println(gnotif.Render(""))
}

// Output:
// # gnotif triggers
//
// - [**0000002**](/r/demo/main:trigger/0000002) gno\.land/r/demo/other · Moved · by g1e8cq4xmeza97f78c2kd6lzwf8hnyet34me44mu
// - [**0000001**](/r/demo/main:trigger/0000001) gno\.land/r/demo/main · TurnPlayed · param next · ✓ verified · by g1e8cq4xmeza97f78c2kd6lzwf8hnyet34me44mu
