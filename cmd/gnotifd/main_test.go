package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/subscribe"
	"github.com/gfanton/gnotif/internal/watch"
)

var requiredArgs = []string{
	"-indexer", "https://indexer.example/graphql/query",
	"-registry", "gno.land/r/dev/gnotif/v0",
	"-vapid-subject", "ops@gnotif.example",
}

func env(drop string) func(string) string {
	vars := map[string]string{
		"GNOTIF_VAPID_PUBLIC_KEY":  "pub",
		"GNOTIF_VAPID_PRIVATE_KEY": "priv",
	}
	delete(vars, drop)
	return func(k string) string { return vars[k] }
}

func without(flag string) []string {
	var out []string
	for i := 0; i < len(requiredArgs); i += 2 {
		if requiredArgs[i] != flag {
			out = append(out, requiredArgs[i], requiredArgs[i+1])
		}
	}
	return out
}

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig(requiredArgs, env(""))
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8080", cfg.Listen)
	assert.Equal(t, "gnotif.db", cfg.DB)
	assert.Equal(t, 5*time.Second, cfg.Poll)
	assert.Equal(t, 10*time.Minute, cfg.MaxAge)
	assert.Equal(t, subscribe.DefaultPushHosts, cfg.PushHosts)
	assert.Equal(t, "pub", cfg.VAPID.Public)
	assert.Equal(t, "priv", cfg.VAPID.Private)

	missing := map[string]struct {
		args []string
		env  func(string) string
	}{
		"-indexer":                 {without("-indexer"), env("")},
		"-registry":                {without("-registry"), env("")},
		"-vapid-subject":           {without("-vapid-subject"), env("")},
		"GNOTIF_VAPID_PUBLIC_KEY":  {requiredArgs, env("GNOTIF_VAPID_PUBLIC_KEY")},
		"GNOTIF_VAPID_PRIVATE_KEY": {requiredArgs, env("GNOTIF_VAPID_PRIVATE_KEY")},
	}
	for name, tc := range missing {
		t.Run("missing "+name, func(t *testing.T) {
			_, err := parseConfig(tc.args, tc.env)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}

	invalid := map[string]struct {
		args []string
		want string
	}{
		"zero poll":        {[]string{"-poll", "0"}, "-poll must be positive"},
		"negative max-age": {[]string{"-max-age", "-1s"}, "-max-age must not be negative"},
	}
	for name, tc := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parseConfig(append(tc.args, requiredArgs...), env(""))
			assert.EqualError(t, err, tc.want)
		})
	}

	t.Run("mailto subject", func(t *testing.T) {
		args := append([]string{"-vapid-subject", "mailto:ops@gnotif.example"}, without("-vapid-subject")...)
		cfg, err := parseConfig(args, env(""))
		require.NoError(t, err)
		assert.Equal(t, "ops@gnotif.example", cfg.VAPIDSubject)
	})

	t.Run("push hosts", func(t *testing.T) {
		cfg, err := parseConfig(append([]string{"-push-hosts", "a.example,b.example"}, requiredArgs...), env(""))
		require.NoError(t, err)
		assert.Equal(t, []string{"a.example", "b.example"}, cfg.PushHosts)
	})

	t.Run("invalid registry", func(t *testing.T) {
		for _, path := range []string{
			"gno.land/r/dev/gnotif",
			"gno.land/r/dev/gnotif/v01",
			"gno.land/p/dev/gnotif/v0",
		} {
			t.Run(path, func(t *testing.T) {
				_, err := parseConfig(append(without("-registry"), "-registry", path), env(""))
				require.ErrorIs(t, err, watch.ErrInvalidRegistry)
				assert.ErrorContains(t, err, "-registry")
				assert.ErrorContains(t, err, "/v<N>")
			})
		}
	})
}

func TestKeygen(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"keygen"}, env(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.Len(t, lines, 2)
	sizes := map[string]int{"GNOTIF_VAPID_PUBLIC_KEY": 65, "GNOTIF_VAPID_PRIVATE_KEY": 32}
	for _, line := range lines {
		name, value, ok := strings.Cut(line, "=")
		require.True(t, ok, line)
		size, known := sizes[name]
		require.True(t, known, line)
		raw, err := base64.RawURLEncoding.DecodeString(value)
		require.NoError(t, err, line)
		assert.Len(t, raw, size, name)
		delete(sizes, name)
	}
	assert.Empty(t, sizes)
}

func TestUsageErrorExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	assert.Equal(t, 2, run(nil, env(""), &stdout, &stderr))
	assert.Contains(t, stderr.String(), "-indexer")
}

func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"-h"}, env(""), &stdout, &stderr))
	for _, want := range []string{"-listen", "-indexer", "-registry", "-start-height", "-db", "-poll", "-max-age", "-vapid-subject", "-push-hosts", "keygen"} {
		assert.Contains(t, stderr.String(), want)
	}
}
