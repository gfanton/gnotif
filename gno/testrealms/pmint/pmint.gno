package pmint

// MakeMinter returns a closure that remembers the storage realm current when
// it was built.
func MakeMinter() any {
	return func(_ int, r realm) realm { return r.Sub("gnotif") }
}
