package main

import (
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// buildGuides builds guides/<slug>/ and the guides/ index from
// <dsl>/tutorial/guides/*.md (plan §3, Guides). A dsl without that folder
// builds no pages. The index lists the guides in the order the tutorial
// README links them, then the others by name.
func buildGuides(c *Ctx) error {
	dir := filepath.Join(c.Source("dsl"), "tutorial", "guides")
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	if len(files) == 0 {
		fmt.Fprintf(c.Log, "guides: no %s, skipped\n", dir)
		return nil
	}
	order := map[string]int{}
	if readme, err := os.ReadFile(filepath.Join(dir, "..", "README.md")); err == nil {
		for _, w := range strings.FieldsFunc(string(readme), func(r rune) bool { return r == '(' || r == ')' }) {
			if rest, ok := strings.CutPrefix(w, "guides/"); ok && strings.HasSuffix(rest, ".md") {
				if _, dup := order[rest]; !dup {
					order[rest] = len(order)
				}
			}
		}
	}
	sort.Slice(files, func(i, j int) bool {
		a, aok := order[filepath.Base(files[i])]
		b, bok := order[filepath.Base(files[j])]
		switch {
		case aok && bok:
			return a < b
		case aok != bok:
			return aok
		}
		return files[i] < files[j]
	})
	var list strings.Builder
	for _, f := range files {
		name := filepath.Base(f)
		slug := strings.TrimSuffix(name, ".md")
		src, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		doc, err := parseMarkdown("tutorial/guides/"+name, src)
		if err != nil {
			return err
		}
		title := ""
		for _, h := range doc.headings(mdOptions{}) {
			if h.Level == 1 {
				title = h.Text
				break
			}
		}
		if title == "" {
			return fmt.Errorf("tutorial/guides/%s: no # heading", name)
		}
		pageURL := c.URL("guides/" + slug + "/")
		opts := mdOptions{Link: func(h string) string { return siteLink(c, pageURL, "tutorial/guides", h) }}
		body, err := doc.render(opts)
		if err != nil {
			return err
		}
		ex := doc.excerpt(200)
		if err := c.AddPage(&Page{URL: pageURL, Title: title, Section: "guides", Description: ex, Body: body, Toc: tocOf(doc, opts)}); err != nil {
			return err
		}
		list.WriteString(`<li><a href="` + html.EscapeString(pageURL) + `">` + html.EscapeString(title) + `</a>`)
		if ex != "" {
			list.WriteString(" &mdash; " + html.EscapeString(ex))
		}
		list.WriteString("</li>\n")
	}
	body := template.HTML("<h1 id=\"guides\">Guides</h1>\n<p>Task pages: do one thing from start to end.</p>\n<ul class=\"guide-list\">\n" + list.String() + "</ul>\n")
	return c.AddPage(&Page{URL: c.URL("guides/"), Title: "Guides", Section: "guides", Description: "Task pages for the Synod CLI, the Mac app, looks, packages and agents.", Body: body})
}
