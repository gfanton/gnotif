package main

import (
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// rewriteHref maps a markdown link written in source (a repo-relative path)
// to its site destination: a listed page becomes its URL, any other
// repository file becomes its GitHub URL, fragments and absolute URLs stay.
func rewriteHref(source, href string, pages []Page) string {
	if strings.HasPrefix(href, "#") || strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
		return href
	}
	file, fragment, hasFragment := strings.Cut(href, "#")
	if hasFragment {
		fragment = "#" + fragment
	}
	target := path.Clean(path.Join(path.Dir(source), file))
	for _, p := range pages {
		if p.Source == target {
			return p.URL() + fragment
		}
	}
	return repoBlobURL + target + fragment
}

// linkRewriter applies rewriteHref to every link in a parsed document.
type linkRewriter struct {
	source string
	pages  []Page
}

func (r *linkRewriter) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if link, ok := n.(*ast.Link); ok && entering {
			link.Destination = []byte(rewriteHref(r.source, string(link.Destination), r.pages))
		}
		return ast.WalkContinue, nil
	})
}
