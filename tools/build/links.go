package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// linkInfo is what link rewriting needs to know about the sources of one
// docs version: the spec chapters and their heading ids, the decisions and
// the diagnostic codes.
type linkInfo struct {
	specs     map[string]string          // "03" to "03-phrases"
	ids       map[string]map[string]bool // "03" to the heading ids of that chapter
	slugs     map[string]map[string]string
	decisions map[int]bool
	codes     map[string]bool
	tutorial  map[string]string // "04" to "04-first-node"
}

// linkCache holds the linkInfo of every Ctx that asked for one.
var linkCache sync.Map // *Ctx to *linkInfo

// linkInfoOf returns the linkInfo of c, loading it on first use.
func linkInfoOf(c *Ctx) (*linkInfo, error) {
	if v, ok := linkCache.Load(c); ok {
		return v.(*linkInfo), nil
	}
	info, err := loadLinkInfo(c)
	if err != nil {
		return nil, err
	}
	linkCache.Store(c, info)
	return info, nil
}

func loadLinkInfo(c *Ctx) (*linkInfo, error) {
	info, err := loadSpecInfo(c)
	if err != nil {
		return nil, err
	}
	data, err := c.Synod.ExplainJSON()
	if err != nil {
		return nil, err
	}
	var ex struct {
		Codes []struct {
			Code string `json:"code"`
		} `json:"codes"`
	}
	if err := jsonDoc("synod explain --json", data, &ex); err != nil {
		return nil, err
	}
	for _, e := range ex.Codes {
		info.codes[e.Code] = true
	}
	return info, nil
}

// loadSpecInfo reads what the dsl checkout says: chapters, headings,
// decisions and tutorial chapters.
func loadSpecInfo(c *Ctx) (*linkInfo, error) {
	dsl := c.Source("dsl")
	info := &linkInfo{
		specs: map[string]string{}, ids: map[string]map[string]bool{},
		slugs: map[string]map[string]string{}, decisions: map[int]bool{},
		codes: map[string]bool{}, tutorial: map[string]string{},
	}
	files, err := specFiles(dsl)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".md")
		nn := base[:2]
		info.specs[nn] = base
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		d, err := parseMarkdown("spec/"+filepath.Base(f), src)
		if err != nil {
			return nil, err
		}
		info.ids[nn] = map[string]bool{}
		info.slugs[nn] = map[string]string{}
		for _, h := range specHeadings(d) {
			info.ids[nn][h.ID] = true
			info.slugs[nn][mdSlug(h.Text)] = h.ID
		}
	}
	if src, err := os.ReadFile(filepath.Join(dsl, "spec", "decisions.md")); err == nil {
		d, err := parseMarkdown("spec/decisions.md", src)
		if err != nil {
			return nil, err
		}
		for _, row := range decisionRows(d) {
			info.decisions[row.N] = true
		}
	}
	if ents, err := os.ReadDir(filepath.Join(dsl, "tutorial")); err == nil {
		for _, e := range ents {
			if n := e.Name(); refChapterFileRe.MatchString(n) {
				info.tutorial[n[:2]] = strings.TrimSuffix(n, ".md")
			}
		}
	}
	return info, nil
}

var refChapterFileRe = regexp.MustCompile(`^\d\d-[a-z0-9-]+\.md$`)

// specFiles lists <dsl>/spec/NN-*.md in order.
func specFiles(dsl string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dsl, "spec", "[0-9][0-9]-*.md"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s has no spec/NN-*.md", dsl)
	}
	sort.Strings(files)
	return files, nil
}

