package main

import (
	"html"
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

			start := strings.Index(html, `<ul class="sidebar-list">`)
			require.Greater(t, start, -1)
			sidebar := html[start : start+strings.Index(html[start:], "</ul>")]
			last := 0
			for _, q := range pages {
				at := strings.Index(sidebar, ">"+q.Title+"</a>")
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
	assert.Contains(t, readOut(t, out, "docs/getting-started/index.html"), `rel="next" href="/docs/triggers/"`, "the triggers reference follows the getting started")

	assert.NotEmpty(t, readOut(t, out, "style.css"))
	highlightCSS := readOut(t, out, "highlight.css")
	assert.Contains(t, highlightCSS, ".chroma")
	assert.Contains(t, highlightCSS, "@media (prefers-color-scheme: light)")
	assert.Contains(t, highlightCSS, "@media (prefers-color-scheme: dark)")
	assert.Regexp(t, `(?s)@media \(prefers-color-scheme: dark\) \{.*?\.chroma \{`, highlightCSS, "the dark rules sit inside the dark media query")
}

func TestBuildLanding(t *testing.T) {
	out := buildSite(t)
	html := readOut(t, out, "index.html")
	text := stripTags(html)

	assert.Contains(t, html, "Realm event in.")
	assert.Contains(t, html, "Browser notification out.")
	assert.Contains(t, html, `href="/docs/getting-started/" class="button primary">Add it to your dapp`)
	assert.Contains(t, html, `href="https://demo.gnotif.xyz" class="button">Try the demo`)
	assert.Equal(t, 2, strings.Count(html, `<a href="https://demo.gnotif.xyz">Demo</a>`), "the nav and the footer link the demo")
	assert.Contains(t, html, `class="hero-more" href="#how"`)
	assert.Contains(t, html, `<section class="flow-section" id="how">`)
	assert.Contains(t, html, `class="scroll-hint"`)
	assert.Contains(t, html, `<a class="wordmark" href="/" aria-label="gnotif">`)
	assert.Contains(t, html, `<span class="tag">testnet</span>`, "the network tag sits beside the wordmark")
	assert.Contains(t, html, `>HTTP API</a>`)
	assert.Contains(t, html, `>npm install gnotif</code>`)

	assert.Equal(t, 5, strings.Count(html, `<div class="scene`), "five examples on the signal line")
	assert.Equal(t, 1, strings.Count(html, `<div class="scene active"`), "one example shown at rest")
	assert.Contains(t, html, "Game 0000042, turn 7")
	assert.Contains(t, text, "the player's browser")
	assert.Contains(t, text, "every voter's browser")

	assert.Contains(t, text, "there is no notification server to run")
	assert.Contains(t, text, "chain.Emit(")
	assert.Contains(t, text, "gnotif.Declare(")
	assert.Contains(t, text, "func init(cur realm)", "the realm declares its trigger at deploy")
	assert.Contains(t, text, "new Gnotif(")
	assert.Contains(t, text, `gnotif.triggers("gno.land/r/<you>/notify")`, "the page asks for its own realm's triggers")
	assert.Contains(t, html, "https://gnotif.xyz")
	assert.GreaterOrEqual(t, strings.Count(html, `class="chroma"`), 3)
	for _, anchor := range []string{"#1-emit-an-event", "#2-declare-the-trigger", "#3-add-the-client-to-your-page"} {
		assert.Contains(t, html, `href="/docs/getting-started/`+anchor+`"`)
	}
	for _, node := range []string{"dapp realm", "gnotif registry", "tx-indexer", "gnotifd", "push service", "sw.js"} {
		assert.Contains(t, html, node)
	}

	assert.Contains(t, html, "Run your own gnotifd")
	assert.NotContains(t, html, "Integrate")
	assert.NotContains(t, html, `class="status"`)
	assert.NotContains(t, strings.ToLower(text), "onyx", "the landing names no network or endpoint")
	assert.NotContains(t, text, "indexer.")
	assert.NotContains(t, html, "hosted public instance")
}

var tagPattern = regexp.MustCompile(`<[^>]+>`)

// stripTags leaves the text of an HTML fragment, entities decoded, so code
// split into highlighting spans and copy with apostrophes match as they read.
func stripTags(page string) string {
	return html.UnescapeString(tagPattern.ReplaceAllString(page, ""))
}

func TestBuildDocsSidebarListOutsideMenu(t *testing.T) {
	out := buildSite(t)
	html := readOut(t, out, "docs/getting-started/index.html")
	list := strings.Index(html, `<ul class="sidebar-list">`)
	menu := strings.Index(html, `<details class="sidebar-menu">`)
	require.Greater(t, list, -1, "a page list that no <details> can hide")
	require.Greater(t, menu, -1)
	assert.Less(t, list, menu, "the always-visible list comes before the phone menu")
}
