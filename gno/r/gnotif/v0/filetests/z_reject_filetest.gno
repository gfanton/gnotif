// PKGPATH: gno.land/r/demo/main

package main

import "gno.land/r/dev/gnotif/v0"

func main(cur realm) {
	gnotif.Declare(cross(cur), "gno.land/r/demo/other", "Moved", "", "", "Moved", "", "/")
}

// Error:
// caller is not the target
