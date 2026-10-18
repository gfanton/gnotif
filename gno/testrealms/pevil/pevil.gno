// Package pevil is /p/ code an attacker publishes.
package pevil

// AV is a plugin used by value: its method runs as the realm that calls it.
type AV [1]func(int, realm)

func (a AV) Setup(_ int, rlm realm) { a[0](0, rlm.Sub("gnotif")) }
