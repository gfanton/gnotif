package watch

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrInvalidRegistry reports a registry path that is not a versioned realm path.
var ErrInvalidRegistry = errors.New("invalid registry path")

var (
	versionElem  = regexp.MustCompile(`^v(0|[1-9][0-9]*)$`)
	registryPath = regexp.MustCompile(`^gno\.land/r/[^/]+(/[^/]+)*$`)
)

// Registry names a registry realm version and, through it, every later
// version of the same realm.
type Registry struct {
	prefix string // the realm path without its version element
	first  int
}

// ParseRegistry reads a path of the form gno.land/r/<path>/v<N>.
func ParseRegistry(path string) (Registry, error) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return Registry{}, fmt.Errorf("%w %q: want gno.land/r/<path>/v<N>", ErrInvalidRegistry, path)
	}
	prefix := path[:i]
	n, ok := parseVersion(path[i+1:])
	if !ok || !registryPath.MatchString(prefix) {
		return Registry{}, fmt.Errorf("%w %q: want gno.land/r/<path>/v<N>", ErrInvalidRegistry, path)
	}
	return Registry{prefix: prefix, first: n}, nil
}

// Version returns the version of pkgPath when it is the registry realm at or
// after the configured version.
func (r Registry) Version(pkgPath string) (int, bool) {
	rest, ok := strings.CutPrefix(pkgPath, r.prefix+"/")
	if !ok {
		return 0, false
	}
	n, ok := parseVersion(rest)
	if !ok || n < r.first {
		return 0, false
	}
	return n, true
}

// idVersion returns the registry version that issues id: N for v<N>-<rest>,
// and 0 for the unprefixed ids of the first version.
func idVersion(id string) int {
	elem, _, ok := strings.Cut(id, "-")
	if !ok {
		return 0
	}
	n, ok := parseVersion(elem)
	if !ok {
		return 0
	}
	return n
}

func parseVersion(elem string) (int, bool) {
	if !versionElem.MatchString(elem) {
		return 0, false
	}
	n, err := strconv.Atoi(elem[1:])
	return n, err == nil
}
