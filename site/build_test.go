package main

import (
	"html"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
			assert.NotContains(t, html, `href="`+serverURL, "the server's base URL serves no page, so the docs never link it")

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
	assert.Contains(t, text, "Web Push for gno.land dapps: users who opt in get a browser notification when your realm emits an event, even with the tab closed.")
	assert.NotContains(t, text, "You add a small client and an on-chain trigger.", "How it works says how")
	assert.NotContains(t, html, `class="eyebrow"`, "no label sits above a heading")
	assert.Contains(t, html, `href="/docs/getting-started/" class="button primary">Add it to your dapp`)
	assert.Contains(t, html, `href="https://demo.gnotif.xyz" class="button">Try the demo`)
	assert.Equal(t, 2, strings.Count(html, `<a href="https://demo.gnotif.xyz">Demo</a>`), "the nav and the footer link the demo")
	assert.Contains(t, html, `class="hero-more" href="#how"`)
	assert.Contains(t, html, `<section class="flow-section" id="how" tabindex="-1" aria-labelledby="how-title">`)
	assert.NotContains(t, html, `class="scroll-hint"`)
	assert.Contains(t, html, `<a class="wordmark" href="/" aria-label="gnotif">`)
	assert.Contains(t, html, `<span class="tag">onyx testnet</span>`, "the network tag sits beside the wordmark")
	assert.Contains(t, html, `<a href="/docs/triggers/">Triggers</a>`, "the nav links the concept the page relies on")
	assert.NotContains(t, html, `>HTTP API</a>`, "the HTTP API stays in the docs sidebar")
	assert.Contains(t, html, `<button type="button" class="install" data-copy="npm install gnotif"><span class="visually-hidden">Copy </span><code>npm install gnotif</code></button>`)

	assert.Equal(t, 5, strings.Count(html, `class="scene"`)+strings.Count(html, `class="scene active"`), "five examples on the signal line")
	assert.Equal(t, 1, strings.Count(html, `class="scene active"`), "one example shown at rest")
	assert.Contains(t, html, "Game 0000042, turn 7")
	assert.Contains(t, text, "the player's browser")
	assert.Contains(t, text, "every voter's browser")
	assert.Contains(t, text, "New vesting grant", "every scene's event comes from someone's transaction")
	assert.NotContains(t, text, "UnlockReached", "a realm emits nothing when a block height passes")
	assert.Contains(t, text, "Lot 309, now 1\u202f250\u00a0GNOT", "an amount never breaks inside its digits or from its unit")
	assert.Contains(t, text, "next=g1k7\u2060…\u2060x2q", "a short address never breaks at its ellipsis")
	assert.NotContains(t, strings.ReplaceAll(text, "\u2060…\u2060", ""), "…", "every short address keeps whole")
	assert.Contains(t, text, "10\u202f000\u00a0GNOT, unlocking at block 1\u202f300\u202f000")

	facts := []string{
		"No server to run: the public server sends the notifications, on the onyx testnet, version 0.",
		"Works in browsers with Web Push, such as Chrome. On iPhone and iPad, only for sites added to the Home Screen.",
	}
	for _, fact := range facts {
		assert.Contains(t, text, fact)
	}
	factList := html[strings.Index(html, `<ul class="facts">`):]
	assert.Equal(t, len(facts), strings.Count(factList[:strings.Index(factList, "</ul>")], "<li>"), "two facts, no more")
	description := strings.Index(html, `class="hero-description"`)
	buttons := strings.Index(html, `class="button">Try the demo`)
	more := strings.Index(html, `class="hero-more"`)
	list := strings.Index(html, `<ul class="facts">`)
	signal := strings.Index(html, `<div class="signal"`)
	assert.Less(t, strings.Index(html, "<h1>"), description)
	assert.Less(t, description, buttons)
	assert.Less(t, buttons, more, "the How it works link sits in the buttons' row")
	assert.Less(t, more, list, "the facts sit under the buttons' row")
	assert.Less(t, list, signal, "the buttons and facts come before the signal line, so a phone's first screen shows them")
	assert.Less(t, signal, strings.Index(html, `id="how"`), "the signal line stays in the hero")
	assert.Contains(t, text, "The notification travels through the browser’s push service to the service worker on your dapp’s origin.")
	assert.Equal(t, 1, strings.Count(text, "No server to run"), "the delivery path no longer repeats the no-server fact")
	assert.Contains(t, text, "chain.Emit(")
	assert.Contains(t, text, "func init(cur realm)", "the realm sets its trigger at deploy")
	assert.Contains(t, text, `import "gno.land/r/nym-gfanton001/gnotif/v1"`)
	assert.Contains(t, text, "me := cur.Sub(gnotif.Subpath)", "the registry takes triggers only from the realm's gnotif identity")
	assert.Contains(t, text, "h := gnotif.Open(cross(me))")
	assert.Contains(t, text, `h.Set(cross(me), "message", gnotif.Trigger{`)
	assert.Contains(t, text, "opens its handle once, in init")
	assert.Contains(t, text, "new Gnotif(")
	assert.Contains(t, text, `gnotif.triggers("gno.land/r/<you>/notify")`, "the page asks for its own realm's triggers")
	assert.Contains(t, text, "// <you>: the address you deployed the realm from\nconst [message] = await gnotif.triggers(", "the sample names what <you> stands for")
	assert.Contains(t, html, `<span class="wordmark small" role="img" aria-label="gnotif">`, "the footer wordmark reads as one word")
	assert.Contains(t, text, `new Gnotif({ network: "onyx" })`, "the snippet names its network")
	assert.NotContains(t, text, serverURL, "the facts name the public server, not its URL, which serves no page")
	assert.GreaterOrEqual(t, strings.Count(html, `class="chroma"`), 4)
	for _, anchor := range []string{"#1-emit-an-event", "#2-set-the-trigger", "#3-add-the-client-to-your-page"} {
		assert.Contains(t, html, `href="/docs/getting-started/`+anchor+`"`)
	}
	for _, node := range []string{"dapp realm", "gnotif registry", "tx-indexer", "gnotifd", "push service", "sw.js"} {
		assert.Contains(t, html, node)
	}

	assert.Equal(t, 2, strings.Count(html, `href="/docs/getting-started/" class="button primary">Add it to your dapp`), "the page ends on the hero's buttons")
	assert.Equal(t, 2, strings.Count(html, `href="https://demo.gnotif.xyz" class="button">Try the demo`), "the page ends on the hero's buttons")
	selfHost := strings.Index(text, "Want your own server? gnotifd is one binary with one SQLite file.")
	if assert.NotEqual(t, -1, selfHost, "one line points to self-hosting") {
		assert.Less(t, strings.LastIndex(text, "Try the demo"), selfHost, "self-hosting follows the closing buttons")
	}
	assert.Contains(t, html, `<a href="/docs/running-gnotifd/">Running gnotifd →</a>`)
	assert.NotContains(t, html, "Integrate")
	assert.NotContains(t, html, `class="status"`)
	rest := strings.NewReplacer(`network: "onyx"`, "", serverURL, "", "onyx testnet", "").Replace(text)
	assert.NotContains(t, strings.ToLower(rest), "onyx", "only the snippet's network, the server URL and the testnet name onyx")
	assert.NotContains(t, text, "indexer.")
	assert.NotContains(t, html, "hosted public instance")
}

