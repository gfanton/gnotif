// PKGPATH: gno.land/r/demo/main

package main

import (
	"chain"

	"gno.land/r/dev/gnotif/v0"
	_ "gno.land/r/dev/pingpong/v0"
)

func main() {
	println(chain.PackageAddress("gno.land/r/dev/pingpong/v0"))
	println(gnotif.Render("target/gno.land/r/dev/pingpong/v0"))
}

// Output:
// g1tw6tyv67zjyf26nskj6g5z9fhtvv4996w50xwf
// # gnotif triggers for gno\.land/r/dev/pingpong/v0
//
// - [**0000001**](/r/dev/gnotif/v0:trigger/0000001) gno\.land/r/dev/pingpong/v0 · TurnPlayed · param next · ✓ verified · by g1tw6tyv67zjyf26nskj6g5z9fhtvv4996w50xwf
//
// [All triggers](/r/dev/gnotif/v0)
