package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildSite(t *testing.T) string {
	t.Helper()
	out := t.TempDir()
	require.NoError(t, build(repoRoot, out))
	return out
}

func readOut(t *testing.T, out string, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(rel)))
	require.NoError(t, err, rel)
	return string(b)
}

func TestBuildDocs(t *testing.T) {
	out := buildSite(t)

	for i, p := range pages {
		t.Run(p.Slug, func(t *testing.T) {
			html := readOut(t, out, "docs/"+p.Slug+"/index.html")
			assert.Contains(t, html, "<title>"+p.Title)

			last := 0
			for _, q := range pages {
				at := strings.Index(html, ">"+q.Title+"</a>")
				require.Greater(t, at, last, "sidebar lists %q after the one before it", q.Title)
				last = at
			}
			assert.Contains(t, html, `aria-current="page" href="`+p.URL()+`"`)

			r, err := renderPage(repoRoot, p)
			require.NoError(t, err)
			for _, h := range r.Headings {
				assert.Contains(t, html, `href="#`+h.ID+`"`)
			}

			if i > 0 {
				assert.Contains(t, html, `rel="prev" href="`+pages[i-1].URL()+`"`)
			} else {
				assert.NotContains(t, html, `rel="prev"`)
			}
			if i < len(pages)-1 {
				assert.Contains(t, html, `rel="next" href="`+pages[i+1].URL()+`"`)
			} else {
				assert.NotContains(t, html, `rel="next"`)
			}
		})
	}

	redirect := readOut(t, out, "docs/index.html")
	assert.Contains(t, redirect, `<meta http-equiv="refresh" content="0; url=/docs/getting-started/">`)
	assert.Contains(t, redirect, `<a href="/docs/getting-started/">`)

	assert.Equal(t, "gnotif.xyz\n", readOut(t, out, "CNAME"))
	assert.NotEmpty(t, readOut(t, out, "style.css"))
	highlightCSS := readOut(t, out, "highlight.css")
	assert.Contains(t, highlightCSS, ".chroma")
	assert.Contains(t, highlightCSS, "@media (prefers-color-scheme: light)")
}
