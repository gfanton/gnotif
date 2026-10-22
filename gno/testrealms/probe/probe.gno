// Package probe reports what /p/ code sees of a realm value it is given.
package probe

func Who(_ int, rlm realm) string {
	s := rlm.PkgPath() + " current=" + bool2s(rlm.IsCurrent())
	if p := rlm.Previous(); p != nil {
		s += " previous=" + p.PkgPath() + "@" + p.Address().String()
	}
	return s
}

func bool2s(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
