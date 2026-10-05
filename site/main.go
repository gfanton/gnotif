// Command site renders the gnotif website: the landing page and the
// documentation pages, from the markdown in this repository.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "repository root the markdown is read from")
	out := flag.String("out", "site/dist", "directory the site is written to")
	flag.Parse()

	if err := build(*root, *out); err != nil {
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}
}
