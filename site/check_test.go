package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSite(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

func TestCheckLinksDangling(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"a/index.html": `<a href="/missing/">x</a> <a href="/b/#nope">y</a> <a href="#gone">z</a> <a href="/b/#here">ok</a>`,
		"b/index.html": `<h2 id="here">Here</h2>`,
	})

	err := checkLinks(dir)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "/missing/")
	assert.Contains(t, msg, "/b/#nope")
	assert.Contains(t, msg, "#gone")
	assert.NotContains(t, msg, "/b/#here")
}

func TestCheckLinksClean(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"a/index.html": `<p>a</p> <a href="#a%c3%a7%c3%a3o">accent</a> <a href="/b/?x=1#here">query</a> <h2 id="ação">Ação</h2>`,
		"b/index.html": `<a href="/a/">a</a> <a href="#here">here</a> <a href="https://example.com/x.md">out</a> <h2 id="here">Here</h2>`,
		"style.css":    `body{}`,
		"index.html":   `<a href="/style.css">css</a> <a href="/b/">b</a>`,
	})

	assert.NoError(t, checkLinks(dir))
}