func TestBuildLandingExamples(t *testing.T) {
	out := buildSite(t)
	html := readOut(t, out, "index.html")

	assert.Contains(t, html, `<div class="signal" role="group" aria-label="A realm event becoming a browser notification">`)
	assert.Contains(t, html, `<div class="scene-bar" hidden>`, "the example controls show once the script that runs them does")
	assert.Contains(t, html, `<div class="scene-tabs" role="tablist" aria-label="Notification examples">`)
	assert.Equal(t, len(scenes), strings.Count(html, `role="tab"`))
	assert.Equal(t, len(scenes), strings.Count(html, `role="tabpanel"`))
	assert.Equal(t, 1, strings.Count(html, `aria-selected="true"`), "one example selected at rest")
	for i, s := range scenes {
		n := strconv.Itoa(i)
		assert.Contains(t, html, `id="scene-tab-`+n+`" aria-controls="scene-`+n+`"`)
		assert.Contains(t, html, `id="scene-`+n+`" aria-labelledby="scene-tab-`+n+`"`)
		assert.Contains(t, html, ">"+s.Event+"</button>")
	}
	signal := html[strings.Index(html, `<div class="signal"`):strings.Index(html, `<div class="scene-bar"`)]
	assert.Equal(t, len(scenes), strings.Count(signal, `class="indicator"`), "the wire's dot is the only light on the signal line")
	for _, s := range scenes {
		assert.Contains(t, signal, `<span class="toast-icon" aria-hidden="true">`+s.App[:1]+`</span>`, "the card shows the dapp's icon, its initial")
	}
	assert.Contains(t, html, `aria-label="Pause automatic examples">Pause</button>`)
	assert.NotContains(t, html, "⏸")
	assert.NotContains(t, html, "▷")
}

