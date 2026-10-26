package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoRoot = ".."

func TestRenderPage(t *testing.T) {
	for _, p := range pages {
		t.Run(p.Slug, func(t *testing.T) {
			r, err := renderPage(repoRoot, p)
			require.NoError(t, err)
			assert.NotEmpty(t, r.Body)
			assert.NotContains(t, string(r.Body), "<h1")
			require.NotEmpty(t, r.Headings)
			for _, h := range r.Headings {
				assert.Contains(t, []int{2, 3}, h.Level, h.Text)
				assert.NotEmpty(t, h.ID, h.Text)
			}
		})
	}
}

// The site shows a page's Title in place of the source's first heading, so a
// page whose two differ reads one title on the site and another on GitHub.
func TestPageTitleMatchesSourceHeading(t *testing.T) {
	for _, p := range pages {
		// js/README.md is headed by the npm package's name.
		if p.Source == "js/README.md" {
			continue
		}
		t.Run(p.Slug, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(p.Source)))
			require.NoError(t, err)
			first, _, _ := strings.Cut(string(src), "\n")
			assert.Equal(t, "# "+p.Title, first)
		})
	}
}

// pageBySlug finds a listed page without depending on its place in the list.
func pageBySlug(t *testing.T, slug string) Page {
	t.Helper()
	i := slices.IndexFunc(pages, func(p Page) bool { return p.Slug == slug })
	require.GreaterOrEqual(t, i, 0, "no page %q", slug)
	return pages[i]
}

func TestRenderPageGettingStarted(t *testing.T) {
	r, err := renderPage(repoRoot, pageBySlug(t, "getting-started"))
	require.NoError(t, err)
	assert.Equal(t, Heading{Level: 2, ID: "1-emit-an-event", Text: "1. Emit an event"}, r.Headings[0])
	assert.Contains(t, string(r.Body), `href="/docs/triggers/#a-triggers-id-can-change"`)
	assert.Contains(t, string(r.Body), `class="chroma"`)
}

func TestRenderPageTriggers(t *testing.T) {
	r, err := renderPage(repoRoot, pageBySlug(t, "triggers"))
	require.NoError(t, err)
	assert.Contains(t, r.Headings, Heading{Level: 2, ID: "a-triggers-id-can-change", Text: "A trigger's id can change"})
	assert.Contains(t, string(r.Body), `href="/docs/how-it-works/#what-the-verified-mark-means"`)
}

func TestRenderPageHowItWorks(t *testing.T) {
	r, err := renderPage(repoRoot, pageBySlug(t, "how-it-works"))
	require.NoError(t, err)
	assert.Contains(t, string(r.Body), `href="/docs/http-api/#replace-a-rotated-subscription"`)
}

func TestRenderPagePlainFence(t *testing.T) {
	root := t.TempDir()
	src := "# X\n\n```\nplain text\n```\n\n```sh\necho hi\n```\n"
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "x.md"), []byte(src), 0o644))

	r, err := renderPage(root, Page{Title: "X", Source: "docs/x.md", Slug: "x"})
	require.NoError(t, err)
	body := string(r.Body)
	assert.Contains(t, body, "<pre")
	assert.Contains(t, body, "plain text")
	assert.Equal(t, 2, strings.Count(body, `<div class="code">`), body)
	assert.Contains(t, body, `<div class="code"><div class="code-head"><button type="button" class="copy" hidden>Copy</button></div><pre><code>plain text`, "a block without a language still gets its copy button")
	assert.Contains(t, body, `<div class="code-head"><span>sh</span><button type="button" class="copy" hidden>Copy</button></div>`)
}

func TestRenderPageGnoFence(t *testing.T) {
	root := t.TempDir()
	src := "# X\n\n```gno\nfunc init(cur realm) {}\n```\n"
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "x.md"), []byte(src), 0o644))

	r, err := renderPage(root, Page{Title: "X", Source: "docs/x.md", Slug: "x"})
	require.NoError(t, err)
	body := string(r.Body)
	assert.Contains(t, body, `<div class="code-head"><span>gno</span>`)
	assert.Contains(t, body, `<span class="kd">func</span>`, "Gno is highlighted with Go's lexer")
}

func TestRenderPageAlert(t *testing.T) {
	for name, tc := range map[string]struct {
		src, class, title, text string
	}{
		"note":                  {"> [!NOTE]\n> Read the chain again.\n", "callout callout-note", "Note", "<p>Read the chain again.</p>"},
		"warning":               {"> [!WARNING]\n> Anyone can call it.\n", "callout callout-warning", "Warning", "<p>Anyone can call it.</p>"},
		"marker in a paragraph": {"> [!TIP]\n>\n> Allow notifications.\n", "callout callout-tip", "Tip", "<p>Allow notifications.</p>"},
		"lowercase marker":      {"> [!note]\n> Read the chain again.\n", "callout callout-note", "Note", "<p>Read the chain again.</p>"},
		"lazy line":             {"> [!NOTE]\nlazy text\n", "callout callout-note", "Note", "<p>lazy text</p>"},
		"markup after marker":   {"> [!NOTE]\n> **Bold** x\n", "callout callout-note", "Note", "<p><strong>Bold</strong> x</p>"},
	} {
		t.Run(name, func(t *testing.T) {
			body := renderSource(t, "# X\n\n"+tc.src)
			assert.Contains(t, body, `<blockquote class="`+tc.class+`" role="note">`)
			assert.Contains(t, body, `<p class="callout-title">`+tc.title+`</p>`)
			assert.Contains(t, body, tc.text, "the marker line leaves the text")
			assert.NotContains(t, body, "[!")
		})
	}
}

