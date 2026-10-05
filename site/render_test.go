package main

import (
	"os"
	"path/filepath"
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

func TestRenderPageGettingStarted(t *testing.T) {
	r, err := renderPage(repoRoot, pages[0])
	require.NoError(t, err)
	assert.Equal(t, Heading{Level: 2, ID: "1-emit-an-event", Text: "1. Emit an event"}, r.Headings[0])
	assert.Contains(t, r.Headings, Heading{Level: 3, ID: "templates", Text: "Templates"})
	assert.Contains(t, string(r.Body), `href="/docs/how-it-works/#what-the-verified-mark-means"`)
	assert.Contains(t, string(r.Body), `class="chroma"`)
}

func TestRenderPageHowItWorks(t *testing.T) {
	r, err := renderPage(repoRoot, pages[1])
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
	assert.Equal(t, 1, strings.Count(body, "data-lang="), body)
	assert.Contains(t, body, `data-lang="sh"`)
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
