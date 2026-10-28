package main

import (
	"bytes"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

// searchEntry is one search.json record: url, title, section, excerpt
// (at most 300 characters).
type searchEntry struct {
	U string `json:"u"`
	T string `json:"t"`
	S string `json:"s"`
	X string `json:"x"`
}

const searchExcerptMax = 300

// sectionLabels name the Page.Section keys in search results.
var sectionLabels = map[string]string{
	"tutorial": "Tutorial", "reference": "Reference", "library": "Library",
	"examples": "Examples", "guides": "Guides", "errors": "Errors", "design": "Design", "looks": "Looks",
}

// buildSearch writes search.json of the version: for every registered page of
// the version one entry for the page and one for each heading in its table of
// contents (a Library phrase and an Errors code are such headings or pages).
// It runs after every other builder.
func buildSearch(c *Ctx) error {
	entries := searchEntries(c.Pages.All(), c.DocsURL())
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entries); err != nil {
		return err
	}
	return c.WriteDocs("search.json", buf.Bytes())
}

// searchEntries makes the records of the pages that live under docsURL, or
// outside /docs/ (the tribute); the home page is not searched.
func searchEntries(pages []*Page, docsURL string) []searchEntry {
	entries := []searchEntry{}
	for _, p := range pages {
		if p.URL == "/" || p.URL == docsURL {
			continue
		}
		if !strings.HasPrefix(p.URL, docsURL) && strings.HasPrefix(p.URL, "/docs/") {
			continue
		}
		section := sectionLabels[p.Section]
		if section == "" {
			section = "Docs"
		}
		body := string(p.Body)
		entries = append(entries, searchEntry{U: p.URL, T: p.Title, S: section, X: searchExcerpt(searchText(body))})
		for _, it := range p.Toc {
			if it.ID == "" || it.Title == "" {
				continue
			}
			entries = append(entries, searchEntry{
				U: p.URL + "#" + it.ID, T: it.Title, S: section + ": " + p.Title,
				X: searchExcerpt(searchText(searchAfterID(body, it.ID))),
			})
		}
	}
	return entries
}

var (
	searchSkipBlocks = regexp.MustCompile(`(?is)<(script|style|pre)\b.*?</(script|style|pre)>`)
	searchAnyTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	searchNextHead   = regexp.MustCompile(`(?i)<h[1-6][\s>]`)
)

// searchText is the readable text of an HTML fragment, without code blocks and
// widgets, single-spaced.
func searchText(s string) string {
	s = searchSkipBlocks.ReplaceAllString(s, " ")
	s = searchAnyTag.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

// searchAfterID is the HTML that follows the heading with the given id (or the
// heading at the start of the element with that id) up to the next heading;
// empty when the id is not in body.
func searchAfterID(body, id string) string {
	i := strings.Index(body, `id="`+id+`"`)
	if i < 0 {
		return ""
	}
	rest := body[i:]
	if j := strings.IndexByte(rest, '>'); j >= 0 {
		rest = rest[j+1:]
	}
	if open := strings.LastIndex(body[:i], "<"); open >= 0 && searchHeadTag.MatchString(body[open:]) {
		rest = searchSkipHeading(rest) // id on the heading itself: its text is the title
	}
	if t := strings.TrimLeft(rest, " \t\r\n"); searchHeadTag.MatchString(t) {
		rest = searchSkipHeading(t[strings.IndexByte(t, '>')+1:]) // id on a wrapper
	}
	if j := searchNextHead.FindStringIndex(rest); j != nil {
		rest = rest[:j[0]]
	}
	return rest
}

var searchHeadTag = regexp.MustCompile(`(?i)^<h[1-6][\s>]`)

// searchSkipHeading drops what a heading element holds, up to just after its end tag.
func searchSkipHeading(s string) string {
	if j := strings.Index(strings.ToLower(s), "</h"); j >= 0 {
		if k := strings.IndexByte(s[j:], '>'); k >= 0 {
			return s[j+k+1:]
		}
	}
	return s
}

// searchExcerpt cuts s to at most searchExcerptMax characters at a word boundary.
func searchExcerpt(s string) string {
	if utf8.RuneCountInString(s) <= searchExcerptMax {
		return s
	}
	r := []rune(s)[:searchExcerptMax-1]
	cut := len(r)
	for i := len(r) - 1; i > searchExcerptMax/2; i-- {
		if r[i] == ' ' {
			cut = i
			break
		}
	}
	return strings.TrimRight(string(r[:cut]), " ,;:.") + "…"
}
