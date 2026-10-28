package main

import (
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// buildReference builds reference/NN-slug/, one page per
// <dsl>/spec/NN-*.md rendered by md.go, with heading ids like `s3-3`,
// links rewritten by rewriteHref (plan §3, Reference), a "Taught in" box
// from the coverage rows, and program widgets for complete `# package main`
// blocks (module example.com/snippet, file snippet/main.synod). Also the
// reference/ index.
func buildReference(c *Ctx) error {
	dsl := c.Source("dsl")
	files, err := specFiles(dsl)
	if err != nil {
		return err
	}
	if _, err := linkInfoOf(c); err != nil {
		return err
	}
	taught, err := refTaughtIn(c)
	if err != nil {
		return err
	}
	type chapter struct {
		nn, slug, title, excerpt string
	}
	var chapters []chapter
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".md")
		src, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		d, err := parseMarkdown("spec/"+filepath.Base(f), src)
		if err != nil {
			return err
		}
		heads := specHeadings(d)
		if len(heads) == 0 || heads[0].Level != 1 {
			return fmt.Errorf("spec/%s.md does not start with a chapter heading", base)
		}
		chapters = append(chapters, chapter{base[:2], base, heads[0].Text, d.excerpt(200)})
	}
	for i, f := range files {
		base := chapters[i].slug
		src, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		name := "spec/" + base + ".md"
		d, err := parseMarkdown(name, src)
		if err != nil {
			return err
		}
		url := c.URL("reference/" + base + "/")
		heads := specHeadings(d)
		sn := &refSnippets{c: c, name: name}
		next := 0
		opts := mdOptions{
			HeadingID: func(level int, text string) string {
				id := heads[next%len(heads)].ID
				next++
				return id
			},
			Link: func(href string) string {
				if u, ok := rewriteHref(c, url, href); ok {
					return u
				}
				return href
			},
			Code: sn.render,
		}
		body, err := d.render(opts)
		if err != nil {
			return err
		}
		if sn.err != nil {
			return sn.err
		}
		text := string(body)
		for _, h := range heads {
			if chs := taught[h.ID]; len(chs) > 0 && strings.HasPrefix(h.ID, "s") && h.Level >= 2 {
				text = refInsertAfterHeading(text, h.ID, refTaughtBox(c, chs))
			}
		}
		text = linkifyHTML(c, url, text)
		text += refPager(len(chapters), i, func(ch int) (string, string) {
			return c.URL("reference/" + chapters[ch].slug + "/"), chapters[ch].title
		})
		p := &Page{
			URL: url, Title: chapters[i].title, Section: "reference",
			Description: chapters[i].excerpt, Body: template.HTML(text),
		}
		for _, h := range heads {
			if h.Level == 2 || h.Level == 3 {
				p.Toc = append(p.Toc, TocItem{ID: h.ID, Title: h.Text, Level: h.Level})
			}
		}
		if err := c.AddPage(p); err != nil {
			return err
		}
	}
	var idx strings.Builder
	idx.WriteString("<h1>Reference</h1>\n<p>The specification of Synod, one page per chapter. It is normative: when an example and the specification disagree, the example is a bug. The reasons behind it are in the <a href=\"" +
		c.URL("design/decisions/") + "\">design decisions</a>.</p>\n<ul class=\"chapters\">\n")
	for _, ch := range chapters {
		idx.WriteString(`<li><a href="` + c.URL("reference/"+ch.slug+"/") + `">` + html.EscapeString(ch.title) + `</a>`)
		if ch.excerpt != "" {
			idx.WriteString(" &mdash; " + html.EscapeString(ch.excerpt))
		}
		idx.WriteString("</li>\n")
	}
	idx.WriteString("</ul>\n")
	return c.AddPage(&Page{
		URL: c.URL("reference/"), Title: "Reference", Section: "reference",
		Description: "The Synod language specification, one page per chapter.",
		Body:        template.HTML(idx.String()),
	})
}

// pager renders previous and next links for item i of n items.
func refPager(n, i int, at func(int) (string, string)) string {
	var b strings.Builder
	b.WriteString("<nav class=\"pager\">\n")
	if i > 0 {
		u, t := at(i - 1)
		b.WriteString(`<a class="prev" rel="prev" href="` + u + `">&larr; ` + html.EscapeString(t) + "</a>\n")
	}
	if i+1 < n {
		u, t := at(i + 1)
		b.WriteString(`<a class="next" rel="next" href="` + u + `">` + html.EscapeString(t) + " &rarr;</a>\n")
	}
	b.WriteString("</nav>\n")
	return b.String()
}

