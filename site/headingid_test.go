package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

func TestSlug(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"numbered heading":     {"4. Deploy the registry", "4-deploy-the-registry"},
		"code heading":         {"triggers()", "triggers"},
		"apostrophe":           {"Triggers come from the registry's events", "triggers-come-from-the-registrys-events"},
		"hyphen kept":          {"Set the opt-ins", "set-the-opt-ins"},
		"one hyphen per space": {"  HTTP  API ", "http--api"},
		"ampersand dropped":    {"Fields & templates", "fields--templates"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, slug(tc.in))
		})
	}
}

func TestGithubIDsRepeat(t *testing.T) {
	ids := newGithubIDs()
	require.Equal(t, "fields", string(ids.Generate([]byte("Fields"), ast.KindHeading)))
	require.Equal(t, "fields-1", string(ids.Generate([]byte("Fields"), ast.KindHeading)))
	require.Equal(t, "fields-2", string(ids.Generate([]byte("Fields"), ast.KindHeading)))

	ids.Put([]byte("templates"))
	assert.Equal(t, "templates-1", string(ids.Generate([]byte("Templates"), ast.KindHeading)))
}
