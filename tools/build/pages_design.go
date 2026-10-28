package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// buildDesign builds design/ (the "Design principles" and "Why Synod"
// sections of the dsl README) and design/decisions/ with the decisions table
// and one #dN anchor per decision (plan §3, Design).
func buildDesign(c *Ctx) error {
	if _, err := linkInfoOf(c); err != nil {
		return err
	}
	if err := buildDesignIndex(c); err != nil {
		return err
	}
	return buildDecisions(c)
}

// designOptions are the Markdown options of the design pages: links
// rewritten for the site, fenced synod blocks shown without highlighting
// (they are fragments).
func designOptions(c *Ctx, page string) mdOptions {
	return mdOptions{
		Link: func(href string) string {
			if u, ok := rewriteHref(c, page, href); ok {
				return u
			}
			return href
		},
		Code: func(info, text string) (template.HTML, bool) {
			if f := strings.Fields(info); len(f) == 0 || f[0] != "synod" {
				return "", false
			}
			return template.HTML(`<pre class="synod"><code>` + string(renderCode(text, nil, nil)) + "</code></pre>"), true
		},
	}
}

// sectionBlocks returns the top-level blocks of the level 2 section titled
// title, heading included, up to the next level 2 heading.
func sectionBlocks(d *mdDoc, title string) ([]*mdBlock, error) {
	start := -1
	for i, b := range d.blocks {
		if b.kind != mdkHeading || b.level > 2 {
			continue
		}
		if start >= 0 {
			return d.blocks[start:i], nil
		}
		if b.level == 2 && mdPlainInline(b.text) == title {
			start = i
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("%s has no section %q", d.name, title)
	}
	return d.blocks[start:], nil
}

func buildDesignIndex(c *Ctx) error {
	src, err := os.ReadFile(filepath.Join(c.Source("dsl"), "README.md"))
	if err != nil {
		return err
	}
	d, err := parseMarkdown("README.md", src)
	if err != nil {
		return err
	}
	url := c.URL("design/")
	opts := designOptions(c, url)
	var b strings.Builder
	b.WriteString("<h1 id=\"design\">Design</h1>\n<p>Why Synod is the way it is. The principles hold together; the " +
		"<a href=\"" + c.URL("design/decisions/") + "\">decisions</a> record every choice the specification had to make, each with its reason.</p>\n")
	var toc []TocItem
	for _, title := range []string{"Design principles", "Why Synod"} {
		blocks, err := sectionBlocks(d, title)
		if err != nil {
			return err
		}
		out, err := d.renderBlocks(blocks, opts)
		if err != nil {
			return err
		}
		b.WriteString(string(out))
		for _, h := range d.headings(mdOptions{}) {
			if h.Level == 2 && h.Text == title {
				toc = append(toc, TocItem{ID: h.ID, Title: h.Text, Level: 2})
			}
		}
	}
	toc = append(toc, TocItem{ID: "decisions", Title: "Decisions", Level: 2})
	b.WriteString("<h2 id=\"decisions\">Decisions</h2>\n<p>See the <a href=\"" + c.URL("design/decisions/") + "\">numbered table of design decisions</a>.</p>\n")
	return c.AddPage(&Page{
		URL: url, Title: "Design", Section: "design",
		Description: "The design principles of Synod and why it is named after Paxos.",
		Body:        template.HTML(linkifyHTML(c, url, b.String())), Toc: toc,
	})
}

func buildDecisions(c *Ctx) error {
	src, err := os.ReadFile(filepath.Join(c.Source("dsl"), "spec", "decisions.md"))
	if err != nil {
		return err
	}
	d, err := parseMarkdown("spec/decisions.md", src)
	if err != nil {
		return err
	}
	url := c.URL("design/decisions/")
	opts := designOptions(c, url)
	rows := decisionRows(d)
	if len(rows) == 0 {
		return fmt.Errorf("spec/decisions.md has no decisions table")
	}
	var b strings.Builder
	tableDone := false
	for _, blk := range d.blocks {
		if blk.kind == mdkTable && !tableDone {
			tableDone = true
			t, err := decisionsTable(rows, opts)
			if err != nil {
				return err
			}
			b.WriteString(t)
			continue
		}
		out, err := d.renderBlocks([]*mdBlock{blk}, opts)
		if err != nil {
			return err
		}
		b.WriteString(string(out))
	}
	b.WriteString("<p><a href=\"" + c.URL("design/") + "\">&larr; Design principles</a></p>\n")
	return c.AddPage(&Page{
		URL: url, Title: "Design decisions", Section: "design",
		Description: "Every design decision of Synod, numbered, with its reason.",
		Body:        template.HTML(linkifyHTML(c, url, b.String())),
	})
}

// decisionsTable renders the decisions as a table whose row N has the id dN
// and a number that links to itself.
func decisionsTable(rows []decisionRow, opts mdOptions) (string, error) {
	var b strings.Builder
	b.WriteString("<table class=\"decisions\">\n<thead>\n<tr>\n<th>#</th>\n<th>Decision</th>\n<th>Why</th>\n</tr>\n</thead>\n<tbody>\n")
	for _, r := range rows {
		id := "d" + strconv.Itoa(r.N)
		dec, err := mdRenderInline("spec/decisions.md", r.Decision, opts)
		if err != nil {
			return "", fmt.Errorf("decision %d: %v", r.N, err)
		}
		why, err := mdRenderInline("spec/decisions.md", r.Why, opts)
		if err != nil {
			return "", fmt.Errorf("decision %d: %v", r.N, err)
		}
		fmt.Fprintf(&b, "<tr id=\"%s\">\n<td><a href=\"#%s\">%d</a></td>\n<td>%s</td>\n<td>%s</td>\n</tr>\n",
			id, id, r.N, dec, why)
	}
	b.WriteString("</tbody>\n</table>\n")
	return b.String(), nil
}