func TestBuildLandingPath(t *testing.T) {
	out := buildSite(t)
	html := readOut(t, out, "index.html")
	start := strings.Index(html, `id="how"`)
	end := strings.Index(html, `<section class="closing">`)
	require.Less(t, start, end)
	how := html[start:end]
	text := stripTags(how)

	assert.NotContains(t, html, "Add notifications to a dapp", "the steps and How it works are one section")
	assert.NotContains(t, text, "01 / Emit")
	last := -1
	for i, title := range []string{"Emit an event from your realm", "Set a trigger in the registry", "Subscribe the browser from your page"} {
		at := strings.Index(how, "<h3>"+title+"</h3>")
		require.Greater(t, at, last, "station %d", i+1)
		last = at
	}
	for _, s := range []string{
		"A realm with one function, Notify, emits a message and the address it is for.",
		"The realm opens its handle once, in init, and sets its trigger: which event to watch, who hears it, and what the notification says.",
		"Install the client and serve its sw.js at your site's root. From a click, call enable(), then opt the browser in with the user's address. Call connect() too, and an open page gets each one live.",
		"// button: a button on your page; address: the user's gno.land address",
		"npm install gnotif\ncp node_modules/gnotif/dist/sw.js public/sw.js",
		"Dashed: yours, in your dapp. Solid: run by gnotif, the chain and the browser’s vendor.",
	} {
		assert.Contains(t, text, s)
	}
	assert.NotContains(t, how, `class="path-event"`, "the hero already shows the event")
	assert.NotContains(t, how, `class="path-toast"`, "the hero already shows the notification")
	assert.NotContains(t, text, "Your realm emits an event. gnotif finds the matching trigger", "no intro line repeats the hero")
	for i, anchor := range []string{"#1-emit-an-event", "#2-set-the-trigger", "#3-add-the-client-to-your-page"} {
		assert.Contains(t, how, `<a href="/docs/getting-started/`+anchor+`">Step `+strconv.Itoa(i+1)+` in Getting started →</a>`)
	}
	assert.Equal(t, 3, strings.Count(how, `<span class="station-dot" aria-hidden="true">`))
	assert.Equal(t, 1, strings.Count(how, `<span class="station-dot" aria-hidden="true"><i class="indicator"></i></span>`), "only the last station, the browser, lights the dot")
}

