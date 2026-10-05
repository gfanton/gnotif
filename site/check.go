package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	hrefPattern = regexp.MustCompile(`href="([^"]*)"`)
	idPattern   = regexp.MustCompile(`id="([^"]*)"`)
)

// checkLinks walks the HTML under dir and returns an error naming every
// internal href whose page or fragment does not exist.
func checkLinks(dir string) error {
	htmlFiles := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		htmlFiles[p] = string(data)
		return nil
	})
	if err != nil {
		return err
	}

	ids := map[string]map[string]bool{}
	for p, html := range htmlFiles {
		ids[p] = map[string]bool{}
		for _, m := range idPattern.FindAllStringSubmatch(html, -1) {
			ids[p][m[1]] = true
		}
	}

	var dangling []error
	for p, html := range htmlFiles {
		for _, m := range hrefPattern.FindAllStringSubmatch(html, -1) {
			href := m[1]
			if !strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "#") {
				continue
			}
			target, fragment, _ := strings.Cut(href, "#")
			file := p
			if target != "" {
				file = resolveInternal(dir, target)
				if _, err := os.Stat(file); err != nil {
					dangling = append(dangling, fmt.Errorf("%s: %s", p, href))
					continue
				}
			}
			if fragment != "" && !ids[file][fragment] {
				dangling = append(dangling, fmt.Errorf("%s: %s", p, href))
			}
		}
	}
	if len(dangling) > 0 {
		return fmt.Errorf("dangling links:\n%w", errors.Join(dangling...))
	}
	return nil
}

// resolveInternal maps a site-absolute path to the file that serves it: a
// directory path serves its index.html.
func resolveInternal(dir, target string) string {
	clean := filepath.FromSlash(path.Clean("/" + target))
	if strings.HasSuffix(target, "/") || path.Ext(target) == "" {
		return filepath.Join(dir, clean, "index.html")
	}
	return filepath.Join(dir, clean)
}
