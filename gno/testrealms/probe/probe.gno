// Package probe reports what /p/ code sees of a realm value it is given.
package probe

import "strconv"

func Who(_ int, rlm realm) string {
	s := rlm.PkgPath() + " current=" + strconv.FormatBool(rlm.IsCurrent())
	if p := rlm.Previous(); p != nil {
		s += " previous=" + p.PkgPath() + "@" + p.Address().String()
	}
	return s
}
