package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
)

//go:embed templates static
var assets embed.FS

// siteView is what every template sees about the site itself.
type siteView struct {
	SiteURL   string
	ServerURL string
	DemoURL   string
	RepoURL   string
	BlobURL   string
	EditURL   string
	NpmURL    string
}

var site = siteView{SiteURL: siteURL, ServerURL: serverURL, DemoURL: demoURL, RepoURL: repoURL, BlobURL: repoBlobURL, EditURL: repoEditURL, NpmURL: npmURL}

// docsView is the data of one documentation page.
type docsView struct {
	Site     siteView
	Page     Page
	Pages    []Page
	Rendered Rendered
	Prev     *Page
	Next     *Page
}

// build renders every page into out, copies the static files, writes the
// highlighting stylesheet, and fails on any dangling internal link. Files
// already in out are overwritten, nothing else there is touched.
func build(root, out string) error {
	docsTmpl, err := template.ParseFS(assets, "templates/layout.html", "templates/docs.html")
	if err != nil {
		return err
	}
	for i, p := range pages {
		r, err := renderPage(root, p)
		if err != nil {
			return err
		}
		v := docsView{Site: site, Page: p, Pages: pages, Rendered: r}
		if i > 0 {
			v.Prev = &pages[i-1]
		}
		if i < len(pages)-1 {
			v.Next = &pages[i+1]
		}
		if err := writeTemplate(docsTmpl, filepath.Join(out, "docs", p.Slug, "index.html"), v); err != nil {
			return err
		}
	}

	landingTmpl, err := template.ParseFS(assets, "templates/layout.html", "templates/landing.html")
	if err != nil {
		return err
	}
	landing, err := newLandingView()
	if err != nil {
		return err
	}
	if err := writeTemplate(landingTmpl, filepath.Join(out, "index.html"), landing); err != nil {
		return err
	}

	redirect := fmt.Sprintf(redirectPage, pages[0].URL(), pages[0].URL(), pages[0].Title)
	if err := writeFile(filepath.Join(out, "docs", "index.html"), []byte(redirect)); err != nil {
		return err
	}
	if err := copyStatic(out); err != nil {
		return err
	}
	if err := writeHighlightCSS(out); err != nil {
		return err
	}
	return checkLinks(out)
}

const redirectPage = `<!doctype html>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0; url=%s">
<title>gnotif docs</title>
<p>The documentation starts at <a href="%s">%s</a>.</p>
`

func writeTemplate(t *template.Template, path string, data any) error {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	return writeFile(path, buf.Bytes())
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// copyStatic places the files under static/ at the root of out.
func copyStatic(out string) error {
	return fs.WalkDir(assets, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := assets.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("static", filepath.FromSlash(path))
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(out, rel), data)
	})
}

// writeHighlightCSS writes chroma's rules for both themes, each inside its
// own media query so a token one style leaves unstyled never inherits the
// other style's color.
func writeHighlightCSS(out string) error {
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	var buf bytes.Buffer
	for scheme, style := range map[string]string{"dark": "github-dark", "light": "github"} {
		fmt.Fprintf(&buf, "@media (prefers-color-scheme: %s) {\n", scheme)
		if err := formatter.WriteCSS(&buf, styles.Get(style)); err != nil {
			return err
		}
		buf.WriteString("}\n")
	}
	return writeFile(filepath.Join(out, "highlight.css"), buf.Bytes())
}
