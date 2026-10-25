package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Heading is one entry of a page's "On this page" list.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// Rendered is a documentation page's HTML body, without its first h1, and
// the h2 and h3 headings it contains.
type Rendered struct {
	Body     template.HTML
	Headings []Heading
}

// renderPage converts the markdown source of p, found under root, to HTML.
func renderPage(root string, p Page) (Rendered, error) {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.Source)))
	if err != nil {
		return Rendered{}, fmt.Errorf("render %s: %w", p.Source, err)
	}
	md := newMarkdown(p.Source)
	doc := md.Parser().Parse(text.NewReader(src))

	// IDs come from the rendered heading text, as GitHub derives them.
	ids := newGithubIDs()
	var headings []Heading
	titleDropped := false
	for n := doc.FirstChild(); n != nil; {
		next := n.NextSibling()
		if h, ok := n.(*ast.Heading); ok {
			text := nodeText(h, src)
			id := string(ids.Generate([]byte(text), ast.KindHeading))
			h.SetAttributeString("id", []byte(id))
			switch {
			case h.Level == 1 && !titleDropped:
				doc.RemoveChild(doc, h)
				titleDropped = true
			case h.Level == 2 || h.Level == 3:
				headings = append(headings, Heading{Level: h.Level, ID: id, Text: text})
			}
		}
		n = next
	}

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return Rendered{}, fmt.Errorf("render %s: %w", p.Source, err)
	}
	return Rendered{Body: template.HTML(buf.String()), Headings: headings}, nil
}

func newMarkdown(source string) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
				highlighting.WithWrapperRenderer(wrapCodeBlock),
			),
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(&linkRewriter{source: source, pages: pages}, 100)),
		),
	)
}

// gnoLexer highlights Gno, whose syntax is Go's, with chroma's Go lexer.
type gnoLexer struct{ chroma.Lexer }

func (gnoLexer) Config() *chroma.Config {
	return &chroma.Config{Name: "Gno", Aliases: []string{"gno"}}
}

// goldmark-highlighting looks fences up in chroma's global registry only.
func init() {
	lexers.Register(gnoLexer{lexers.Get("go")})
}

// codeHead is the bar above a code block: its language, when it has one, and
// a copy button that stays hidden until the page's script can copy.
func codeHead(lang string) string {
	var b strings.Builder
	b.WriteString(`<div class="code-head">`)
	if lang != "" {
		b.WriteString("<span>" + template.HTMLEscapeString(lang) + "</span>")
	}
	b.WriteString(`<button type="button" class="copy" hidden>Copy</button></div>`)
	return b.String()
}

// wrapCodeBlock puts every fenced block in a div headed by the fence's
// language, and supplies the pre/code pair chroma does not write for a
// block it did not highlight.
func wrapCodeBlock(w util.BufWriter, c highlighting.CodeBlockContext, entering bool) {
	if entering {
		lang, _ := c.Language()
		_, _ = w.WriteString(`<div class="code">` + codeHead(string(lang)))
		if !c.Highlighted() {
			_, _ = w.WriteString("<pre><code>")
		}
		return
	}
	if !c.Highlighted() {
		_, _ = w.WriteString("</code></pre>")
	}
	_, _ = w.WriteString("</div>\n")
}

// highlight renders one code snippet the way a fenced block in the docs is
// rendered, for code the landing page carries itself.
func highlight(lang, code string) (template.HTML, error) {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return "", fmt.Errorf("highlight %s: %w", lang, err)
	}
	var buf bytes.Buffer
	buf.WriteString(`<div class="code">` + codeHead(lang))
	if err := chromahtml.New(chromahtml.WithClasses(true)).Format(&buf, styles.Fallback, iterator); err != nil {
		return "", fmt.Errorf("highlight %s: %w", lang, err)
	}
	buf.WriteString("</div>\n")
	return template.HTML(buf.String()), nil
}

// nodeText concatenates the text segments under n.
func nodeText(n ast.Node, src []byte) string {
	var buf bytes.Buffer
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			buf.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return buf.String()
}
