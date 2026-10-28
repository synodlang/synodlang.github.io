package main

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// buildTribute builds /tribute/ from content/tribute.md (latest version
// only: it returns nil when !c.IsLatest()). A link written "docs:<path>"
// leads to that page of the version being built.
func buildTribute(c *Ctx) error {
	if !c.IsLatest() {
		return nil
	}
	body, err := renderContent(c, "tribute.md")
	if err != nil {
		return err
	}
	return c.AddPage(&Page{
		URL:         "/tribute/",
		Title:       "Tribute",
		Description: "Leslie Lamport's The Part-Time Parliament and why the language is named Synod.",
		Body:        body,
	})
}

// renderContent renders content/<name> with the site's Markdown renderer.
func renderContent(c *Ctx, name string) (template.HTML, error) {
	src, err := os.ReadFile(filepath.Join(c.Root, "content", name))
	if err != nil {
		return "", err
	}
	doc, err := parseMarkdown("content/"+name, src)
	if err != nil {
		return "", err
	}
	return doc.render(mdOptions{Link: func(href string) string {
		if rest, ok := strings.CutPrefix(href, "docs:"); ok {
			return c.URL(rest)
		}
		return href
	}})
}
