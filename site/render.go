package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
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
	if err := inlineDiagrams(doc, root, p.Source); err != nil {
		return Rendered{}, fmt.Errorf("render %s: %w", p.Source, err)
	}

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
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(diagramRenderer{}, 100)),
		),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(&linkRewriter{source: source, pages: pages}, 100),
				util.Prioritized(alertRewriter{}, 100),
				util.Prioritized(cellLabeler{}, 100),
			),
		),
	)
}

// alertMarker matches the line that opens a GitHub alert, such as [!NOTE] or
// [!note], alone on the first line of a blockquote.
var alertMarker = regexp.MustCompile(`(?i)^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)

// alertRewriter renders a GitHub alert as a callout titled by its kind, so a
// page reads the same on the site as on GitHub. Any other blockquote stays a
// quote.
type alertRewriter struct{}

func (alertRewriter) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	// GitHub renders an alert only at the top level of a document, never in a
	// list item or another quote.
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if quote, ok := n.(*ast.Blockquote); ok {
			toCallout(quote, reader.Source())
		}
	}
}

// toCallout turns quote into a callout when its first line is an alert
// marker and some text follows it.
func toCallout(quote *ast.Blockquote, src []byte) {
	p, ok := quote.FirstChild().(*ast.Paragraph)
	if !ok {
		return
	}
	marker := p.Lines().At(0)
	m := alertMarker.FindSubmatch(marker.Value(src))
	if m == nil || (p.Lines().Len() == 1 && p.NextSibling() == nil) {
		return
	}
	for c := p.FirstChild(); c != nil; {
		t, ok := c.(*ast.Text)
		if !ok || t.Segment.Start >= marker.Stop {
			break
		}
		next := c.NextSibling()
		p.RemoveChild(p, c)
		c = next
	}
	if p.ChildCount() == 0 {
		quote.RemoveChild(quote, p)
	}
	kind := strings.ToLower(string(m[1]))
	title := ast.NewParagraph()
	title.SetAttributeString("class", []byte("callout-title"))
	title.AppendChild(title, ast.NewString([]byte(strings.ToUpper(kind[:1])+kind[1:])))
	quote.InsertBefore(quote, quote.FirstChild(), title)
	quote.SetAttributeString("class", []byte("callout callout-"+kind))
	quote.SetAttributeString("role", []byte("note"))
}

// cellLabeler names each body cell of a table after its column, so a narrow
// screen can show a row as a block of named cells.
type cellLabeler struct{}

func (cellLabeler) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		table, ok := n.(*east.Table)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		header := table.FirstChild()
		var labels [][]byte
		for c := header.FirstChild(); c != nil; c = c.NextSibling() {
			labels = append(labels, []byte(nodeText(c, reader.Source())))
		}
		// goldmark pads or cuts every row to the header's width.
		for row := header.NextSibling(); row != nil; row = row.NextSibling() {
			for i, c := 0, row.FirstChild(); c != nil; i, c = i+1, c.NextSibling() {
				c.SetAttributeString("data-label", labels[i])
			}
		}
		return ast.WalkSkipChildren, nil
	})
}

// diagram is an SVG drawn into the page itself, so it takes the page's fonts
// and colors.
type diagram struct {
	ast.BaseBlock
	svg []byte
}

var kindDiagram = ast.NewNodeKind("Diagram")

func (d *diagram) Kind() ast.NodeKind { return kindDiagram }

func (d *diagram) Dump(src []byte, level int) { ast.DumpHelper(d, src, level, nil, nil) }

// inlineDiagrams replaces each top-level paragraph that holds nothing but an
// image of a repository SVG with that SVG. A file it cannot read fails the page.
func inlineDiagrams(doc ast.Node, root, source string) error {
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		img, ok := n.FirstChild().(*ast.Image)
		if !ok || n.Kind() != ast.KindParagraph || n.ChildCount() != 1 {
			continue
		}
		dest := string(img.Destination)
		if !strings.HasSuffix(dest, ".svg") || strings.Contains(dest, "://") {
			continue
		}
		svg, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path.Join(path.Dir(source), dest))))
		if err != nil {
			return fmt.Errorf("diagram: %w", err)
		}
		d := &diagram{svg: svg}
		doc.ReplaceChild(doc, n, d)
		n = d
	}
	return nil
}

type diagramRenderer struct{}

func (diagramRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindDiagram, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<figure class="diagram">`)
			_, _ = w.Write(n.(*diagram).svg)
			_, _ = w.WriteString("</figure>\n")
		}
		return ast.WalkSkipChildren, nil
	})
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