var sectionNumRe = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?(?:\s+(.*))?$`)

// specHeadings lists the headings of a spec chapter with their ids: a
// numbered section "3.3 Members" is s3-3, the chapter "3. Phrases" is s3,
// and an unnumbered one gets the id of the section it is in plus its slug
// ("### Names" under 3.3 is s3-3-names).
func specHeadings(d *mdDoc) []mdHeading {
	var out []mdHeading
	parent := ""
	seen := map[string]int{}
	for _, h := range d.headings(mdOptions{}) {
		id := ""
		if m := sectionNumRe.FindStringSubmatch(h.Text); m != nil && m[2] != "" {
			id = "s" + strings.ReplaceAll(m[1], ".", "-")
			parent = id
		} else {
			id = mdSlug(h.Text)
			if parent != "" {
				id = parent + "-" + id
			} else {
				id = "s-" + id
			}
		}
		if n := seen[id]; n > 0 {
			id += "-" + strconv.Itoa(n)
		}
		seen[id]++
		h.ID = id
		out = append(out, h)
	}
	return out
}

// decisionRow is one row of the decisions table.
type decisionRow struct {
	N             int
	Decision, Why string // inline Markdown
}

// decisionRows reads the rows `| N | Decision | Why |` of the first table of
// decisions.md whose first column is a number.
func decisionRows(d *mdDoc) []decisionRow {
	var out []decisionRow
	for _, b := range d.blocks {
		if b.kind != mdkTable || len(b.head) < 3 {
			continue
		}
		for _, r := range b.rows {
			n, err := strconv.Atoi(strings.TrimSpace(r[0]))
			if err != nil || len(r) < 3 {
				continue
			}
			out = append(out, decisionRow{n, r[1], r[2]})
		}
	}
	return out
}

var (
	sectionRe  = regexp.MustCompile(`^§(\d+)\.(\d+)$`)
	specRefRe  = regexp.MustCompile(`^spec/(\d\d)(?:-[a-z0-9-]+\.md)?(?:\s+§(\d+)\.(\d+))?$`)
	bareSpecRe = regexp.MustCompile(`^(\d\d)-[a-z0-9-]+\.md$`)
	implRe     = regexp.MustCompile(`^implementation/[A-Za-z0-9._-]+\.md$`)
	tutorialRe = regexp.MustCompile(`^tutorial/(\d\d-[a-z0-9-]+)\.md$`)
	guideRe    = regexp.MustCompile(`^tutorial/guides/([a-z0-9-]+)\.md$`)
)

// rewriteHref rewrites one link of a Markdown source page for the site
// (plan §3, Reference): spec/03 §3.3 and ../spec/NN-*.md go to
// reference/NN-slug/#s3-3, implementation/*.md go to GitHub at the pinned
// dsl tag, spec/decisions.md goes to design/decisions/, a bare §3.3 goes to
// the reference section, tutorial/NN-*.md, tutorial/guides/*.md and
// std/README.md go to their pages, and a code span equal to a diagnostic
// code to the Errors page. page is the site URL of the page the link is on.
// ok is false when href is left as it is.
func rewriteHref(c *Ctx, page, href string) (rewritten string, ok bool) {
	info, err := linkInfoOf(c)
	if err != nil {
		return href, false
	}
	return info.rewrite(c, page, href)
}

func (l *linkInfo) rewrite(c *Ctx, page, href string) (string, bool) {
	if u, ok := l.rewriteSite(c, page, href); ok {
		return u, true
	}
	return href, false
}

func (l *linkInfo) rewriteSite(c *Ctx, page, href string) (string, bool) {
	switch {
	case href == "", strings.HasPrefix(href, "#"), strings.HasPrefix(href, "/"),
		strings.HasPrefix(href, "mailto:"), strings.Contains(href, "://"):
		return href, false
	}
	p, frag, _ := strings.Cut(href, "#")
	p = strings.TrimSpace(p)
	for {
		switch {
		case strings.HasPrefix(p, "./"):
			p = p[2:]
			continue
		case strings.HasPrefix(p, "../"):
			p = p[3:]
			continue
		}
		break
	}
	var url string
	switch {
	case sectionRe.MatchString(p):
		m := sectionRe.FindStringSubmatch(p)
		return l.section(c, page, m[1], m[2])
	case specRefRe.MatchString(p):
		m := specRefRe.FindStringSubmatch(p)
		if m[2] != "" {
			return l.section(c, page, m[2], m[3])
		}
		return l.specPage(c, page, m[1], frag)
	case bareSpecRe.MatchString(p):
		return l.specPage(c, page, p[:2], frag)
	case p == "spec/decisions.md" || p == "decisions.md":
		url = c.URL("design/decisions/")
	case implRe.MatchString(p):
		url = "https://github.com/synodlang/dsl/blob/" + c.V.DSL.Tag + "/" + p
		if frag != "" {
			url += "#" + frag
		}
		return url, true
	case p == "tutorial/README.md":
		url = c.URL("tutorial/")
	case tutorialRe.MatchString(p):
		nn := p[len("tutorial/") : len("tutorial/")+2]
		slug, has := l.tutorial[nn]
		if !has {
			return href, false
		}
		url = c.URL("tutorial/" + slug + "/")
	case guideRe.MatchString(p):
		url = c.URL("guides/" + guideRe.FindStringSubmatch(p)[1] + "/")
	case p == "std/README.md":
		url = c.URL("library/")
	default:
		return href, false
	}
	return withFragment(page, url, frag), true
}

// section is the link to section N.M of the spec.
func (l *linkInfo) section(c *Ctx, page, n, m string) (string, bool) {
	nn := n
	if len(nn) == 1 {
		nn = "0" + nn
	}
	slug, has := l.specs[nn]
	id := "s" + n + "-" + m
	if !has || !l.ids[nn][id] {
		return "", false
	}
	return withFragment(page, c.URL("reference/"+slug+"/"), id), true
}

// specPage is the link to a spec chapter with an optional heading fragment,
// given as a site id or as a GitHub-style slug of the heading text.
func (l *linkInfo) specPage(c *Ctx, page, nn, frag string) (string, bool) {
	slug, has := l.specs[nn]
	if !has {
		return "", false
	}
	if frag != "" && !l.ids[nn][frag] {
		if id, found := l.slugs[nn][frag]; found {
			frag = id
		}
	}
	return withFragment(page, c.URL("reference/"+slug+"/"), frag), true
}

// withFragment adds #frag to url, or gives just "#frag" when url is page.
func withFragment(page, url, frag string) string {
	if frag == "" {
		return url
	}
	if url == page {
		return "#" + frag
	}
	return url + "#" + frag
}

// decisionURL is the link to decision n, empty when there is none.
func (l *linkInfo) decisionURL(c *Ctx, page string, n int) string {
	if !l.decisions[n] {
		return ""
	}
	return withFragment(page, c.URL("design/decisions/"), "d"+strconv.Itoa(n))
}

// refURL resolves a Reference cell of coverage.md such as "spec/06 §6.1" or
// a bare "§3.3" to a site path, for the tutorial's coverage lists.
func refURL(c *Ctx, page, ref string) (string, bool) {
	return rewriteHref(c, page, strings.TrimSpace(ref))
}

var (
	htmlTokRe     = regexp.MustCompile(`(?s)<!--.*?-->|<[^>]*>|[^<]+`)
	bareSectionRe = regexp.MustCompile(`§(\d+)\.(\d+)`)
	followSecRe   = regexp.MustCompile(`^\s*§(\d+)\.(\d+)`)
	decisionRe    = regexp.MustCompile(`\b[Dd]ecisions? \d+(?:\s*(?:,|and|or|&amp;|–|-)\s*\d+)*`)
	numberRe      = regexp.MustCompile(`\d+`)
	tagNameRe     = regexp.MustCompile(`^<([a-zA-Z][a-zA-Z0-9]*)`)
)

// protectedTags are copied unchanged with everything they contain.
var protectedTags = map[string]bool{
	"pre": true, "script": true, "style": true, "a": true, "textarea": true, "button": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// linkifyHTML adds the links that plain text and code spans of rendered
// Markdown imply: `spec/03` §3.3 and bare §3.3 to the Reference,
// `implementation/x.md` to GitHub, "decision N" to the decisions page and a
// code span equal to a diagnostic code to its Errors page. It leaves code
// blocks, scripts, headings and existing links alone. page is the site URL
// of the page the html is on.
func linkifyHTML(c *Ctx, page, in string) string {
	info, err := linkInfoOf(c)
	if err != nil {
		return in
	}
	toks := htmlTokRe.FindAllString(in, -1)
	var out strings.Builder
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if !strings.HasPrefix(t, "<") || strings.HasPrefix(t, "<!--") {
			if strings.HasPrefix(t, "<") {
				out.WriteString(t)
			} else {
				out.WriteString(info.linkText(c, page, t))
			}
			continue
		}
		m := tagNameRe.FindStringSubmatch(t)
		name := ""
		if m != nil {
			name = strings.ToLower(m[1])
		}
		switch {
		case protectedTags[name]:
			j := i
			for j < len(toks) && !strings.EqualFold(toks[j], "</"+name+">") {
				j++
			}
			for k := i; k <= j && k < len(toks); k++ {
				out.WriteString(toks[k])
			}
			i = j
		case name == "code" && t == "<code>" && i+2 < len(toks) && toks[i+2] == "</code>":
			text := html.UnescapeString(toks[i+1])
			href, tail := info.codeLink(c, page, text, toks, i+3)
			if href == "" {
				out.WriteString(toks[i] + toks[i+1] + toks[i+2])
			} else {
				out.WriteString(`<a href="` + html.EscapeString(href) + `">` + toks[i] + toks[i+1] + toks[i+2])
				if tail != "" {
					out.WriteString(tail)
					toks[i+3] = strings.TrimPrefix(toks[i+3], tail)
				}
				out.WriteString("</a>")
			}
			i += 2
		default:
			out.WriteString(t)
		}
	}
	return out.String()
}

// codeLink gives the link of the code span text; tail is the "§3.3" text
// that follows a `spec/NN` span in toks[next] and belongs to the link.
func (l *linkInfo) codeLink(c *Ctx, page, text string, toks []string, next int) (href, tail string) {
	if l.codes[text] {
		return c.URL("errors/" + text + "/"), ""
	}
	if !strings.HasPrefix(text, "spec/") && !strings.HasPrefix(text, "implementation/") {
		return "", ""
	}
	if specRefRe.MatchString(text) && next < len(toks) && !strings.HasPrefix(toks[next], "<") {
		if m := followSecRe.FindStringSubmatch(toks[next]); m != nil {
			if u, ok := l.rewrite(c, page, text+" §"+m[1]+"."+m[2]); ok {
				return u, m[0]
			}
		}
	}
	if u, ok := l.rewrite(c, page, text); ok {
		return u, ""
	}
	return "", ""
}

// linkText links the bare sections and decisions of one text node (already
// HTML-escaped).
func (l *linkInfo) linkText(c *Ctx, page, s string) string {
	if !strings.Contains(s, "§") && !strings.Contains(s, "ecision") {
		return s
	}
	s = decisionRe.ReplaceAllStringFunc(s, func(m string) string {
		return numberRe.ReplaceAllStringFunc(m, func(num string) string {
			n, _ := strconv.Atoi(num)
			if u := l.decisionURL(c, page, n); u != "" {
				return `<a href="` + u + `">` + num + `</a>`
			}
			return num
		})
	})
	return replaceOutsideTags(s, bareSectionRe, func(m []string) string {
		if u, ok := l.section(c, page, m[1], m[2]); ok {
			return `<a href="` + u + `">` + m[0] + `</a>`
		}
		return m[0]
	})
}

// replaceOutsideTags replaces matches of re in the text of s, leaving the
// anchors that linkText has just added untouched.
func replaceOutsideTags(s string, re *regexp.Regexp, fn func(m []string) string) string {
	toks := htmlTokRe.FindAllString(s, -1)
	var b strings.Builder
	inA := false
	for _, t := range toks {
		switch {
		case strings.HasPrefix(t, "<a "):
			inA = true
			b.WriteString(t)
		case t == "</a>":
			inA = false
			b.WriteString(t)
		case strings.HasPrefix(t, "<") || inA:
			b.WriteString(t)
		default:
			b.WriteString(re.ReplaceAllStringFunc(t, func(m string) string {
				return fn(re.FindStringSubmatch(m))
			}))
		}
	}
	return b.String()
}
