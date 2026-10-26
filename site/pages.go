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

// section is one titled group of the sidebar.
type section struct {
	Title string
	Pages []Page
}

// sections groups the documentation in reading order: what a dapp needs for
// its first notifications, then the rest. Nothing else under docs/ reaches
// the site.
var sections = []section{
	{Title: "Basics", Pages: []Page{
		{Title: "Getting started", Source: "docs/getting-started.md", Slug: "getting-started"},
		{Title: "Use cases", Source: "docs/use-cases.md", Slug: "use-cases"},
		{Title: "Triggers", Source: "docs/triggers.md", Slug: "triggers"},
	}},
	{Title: "Advanced", Pages: []Page{
		{Title: "Managing triggers", Source: "docs/managing-triggers.md", Slug: "managing-triggers"},
		{Title: "Browser client", Source: "js/README.md", Slug: "browser-client"},
		{Title: "How gnotif works", Source: "docs/how-it-works.md", Slug: "how-it-works"},
		{Title: "HTTP API", Source: "docs/http-api.md", Slug: "http-api"},
		{Title: "Running gnotifd", Source: "docs/running-gnotifd.md", Slug: "running-gnotifd"},
		{Title: "Deploying the registry", Source: "docs/deploying-realms.md", Slug: "deploying-realms"},
	}},
}

// pages lists every page of sections in reading order, which the pager
// follows.
var pages = readingOrder(sections)

func readingOrder(sections []section) []Page {
	var all []Page
	for _, s := range sections {
		all = append(all, s.Pages...)
	}
	return all
}

const (
	repoURL     = "https://github.com/gfanton/gnotif"
	repoBlobURL = repoURL + "/blob/main/"
	repoEditURL = repoURL + "/edit/main/"
	siteURL     = "https://gnotif.xyz"
	serverURL   = siteURL + "/onyx"
	demoURL     = "https://demo.gnotif.xyz"
	npmURL      = "https://www.npmjs.com/package/gnotif"

	// registryPath is the registry behind serverURL.
	registryPath = "gno.land/r/nym-gfanton001/gnotif/v1"
)
