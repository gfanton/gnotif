package pbox

// Box is a generic /p/ container with a higher-order method.
type Box struct{ N int }

func (b *Box) Do(f func() any) any { return f() }
