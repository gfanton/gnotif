// PKGPATH: gno.land/r/demo/main

package main

import "gno.land/r/dev/gnotif/v0"

func main(cur realm) {
	gnotif.Declare(cross(cur), "gno.land/r/demo/main", "TurnPlayed", "mode=ranked", "next", "Your turn", "Game {game}", "/?game={game}")
	gnotif.Declare(cross(cur), "gno.land/r/demo/other", "Moved", "", "", "Moved", "", "/")
	gnotif.Declare(cross(cur), "gno.land/r/demo/main", "Moved", "", "", "Moved", "", "/")
	println(gnotif.Render("target/gno.land/r/demo/main"))
}

// Output:
// # gnotif triggers for gno\.land/r/demo/main
//
// - [**0000003**](/r/dev/gnotif/v0:trigger/0000003) gno\.land/r/demo/main · Moved · ✓ verified · by g1e8cq4xmeza97f78c2kd6lzwf8hnyet34me44mu
// - [**0000001**](/r/dev/gnotif/v0:trigger/0000001) gno\.land/r/demo/main · TurnPlayed · param next · ✓ verified · by g1e8cq4xmeza97f78c2kd6lzwf8hnyet34me44mu
//
// [All triggers](/r/dev/gnotif/v0)
