package main

// Page is one documentation page: its sidebar title, the markdown file it
// is rendered from (repo-relative) and the last segment of its URL.
type Page struct {
	Title  string
	Source string
	Slug   string
}

func (p Page) URL() string {
	return "/docs/" + p.Slug + "/"
}

// pages lists the documentation in reading order; nothing else under docs/
// reaches the site.
var pages = []Page{
	{Title: "Getting started", Source: "docs/getting-started.md", Slug: "getting-started"},
	{Title: "How gnotif works", Source: "docs/how-it-works.md", Slug: "how-it-works"},
	{Title: "Browser client", Source: "js/README.md", Slug: "browser-client"},
	{Title: "HTTP API", Source: "docs/http-api.md", Slug: "http-api"},
	{Title: "Running gnotifd", Source: "docs/running-gnotifd.md", Slug: "running-gnotifd"},
	{Title: "Deploying the realms", Source: "docs/deploying-realms.md", Slug: "deploying-realms"},
}

const (
	repoURL     = "https://github.com/gfanton/gnotif"
	repoBlobURL = repoURL + "/blob/main/"
	repoEditURL = repoURL + "/edit/main/"
	serverURL   = "https://gnotif.xyz"
	npmURL      = "https://www.npmjs.com/package/gnotif"

	// registryPath and pingpongPath are the realms behind serverURL. The
	// placeholder namespace is replaced once the deploy is known.
	registryPath = "gno.land/r/<namespace>/gnotif/v0"
	pingpongPath = "gno.land/r/<namespace>/pingpong/v0"
)
