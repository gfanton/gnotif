package echo

import (
	"chain"
	"strings"
	"testing"

	"gno.land/p/nt/uassert/v0"

	"gno.land/r/dev/gnotif/v0"
)

func TestInitDeclaresTrigger(t *testing.T) {
	out := gnotif.Render("target/gno.land/r/dev/echo/v0")
	uassert.Equal(t, 1, strings.Count(out, "\n- ["), out)
	by := chain.PackageAddress("gno.land/r/dev/echo/v0").String()
	uassert.True(t, strings.Contains(out, " · Echo · param to · ✓ verified · by "+by+"\n"), out)
}

func TestRender(t *testing.T) {
	out := Render("")
	uassert.True(t, strings.Contains(out, "Echo"), out)
	uassert.True(t, strings.Contains(out, "your own address"), out)
}
