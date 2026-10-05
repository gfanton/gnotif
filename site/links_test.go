package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRewriteHref(t *testing.T) {
	cases := map[string]struct{ source, href, want string }{
		"sibling page":             {"docs/running-gnotifd.md", "http-api.md", "/docs/http-api/"},
		"sibling page with anchor": {"docs/getting-started.md", "how-it-works.md#what-the-verified-mark-means", "/docs/how-it-works/#what-the-verified-mark-means"},
		"client readme":            {"docs/http-api.md", "../js/README.md", "/docs/browser-client/"},
		"client readme anchor":     {"docs/getting-started.md", "../js/README.md#notifications", "/docs/browser-client/#notifications"},
		"repo readme":              {"docs/deploying-realms.md", "../README.md#develop", "https://github.com/gfanton/gnotif/blob/main/README.md#develop"},
		"workflow file":            {"docs/deploying-realms.md", "../.github/workflows/ci.yml", "https://github.com/gfanton/gnotif/blob/main/.github/workflows/ci.yml"},
		"same page anchor":         {"docs/running-gnotifd.md", "#the-push-service-allowlist", "#the-push-service-allowlist"},
		"absolute url":             {"docs/how-it-works.md", "https://github.com/gnolang/tx-indexer", "https://github.com/gnolang/tx-indexer"},
		"absolute url ending md":   {"docs/how-it-works.md", "https://example.com/notes/README.md", "https://example.com/notes/README.md"},
		"from js readme":           {"js/README.md", "../docs/http-api.md", "/docs/http-api/"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, rewriteHref(tc.source, tc.href, pages))
		})
	}
}