func TestRenderPagePlainQuote(t *testing.T) {
	for name, src := range map[string]string{
		"quote":                       "> A plain quote.\n",
		"unknown marker":              "> [!FOO]\n> Text.\n",
		"marker and text on one line": "> [!NOTE] Text.\n",
		"marker alone":                "> [!NOTE]\n",
		"in a list item":              "- > [!TIP]\n  > Text.\n",
		"in a quote":                  "> > [!NOTE]\n> > Text.\n",
	} {
		t.Run(name, func(t *testing.T) {
			body := renderSource(t, "# X\n\n"+src)
			assert.Contains(t, body, "<blockquote>\n", "GitHub renders this as a quote, so the site does too")
			assert.NotContains(t, body, "callout")
		})
	}
}

// A phone shows a table row as a block that names each cell by its column.
func TestRenderPageTableLabels(t *testing.T) {
	body := renderSource(t, "# X\n\n| Flag | Default | Meaning |\n|---|---|---|\n| `-poll` | `5s` | how often |\n| `-db` |\n| a | b | c | d |\n")
	assert.Contains(t, body, `<td data-label="Flag"><code>-poll</code></td>`)
	assert.Contains(t, body, `<td data-label="Default"><code>5s</code></td>`)
	assert.Contains(t, body, `<td data-label="Meaning">how often</td>`)
	assert.Contains(t, body, `<td data-label="Flag"><code>-db</code></td>`, "a short row is labelled too")
	assert.Contains(t, body, `<td data-label="Meaning">c</td>`)
	assert.NotContains(t, body, ">d<", "a row longer than the header loses its extra cells")
	assert.Contains(t, body, `<th>Default</th>`, "a header cell carries no label")
}

// The site draws a diagram inline, so it takes the page's fonts and colors;
// GitHub shows the same file as an image.
func TestRenderPageDiagram(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "x.md"), []byte("# X\n\n![The path](x.svg)\n\nSee ![it](x.svg) here.\n\n![remote](https://example.com/x.svg)\n\n- ![listed](x.svg)\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "x.svg"), []byte(`<svg role="img"><title>The path</title></svg>`+"\n"), 0o644))

	r, err := renderPage(root, Page{Title: "X", Source: "docs/x.md", Slug: "x"})
	require.NoError(t, err)
	body := string(r.Body)
	assert.Contains(t, body, `<figure class="diagram"><svg role="img"><title>The path</title></svg>`+"\n</figure>")
	assert.NotContains(t, body, "<p><figure")
	assert.Contains(t, body, `<img src="x.svg" alt="it">`, "an image inside a sentence stays an image")
	assert.Contains(t, body, `<img src="https://example.com/x.svg" alt="remote">`, "a remote image stays an image")
	assert.Contains(t, body, `<img src="x.svg" alt="listed">`, "an image in a list stays an image")
	assert.Equal(t, 1, strings.Count(body, "<svg"))
}

func TestRenderPageDiagramMissing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "x.md"), []byte("# X\n\n![The path](gone.svg)\n"), 0o644))

	_, err := renderPage(root, Page{Title: "X", Source: "docs/x.md", Slug: "x"})
	require.ErrorContains(t, err, "gone.svg")
}

// renderSource renders src as the one page of a scratch repository.
func renderSource(t *testing.T, src string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "x.md"), []byte(src), 0o644))
	r, err := renderPage(root, Page{Title: "X", Source: "docs/x.md", Slug: "x"})
	require.NoError(t, err)
	return string(r.Body)
}

func TestRenderPageNoH1(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "y.md"), []byte("## Only a section\n\nText.\n"), 0o644))

	r, err := renderPage(root, Page{Title: "Y", Source: "docs/y.md", Slug: "y"})
	require.NoError(t, err)
	require.NotEmpty(t, r.Headings)
	assert.Equal(t, "Only a section", r.Headings[0].Text)
}

func TestRenderPageHeadingWithLink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "z.md"), []byte("# Z\n\n## See [the API](http-api.md)\n"), 0o644))

	r, err := renderPage(root, Page{Title: "Z", Source: "docs/z.md", Slug: "z"})
	require.NoError(t, err)
	require.NotEmpty(t, r.Headings)
	assert.Equal(t, Heading{Level: 2, ID: "see-the-api", Text: "See the API"}, r.Headings[0])
	assert.Contains(t, string(r.Body), `<h2 id="see-the-api">`)
}

func TestHighlight(t *testing.T) {
	out, err := highlight("js", "const x = 1;")
	require.NoError(t, err)
	assert.Contains(t, string(out), `class="chroma"`)
	assert.Contains(t, string(out), `<span class="k`)

	assert.Contains(t, string(out), `<div class="code-head"><span>js</span><button type="button" class="copy" hidden>Copy</button></div>`)

	out, err = highlight("gno", "func F() {}")
	require.NoError(t, err)
	assert.Contains(t, string(out), `<div class="code-head"><span>gno</span>`)
	assert.Contains(t, string(out), `<span class="kd">func</span>`)

	out, err = highlight("nosuchlang", "x")
	require.NoError(t, err)
	assert.Contains(t, string(out), "x")
}

func TestRenderPageKeepsAllContent(t *testing.T) {
	for _, p := range pages {
		t.Run(p.Slug, func(t *testing.T) {
			r, err := renderPage(repoRoot, p)
			require.NoError(t, err)
			assert.NotContains(t, string(r.Body), "raw HTML omitted")
		})
	}
}