// refInsertAfterHeading puts box after the heading element with the given id.
func refInsertAfterHeading(htmlText, id, box string) string {
	at := strings.Index(htmlText, ` id="`+id+`">`)
	if at < 0 {
		return htmlText
	}
	end := strings.Index(htmlText[at:], "</h")
	if end < 0 {
		return htmlText
	}
	end += at
	close := strings.Index(htmlText[end:], ">")
	if close < 0 {
		return htmlText
	}
	end += close + 1
	if end < len(htmlText) && htmlText[end] == '\n' {
		end++
	}
	return htmlText[:end] + box + htmlText[end:]
}

// refChapter is one tutorial chapter that teaches a reference section.
type refChapter struct {
	N     int
	Slug  string
	Title string
}

var refCoverageRe = regexp.MustCompile(`spec/(\d\d) §(\d+)\.(\d+)`)
var refChapterTitleRe = regexp.MustCompile(`^(\d+)\.\s*(.*)$`)

// refTaughtIn reads <dsl>/tutorial/coverage.md and maps a reference heading id
// (s4-1) to the tutorial chapters whose rows name that section. Without a
// tutorial in the pinned dsl it is empty.
func refTaughtIn(c *Ctx) (map[string][]refChapter, error) {
	out := map[string][]refChapter{}
	dir := filepath.Join(c.Source("dsl"), "tutorial")
	src, err := os.ReadFile(filepath.Join(dir, "coverage.md"))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	d, err := parseMarkdown("tutorial/coverage.md", src)
	if err != nil {
		return nil, err
	}
	chapters := map[int]refChapter{}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		n := e.Name()
		if !refChapterFileRe.MatchString(n) {
			continue
		}
		num, _ := strconv.Atoi(n[:2])
		title := fmt.Sprintf("Chapter %d", num)
		if data, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			if cd, err := parseMarkdown(n, data); err == nil {
				if hs := cd.headings(mdOptions{}); len(hs) > 0 && hs[0].Level == 1 {
					title = hs[0].Text
					if m := refChapterTitleRe.FindStringSubmatch(title); m != nil {
						title = fmt.Sprintf("%d. %s", num, m[2])
					}
				}
			}
		}
		chapters[num] = refChapter{num, strings.TrimSuffix(n, ".md"), title}
	}
	for _, b := range d.blocks {
		if b.kind != mdkTable || len(b.head) < 4 {
			continue
		}
		for _, row := range b.rows {
			num, err := strconv.Atoi(strings.TrimSpace(row[3]))
			if err != nil {
				continue
			}
			ch, ok := chapters[num]
			if !ok {
				continue
			}
			for _, m := range refCoverageRe.FindAllStringSubmatch(row[2], -1) {
				n1, _ := strconv.Atoi(m[2])
				id := "s" + strconv.Itoa(n1) + "-" + m[3]
				if !refHasChapter(out[id], num) {
					out[id] = append(out[id], ch)
				}
			}
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool { return out[id][i].N < out[id][j].N })
	}
	return out, nil
}

func refHasChapter(list []refChapter, n int) bool {
	for _, c := range list {
		if c.N == n {
			return true
		}
	}
	return false
}

// refTaughtBox is the "Taught in" box of a section.
func refTaughtBox(c *Ctx, chs []refChapter) string {
	var b strings.Builder
	b.WriteString(`<p class="taught"><span class="taught-label">Taught in</span> `)
	for i, ch := range chs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`<a href="` + c.URL("tutorial/"+ch.Slug+"/") + `">` + html.EscapeString(ch.Title) + `</a>`)
	}
	b.WriteString("</p>\n")
	return b.String()
}

// refSnippets renders the fenced blocks of one reference page: a complete
// program becomes a program widget, a complete library a highlighted block
// with Check, anything else plain text. Every complete block is checked or
// run natively and a failure fails the build.
type refSnippets struct {
	c    *Ctx
	name string
	n    int
	err  error
}

