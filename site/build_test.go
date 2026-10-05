package main

import (
	"os"
	"path/filepath"
	"regexp"
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

func TestBuildLanding(t *testing.T) {
	out := buildSite(t)
	html := readOut(t, out, "index.html")
	text := stripTags(html)

	assert.Contains(t, html, "Realm event in.")
	assert.Contains(t, html, "Browser notification out.")
	assert.Contains(t, html, `href="/docs/getting-started/" class="button primary">Add it to your dapp`)
	assert.Contains(t, html, `href="/docs/running-gnotifd/" class="button">Run your own server`)
	assert.Contains(t, html, "https://gnotif.xyz")

	assert.Contains(t, text, "chain.Emit(")
	assert.Contains(t, text, "gnotif.Declare(")
	assert.Contains(t, text, "new Gnotif(")
	assert.GreaterOrEqual(t, strings.Count(html, `class="chroma"`), 3)
	for _, anchor := range []string{"#1-emit-an-event", "#2-declare-a-trigger", "#3-add-the-client-to-the-page"} {
		assert.Contains(t, html, `href="/docs/getting-started/`+anchor+`"`)
	}

	for _, node := range []string{"dapp realm", "gnotif registry", "tx-indexer", "gnotifd", "push service", "sw.js"} {
		assert.Contains(t, html, node)
	}
	assert.Contains(t, html, "Integrate")
	assert.Contains(t, html, "Operate")
	assert.Contains(t, html, "mainnet")
	assert.NotContains(t, html, "hosted public instance")
	assert.Contains(t, html, "Your turn")
	assert.Contains(t, html, "Game 42, turn 7")
}

var tagPattern = regexp.MustCompile(`<[^>]+>`)

// stripTags leaves the text of an HTML fragment, so code split into
// highlighting spans can be matched as it reads.
func stripTags(html string) string {
	return tagPattern.ReplaceAllString(html, "")
}
