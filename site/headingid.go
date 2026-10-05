package main

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
)

// slug turns a heading's text into the id GitHub would give it: lowercase,
// letters, digits, hyphens and underscores kept, every space a hyphen, all
// other characters dropped.
func slug(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range strings.TrimSpace(text) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// githubIDs hands out heading ids the way GitHub does, suffixing repeats
// with -1, -2 and so on. It implements goldmark's parser.IDs.
type githubIDs struct {
	used map[string]int
}

func newGithubIDs() *githubIDs {
	return &githubIDs{used: map[string]int{}}
}

func (g *githubIDs) Generate(value []byte, _ ast.NodeKind) []byte {
	base := slug(string(value))
	id := base
	for n := 1; g.used[id] > 0; n++ {
		id = base + "-" + strconv.Itoa(n)
	}
	g.used[id]++
	return []byte(id)
}

func (g *githubIDs) Put(value []byte) {
	g.used[string(value)]++
}