var (
	packageLineRe = regexp.MustCompile(`^# package ([a-z][a-z0-9]*)\s*$`)
	examplesUseRe = regexp.MustCompile(`(?m)^use github\.com/synodlang/examples/([A-Za-z0-9_/.-]+)\s*$`)
)

// snippetKind classifies the text of a synod block: "program" for a
// `# package main` block, "library" for any other `# package name` block
// (with its name), "" for a fragment.
func snippetKind(text string) (kind, pkg string) {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := packageLineRe.FindStringSubmatch(line)
		switch {
		case m == nil:
			return "", ""
		case m[1] == "main":
			return "program", "main"
		}
		return "library", m[1]
	}
	return "", ""
}

func (s *refSnippets) render(info, text string) (template.HTML, bool) {
	if f := strings.Fields(info); len(f) == 0 || f[0] != "synod" {
		return "", false
	}
	kind, pkg := snippetKind(text)
	if kind == "" {
		return template.HTML(`<pre class="synod"><code>` + string(renderCode(text, nil, nil)) + "</code></pre>"), true
	}
	h, err := s.widget(kind, pkg, text)
	if err != nil {
		if s.err == nil {
			s.err = fmt.Errorf("%s: snippet %d: %v", s.name, s.n+1, err)
		}
		return "", false
	}
	return h, true
}

// snippetFiles makes the module of a complete block: synod.mod, the block's
// file and, when it imports github.com/synodlang/examples, the vendored
// files of the imported packages.
func (s *refSnippets) snippetFiles(kind, pkg, text string) (files map[string]string, file string, err error) {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	file = "snippet/main.synod"
	if kind == "library" {
		file = pkg + "/" + pkg + ".synod"
	}
	files = map[string]string{file: text}
	mod := "module example.com/snippet\nsynod 0.2\n"
	if examplesUseRe.MatchString(text) {
		ver := s.c.V.Examples.Tag
		mod += "require github.com/synodlang/examples " + ver + "\n"
		if err := vendorExamples(s.c, files, text, ver); err != nil {
			return nil, "", err
		}
	}
	files["synod.mod"] = mod
	return files, file, nil
}

// vendorExamples adds the files of the examples packages that text imports,
// and of those they import, under vendor/github.com/synodlang/examples@ver/.
func vendorExamples(c *Ctx, files map[string]string, text, ver string) error {
	root := c.Source("examples")
	prefix := "vendor/github.com/synodlang/examples@" + ver + "/"
	mod, err := os.ReadFile(filepath.Join(root, "synod.mod"))
	if err != nil {
		return err
	}
	files[prefix+"synod.mod"] = string(mod)
	seen := map[string]bool{}
	queue := []string{text}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, m := range examplesUseRe.FindAllStringSubmatch(cur, -1) {
			if seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(m[1])))
			if err != nil {
				return fmt.Errorf("import github.com/synodlang/examples/%s: %v", m[1], err)
			}
			for _, e := range ents {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".synod") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(m[1]), e.Name()))
				if err != nil {
					return err
				}
				files[prefix+m[1]+"/"+e.Name()] = string(data)
				queue = append(queue, string(data))
			}
		}
	}
	return nil
}

func (s *refSnippets) widget(kind, pkg, text string) (template.HTML, error) {
	c := s.c
	files, file, err := s.snippetFiles(kind, pkg, text)
	if err != nil {
		return "", err
	}
	var toks map[string][]Token
	err = WithFiles(files, func(dir string) error {
		if kind == "program" {
			res, err := c.Synod.RunSummary(dir, "snippet")
			if err != nil {
				return err
			}
			if res.Code != 0 {
				return fmt.Errorf("synod run snippet: exit %d:\n%s%s", res.Code, res.Stdout, res.Stderr)
			}
		} else if _, err := c.Synod.must(dir, []int{0}, "check", "."); err != nil {
			return err
		}
		var err error
		toks, err = moduleTokens(c, dir)
		return err
	})
	if err != nil {
		return "", err
	}
	s.n++
	d := ProgramData{
		ID: "f" + strconv.Itoa(s.n), Program: "snippet", Op: "run", Files: files,
		Show: []string{file}, Predict: true,
		Code: []CodeFile{{Name: file, HTML: renderCode(files[file], toks[file], nil)}},
	}
	if kind == "library" {
		d.Program, d.Op, d.Predict = pkg, "check", false
	}
	return c.Layout.Program(d)
}
