package watch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRegistry(t *testing.T) {
	for path, first := range map[string]int{
		"gno.land/r/dev/gnotif/v0":  0,
		"gno.land/r/dev/gnotif/v12": 12,
	} {
		t.Run(path, func(t *testing.T) {
			r, err := ParseRegistry(path)
			require.NoError(t, err)
			assert.Equal(t, first, r.first)
		})
	}

	for _, path := range []string{
		"",
		"gno.land/r/dev/gnotif",
		"gno.land/r/dev/gnotif/v01",
		"gno.land/r/dev/gnotif/v1/extra",
		"gno.land/r/dev/gnotif/V1",
		"gno.land/p/dev/gnotif/v0",
		"gno.land/r/dev/gnotif/v0/",
		"gno.land/r/dev//v0",
		"gno.land/r/v0",
		"gno.land/r/dev/gnotif/v99999999999999999999",
	} {
		t.Run("invalid "+path, func(t *testing.T) {
			_, err := ParseRegistry(path)
			assert.ErrorIs(t, err, ErrInvalidRegistry)
		})
	}
}

func TestVersion(t *testing.T) {
	r, err := ParseRegistry("gno.land/r/dev/gnotif/v1")
	require.NoError(t, err)
	tests := []struct {
		path string
		want int
		ok   bool
	}{
		{"gno.land/r/dev/gnotif/v1", 1, true},
		{"gno.land/r/dev/gnotif/v2", 2, true},
		{"gno.land/r/dev/gnotif/v10", 10, true},
		{"gno.land/r/dev/gnotif/v0", 0, false},
		{"gno.land/r/dev/gnotif/v01", 0, false},
		{"gno.land/r/dev/gnotif/V2", 0, false},
		{"gno.land/r/dev/gnotif/v1/x", 0, false},
		{"gno.land/r/dev/gnotifx/v1", 0, false},
		{"gno.land/r/other/gnotif/v1", 0, false},
		{"gno.land/r/dev/gnotif", 0, false},
		{"gno.land/r/dev/gnotif/v99999999999999999999", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := r.Version(tt.path)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIDVersion(t *testing.T) {
	tests := map[string]int{
		"0000001":                 0,
		"v1-0000001":              1,
		"v12-x":                   12,
		"v01-x":                   0,
		"v1":                      0,
		"v-1":                     0,
		"v99999999999999999999-x": 0,
	}
	for id, want := range tests {
		t.Run(id, func(t *testing.T) {
			assert.Equal(t, want, idVersion(id))
		})
	}
}