func TestBuildCodeBlocks(t *testing.T) {
	out := buildSite(t)
	for _, rel := range []string{"index.html", "docs/getting-started/index.html", "docs/triggers/index.html", "docs/how-it-works/index.html"} {
		page := readOut(t, out, rel)
		blocks := strings.Count(page, `<div class="code">`)
		require.Positive(t, blocks, rel)
		assert.Equal(t, blocks, strings.Count(page, `<button type="button" class="copy" hidden>Copy</button>`), "%s: every code block has a copy button, which the script reveals", rel)
		assert.NotContains(t, page, `<span>go</span>`, "%s: realm code reads gno", rel)
		assert.Contains(t, page, `<p class="visually-hidden" role="status" id="copy-status"></p>`, rel)
	}
	landing := readOut(t, out, "index.html")
	assert.Equal(t, 2, strings.Count(landing, `<div class="code-head"><span>gno</span>`))
	assert.Equal(t, 1, strings.Count(landing, `<div class="code-head"><span>sh</span>`))
	assert.Equal(t, 1, strings.Count(landing, `<div class="code-head"><span>js</span>`))
	started := readOut(t, out, "docs/getting-started/index.html")
	assert.Equal(t, 2, strings.Count(started, `<div class="code-head"><span>gno</span>`), "the two realm blocks")
	assert.Contains(t, started, `<span class="kd">func</span>`, "realm code is highlighted as Go, whose syntax it shares")
}

func TestBuildFonts(t *testing.T) {
	out := buildSite(t)
	page := readOut(t, out, "index.html")
	assert.NotContains(t, page, "fonts.googleapis.com", "the site serves its own fonts")
	assert.NotContains(t, page, "fonts.gstatic.com")
	assert.Contains(t, page, `<link rel="preload" href="/fonts/ibm-plex-mono-500-latin.woff2" as="font" type="font/woff2" crossorigin>`, "the headline's face loads first")

	css := readOut(t, out, "style.css")
	faces := regexp.MustCompile(`@font-face \{[^}]*\}`).FindAllString(css, -1)
	require.Len(t, faces, 12, "two families, three weights, latin and latin-ext")
	src := regexp.MustCompile(`url\("(/fonts/[^"]+\.woff2)"\)`)
	for _, face := range faces {
		assert.Contains(t, face, "font-display: swap;", face)
		assert.Contains(t, face, "unicode-range:", face)
		m := src.FindStringSubmatch(face)
		if assert.Len(t, m, 2, face) {
			assert.FileExists(t, filepath.Join(out, filepath.FromSlash(m[1])))
		}
	}
	for _, family := range []string{"IBM Plex Mono", "IBM Plex Sans"} {
		for _, weight := range []string{"400", "500", "600"} {
			n := 0
			for _, face := range faces {
				if strings.Contains(face, `font-family: "`+family+`";`) && strings.Contains(face, "font-weight: "+weight+";") {
					n++
				}
			}
			assert.Equal(t, 2, n, "%s %s: a latin and a latin-ext face", family, weight)
		}
	}
	assert.FileExists(t, filepath.Join(out, "fonts", "OFL.txt"), "the fonts ship with their license")
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

func TestBuildLinkPreview(t *testing.T) {
	out := buildSite(t)
	for rel, url := range map[string]string{
		"index.html":                      "https://gnotif.xyz/",
		"docs/getting-started/index.html": "https://gnotif.xyz/docs/getting-started/",
	} {
		page := readOut(t, out, rel)
		assert.Contains(t, page, `<meta property="og:url" content="`+url+`">`, rel)
		assert.Contains(t, page, `<meta property="og:image" content="https://gnotif.xyz/og.png">`, "%s: crawlers need an absolute image URL", rel)
		assert.Contains(t, page, `<meta name="twitter:card" content="summary_large_image">`, rel)
		assert.Regexp(t, `<meta property="og:title" content="[^"]+ · gnotif|gnotif · `, page, rel)
		assert.Contains(t, page, `<meta property="og:description" content="`, rel)
	}

	f, err := os.Open(filepath.Join(out, "og.png"))
	require.NoError(t, err)
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	require.NoError(t, err, "og.png is a PNG: most platforms ignore SVG previews")
	assert.Equal(t, [2]int{1200, 630}, [2]int{cfg.Width, cfg.Height})
}
