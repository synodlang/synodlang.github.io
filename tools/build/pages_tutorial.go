package main

import (
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// buildTutorial builds tutorial/NN-slug/ for the chapters of
// <dsl>/tutorial and the tutorial/ index (plan §3, Tutorial): a
// `<!-- step K program=<folder> -->` directive becomes a program widget (via
// c.Layout.Program) whose changed lines come from diffLines against step K-1
// (step 1 against the previous chapter's last step); `<!-- new -->` becomes
// the coverage list linking Reference anchors; a `<!-- why <slug> -->`
// shows the captured `synod check --style rich` diagnostic (c.Synod.CheckRich)
// and links the Errors page and its decision; previous and next links.
// Steps with synod.sum go through c.Synod.Get in a temp copy to embed vendor/.
//
// Every step is run natively and must print its stepK.summary; every why
// folder must fail with the code in its expect file; anything else fails the
// build. A dsl without tutorial/ builds no pages.
func buildTutorial(c *Ctx) error {
	root := filepath.Join(c.Source("dsl"), "tutorial")
	if _, err := os.Stat(filepath.Join(root, "README.md")); err != nil {
		fmt.Fprintf(c.Log, "tutorial: no %s, skipped\n", root)
		return nil
	}
	t := &tutorial{c: c, root: root}
	return t.build()
}

type tutorial struct {
	c     *Ctx
	root  string
	cov   []covRow
	codes map[string]bool
	prev  map[string]string // files of the previous step
	chaps []tutChapter
}

type tutChapter struct {
	num   int
	slug  string // 01-hello
	file  string
	title string // plain text of the h1
	doc   *mdDoc
}

func (ch tutChapter) url(c *Ctx) string { return c.URL("tutorial/" + ch.slug + "/") }

var tutChapterFileRe = regexp.MustCompile(`^(\d\d)-([a-z0-9-]+)\.md$`)

func (t *tutorial) build() error {
	c := t.c
	var err error
	if t.codes, err = diagnosticCodes(c); err != nil {
		return err
	}
	if t.cov, err = readCoverage(filepath.Join(t.root, "coverage.md")); err != nil {
		return err
	}
	entries, err := os.ReadDir(t.root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		m := tutChapterFileRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		file := filepath.Join(t.root, e.Name())
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		doc, err := parseMarkdown("tutorial/"+e.Name(), src)
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
			return fmt.Errorf("tutorial/%s: no # heading", e.Name())
		}
		t.chaps = append(t.chaps, tutChapter{num, m[1] + "-" + m[2], file, title, doc})
	}
	sort.Slice(t.chaps, func(i, j int) bool { return t.chaps[i].slug < t.chaps[j].slug })
	for i, ch := range t.chaps {
		if err := t.chapterPage(i, ch); err != nil {
			return fmt.Errorf("%s: %v", ch.slug, err)
		}
	}
	return t.indexPage()
}

func (t *tutorial) indexPage() error {
	c := t.c
	src, err := os.ReadFile(filepath.Join(t.root, "README.md"))
	if err != nil {
		return err
	}
	doc, err := parseMarkdown("tutorial/README.md", src)
	if err != nil {
		return err
	}
	pageURL := c.URL("tutorial/")
	body, err := doc.render(mdOptions{Link: func(h string) string { return siteLink(c, pageURL, "tutorial", h) }})
	if err != nil {
		return err
	}
	return c.AddPage(&Page{URL: pageURL, Title: "Learn Synod", Section: "tutorial", Description: doc.excerpt(200), Body: body, Toc: tocOf(doc, mdOptions{})})
}

func (t *tutorial) chapterPage(i int, ch tutChapter) error {
	c := t.c
	pageURL := ch.url(c)
	link := func(h string) string { return siteLink(c, pageURL, "tutorial", h) }
	opts := mdOptions{Link: link}
	var sb strings.Builder
	inWhy := false
	nextID := 0
	for _, b := range ch.doc.blocks {
		o := opts
		if b.kind == mdkHeading && b.level == 2 {
			inWhy = mdPlainInline(b.text) == "Why it looks like this"
		}
		if inWhy {
			o.CodeSpan = func(text string) (template.HTML, bool) {
				if t.codes[text] {
					return template.HTML(`<a href="` + html.EscapeString(c.URL("errors/"+text+"/")) + `"><code>` + html.EscapeString(text) + `</code></a>`), true
				}
				return "", false
			}
		}
		if b.kind == mdkDirective {
			h, err := t.directive(ch, pageURL, b.text, &nextID)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", ch.file, b.line, err)
			}
			sb.WriteString(string(h))
			continue
		}
		h, err := ch.doc.renderBlocks([]*mdBlock{b}, o)
		if err != nil {
			return err
		}
		sb.WriteString(string(h))
	}
	sb.WriteString(t.pager(i))
	return c.AddPage(&Page{
		URL: pageURL, Title: ch.title, Section: "tutorial",
		Description: ch.doc.excerpt(200), Body: template.HTML(sb.String()), Toc: tocOf(ch.doc, mdOptions{}),
	})
}

// pager links the previous and next chapter (the index before chapter 1).
func (t *tutorial) pager(i int) string {
	c := t.c
	var sb strings.Builder
	sb.WriteString(`<nav class="pager" aria-label="Chapters">`)
	if i == 0 {
		sb.WriteString(`<a rel="prev" href="` + html.EscapeString(c.URL("tutorial/")) + `">&larr; Contents</a>`)
	} else {
		p := t.chaps[i-1]
		sb.WriteString(`<a rel="prev" href="` + html.EscapeString(p.url(c)) + `">&larr; ` + html.EscapeString(p.title) + `</a>`)
	}
	if i+1 < len(t.chaps) {
		n := t.chaps[i+1]
		sb.WriteString(` <a rel="next" href="` + html.EscapeString(n.url(c)) + `">` + html.EscapeString(n.title) + ` &rarr;</a>`)
	} else {
		sb.WriteString(` <a rel="next" href="` + html.EscapeString(c.URL("examples/")) + `">Examples &rarr;</a>`)
	}
	sb.WriteString("</nav>\n")
	return sb.String()
}

// tocOf lists the h2 headings of doc.
func tocOf(doc *mdDoc, o mdOptions) []TocItem {
	var toc []TocItem
	for _, h := range doc.headings(o) {
		if h.Level == 2 || h.Level == 3 {
			toc = append(toc, TocItem{ID: h.ID, Title: h.Text, Level: h.Level})
		}
	}
	return toc
}

var stepRe = regexp.MustCompile(`^step (\d+) program=(\S+)$`)

func (t *tutorial) directive(ch tutChapter, page, text string, nextID *int) (template.HTML, error) {
	switch {
	case text == "new":
		return t.newList(ch, page)
	case stepRe.MatchString(text):
		m := stepRe.FindStringSubmatch(text)
		k, _ := strconv.Atoi(m[1])
		*nextID++
		return t.step(ch, k, m[2], fmt.Sprintf("s%d", *nextID))
	case strings.HasPrefix(text, "why "):
		return t.why(ch, strings.TrimSpace(strings.TrimPrefix(text, "why ")))
	}
	return "", fmt.Errorf("unknown directive %q", text)
}

// covRow is one row of coverage.md.
type covRow struct {
	Item, Shown, Ref string
	Chapter          int
}

func readCoverage(file string) ([]covRow, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	doc, err := parseMarkdown("tutorial/coverage.md", src)
	if err != nil {
		return nil, err
	}
	var rows []covRow
	var bad error
	mdWalkBlocks(doc.blocks, func(b *mdBlock) {
		if b.kind != mdkTable || len(b.head) != 4 || b.head[0] != "Item" {
			return
		}
		for _, r := range b.rows {
			n, err := strconv.Atoi(strings.TrimSpace(r[3]))
			if err != nil {
				bad = fmt.Errorf("tutorial/coverage.md:%d: chapter %q is not a number", b.line, r[3])
				return
			}
			rows = append(rows, covRow{strings.Trim(strings.TrimSpace(r[0]), "`"), r[1], strings.TrimSpace(r[2]), n})
		}
	})
	return rows, bad
}

var refRe = regexp.MustCompile(`^spec/(\d\d)\b`)
var secRe = regexp.MustCompile(`§(\d+(?:\.\d+)*)`)

// refHref returns the Reference or Library URL a coverage Reference cell
// names, or "" when it names none.
func (t *tutorial) refHref(page, ref, item string) string {
	c := t.c
	if strings.HasPrefix(ref, "std/") && strings.HasPrefix(item, "package:") {
		return c.URL("library/" + strings.ReplaceAll(strings.TrimPrefix(item, "package:"), "/", "-") + "/")
	}
	if r, ok := rewriteHref(c, page, ref); ok {
		return r
	}
	if strings.HasPrefix(ref, "std/") {
		return c.URL("library/")
	}
	m := refRe.FindStringSubmatch(ref)
	if m == nil {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(c.Source("dsl"), "spec", m[1]+"-*.md"))
	if len(matches) == 0 {
		return ""
	}
	slug := strings.TrimSuffix(filepath.Base(matches[0]), ".md")
	u := c.URL("reference/" + slug + "/")
	if s := secRe.FindStringSubmatch(ref); s != nil {
		u += "#s" + strings.ReplaceAll(s[1], ".", "-")
	}
	return u
}

// newList renders the coverage rows of the chapter.
func (t *tutorial) newList(ch tutChapter, page string) (template.HTML, error) {
	var sb strings.Builder
	for _, r := range t.cov {
		if r.Chapter != ch.num {
			continue
		}
		shown, err := mdRenderInline("tutorial/coverage.md", r.Shown, mdOptions{})
		if err != nil {
			return "", err
		}
		kind, id, _ := strings.Cut(r.Item, ":")
		sb.WriteString(`<li><span class="new-shown">` + string(shown) + `</span> <span class="new-item">` + html.EscapeString(kind+" "+id) + `</span>`)
		if href := t.refHref(page, r.Ref, r.Item); href != "" {
			sb.WriteString(` <a class="new-ref" href="` + html.EscapeString(href) + `">` + html.EscapeString(r.Ref) + `</a>`)
		} else {
			sb.WriteString(` <span class="new-ref">` + html.EscapeString(r.Ref) + `</span>`)
		}
		sb.WriteString("</li>\n")
	}
	if sb.Len() == 0 {
		return `<p>Nothing new: this chapter only puts what you know to use.</p>`, nil
	}
	return template.HTML(`<ul class="new-list">` + "\n" + sb.String() + "</ul>\n"), nil
}

// tutorialFile reports whether a file of a step folder is part of the
// program: .synod sources, synod.mod and synod.sum, vendor/ included.
func tutorialFile(rel string) bool {
	base := path.Base(rel)
	return strings.HasSuffix(base, ".synod") || base == "synod.mod" || base == "synod.sum"
}

// readProgramFiles reads the program files of dir (slash path to text).
func readProgramFiles(dir string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || rel == "out") {
				return filepath.SkipDir
			}
			return nil
		}
		if !tutorialFile(rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	})
	return files, err
}

// step renders step k of chapter ch as a program widget after running it.
func (t *tutorial) step(ch tutChapter, k int, program, id string) (template.HTML, error) {
	c := t.c
	base := filepath.Join(t.root, "code", ch.slug)
	dir := filepath.Join(base, fmt.Sprintf("step%d", k))
	if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(program))); err != nil || !st.IsDir() {
		return "", fmt.Errorf("step %d: program folder %s is missing in %s", k, program, dir)
	}
	golden, err := os.ReadFile(filepath.Join(base, fmt.Sprintf("step%d.summary", k)))
	if err != nil {
		return "", err
	}
	var files map[string]string
	var toks map[string][]Token
	err = WithCopy(dir, func(tmp string) error {
		if _, err := os.Stat(filepath.Join(tmp, "synod.sum")); err == nil {
			if err := c.Synod.Get(tmp); err != nil {
				return fmt.Errorf("step %d: %v", k, err)
			}
		}
		res, err := c.Synod.RunSummary(tmp, program)
		if err != nil {
			return fmt.Errorf("step %d: %v", k, err)
		}
		if res.Stdout != string(golden) {
			return fmt.Errorf("step %d: synod run %s --summary --style short differs from step%d.summary\n--- got\n%s--- want\n%s", k, program, k, res.Stdout, golden)
		}
		if toks, err = moduleTokens(c, tmp); err != nil {
			return fmt.Errorf("step %d: %v", k, err)
		}
		files, err = readProgramFiles(tmp)
		return err
	})
	if err != nil {
		return "", err
	}
	bl := baselines(t.prev, files)
	show := changedFiles(bl, files)
	d := ProgramData{ID: id, Program: program, Op: "run", Files: files, Predict: ch.num >= 4}
	for _, name := range show {
		changed := diffLines(bl[name], files[name])
		cf := CodeFile{Name: name, HTML: renderCode(files[name], toks[name], changed)}
		for i, n := range changed {
			if i > 0 {
				cf.Changed += ","
			}
			cf.Changed += strconv.Itoa(n)
		}
		d.Show = append(d.Show, name)
		d.Code = append(d.Code, cf)
	}
	t.prev = files
	w, err := c.Layout.Program(d)
	if err != nil {
		return "", err
	}
	return template.HTML(fmt.Sprintf("<div class=\"tutorial-step\" id=\"step-%d\">\n<p class=\"step-label\">Step %d</p>\n%s\n</div>\n", k, k, w)), nil
}

// baselines gives, for every file of cur, the text the step is compared with:
// the previous step's file of that path, or for a new path the vanished
// file it most resembles when at least half of its lines match (a rename,
// such as tutorial/ to counter/ in chapter 4). Other new files have none.
func baselines(prev, cur map[string]string) map[string]string {
	base := map[string]string{}
	for n := range cur {
		if old, ok := prev[n]; ok {
			base[n] = old
		}
	}
	var gone, fresh []string
	for n := range prev {
		if _, ok := cur[n]; !ok && !strings.HasPrefix(n, "vendor/") {
			gone = append(gone, n)
		}
	}
	for n := range cur {
		if _, ok := prev[n]; !ok && !strings.HasPrefix(n, "vendor/") {
			fresh = append(fresh, n)
		}
	}
	sort.Strings(gone)
	sort.Strings(fresh)
	used := map[string]bool{}
	for _, n := range fresh {
		lines := len(splitLines(cur[n]))
		best, bestCommon := "", 0
		for _, g := range gone {
			if used[g] || path.Ext(g) != path.Ext(n) {
				continue
			}
			if common := lines - len(diffLines(prev[g], cur[n])); common > bestCommon {
				best, bestCommon = g, common
			}
		}
		if best != "" && bestCommon*2 >= lines {
			used[best] = true
			base[n] = prev[best]
		}
	}
	return base
}

// changedFiles lists the shown files of a step: the files outside vendor/
// that are new or differ from their baseline, sources first and synod.mod last. When
// nothing changed, every source.
func changedFiles(base, cur map[string]string) []string {
	var names []string
	for n, text := range cur {
		if strings.HasPrefix(n, "vendor/") || path.Base(n) == "synod.sum" {
			continue
		}
		if old, ok := base[n]; !ok || old != text {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		for n := range cur {
			if strings.HasSuffix(n, ".synod") && !strings.HasPrefix(n, "vendor/") {
				names = append(names, n)
			}
		}
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := strings.HasSuffix(names[i], ".synod"), strings.HasSuffix(names[j], ".synod")
		if a != b {
			return a
		}
		return names[i] < names[j]
	})
	return names
}

var tutDecisionRe = regexp.MustCompile(`\bdecisions? (\d+)\b`)

// why renders the diagnostic of why-<slug> and checks it fails with its
// expect code.
func (t *tutorial) why(ch tutChapter, slug string) (template.HTML, error) {
	c := t.c
	dir := filepath.Join(t.root, "code", ch.slug, "why-"+slug)
	exp, err := os.ReadFile(filepath.Join(dir, "expect"))
	if err != nil {
		return "", fmt.Errorf("why %s: %v", slug, err)
	}
	code := strings.TrimSpace(string(exp))
	var out string
	err = WithCopy(dir, func(tmp string) error {
		if _, err := os.Stat(filepath.Join(tmp, "synod.sum")); err == nil {
			if err := c.Synod.Get(tmp); err != nil {
				return err
			}
		}
		var err error
		out, err = whyDiagnostic(c, tmp, code)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("why %s: %v", slug, err)
	}
	esc := html.EscapeString(out)
	esc = tutDecisionRe.ReplaceAllStringFunc(esc, func(s string) string {
		n := tutDecisionRe.FindStringSubmatch(s)[1]
		return `<a href="` + html.EscapeString(c.URL("design/decisions/")) + `#d` + n + `">` + s + `</a>`
	})
	errURL := html.EscapeString(c.URL("errors/" + code + "/"))
	return template.HTML(`<figure class="diagnostic" id="why-` + html.EscapeString(slug) + `">` +
		"\n<pre><code>" + esc + "</code></pre>\n" +
		`<figcaption>Error <a href="` + errURL + `"><code>` + html.EscapeString(code) + `</code></a></figcaption>` + "\n</figure>\n"), nil
}

// whyDiagnostic returns the rich diagnostic of the module in dir and fails
// when it is not code. A program that passes check is run instead: its
// failure is the diagnostic.
func whyDiagnostic(c *Ctx, dir, code string) (string, error) {
	want := "[" + code + "]"
	res, err := c.Synod.CheckRich(dir)
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		out := res.Stdout + res.Stderr
		if !strings.Contains(out, want) {
			return "", fmt.Errorf("check fails with a different diagnostic than %s:\n%s", code, out)
		}
		return strings.TrimRight(out, "\n") + "\n", nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "vendor" || e.Name() == "out" {
			continue
		}
		r, err := c.Synod.must(dir, []int{0, 1}, "run", e.Name(), "--style", "rich", "--color", "never")
		if err != nil {
			return "", err
		}
		if out := r.Stderr + r.Stdout; r.Code != 0 && strings.Contains(out, want) {
			return strings.TrimRight(out, "\n") + "\n", nil
		}
	}
	return "", fmt.Errorf("neither check nor run fails with %s", code)
}

// diagnosticCodes lists the diagnostic codes of `synod explain --json`.
func diagnosticCodes(c *Ctx) (map[string]bool, error) {
	data, err := c.Synod.ExplainJSON()
	if err != nil {
		return nil, err
	}
	var doc struct {
		Codes []struct {
			Code string `json:"code"`
		} `json:"codes"`
	}
	if err := jsonDoc("synod explain --json", data, &doc); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, e := range doc.Codes {
		out[e.Code] = true
	}
	return out, nil
}

// siteLink rewrites a link of a tutorial or guide source (dir is "tutorial"
// or "tutorial/guides", relative to the dsl root) to the site: sibling chapters
// and guides to their pages, the rest through rewriteHref, and any other file
// of the dsl to GitHub at the pinned tag. External links, #fragments and
// absolute paths stay.
func siteLink(c *Ctx, page, dir, href string) string {
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "/") || strings.Contains(href, ":") {
		if r, ok := rewriteHref(c, page, href); ok {
			return r
		}
		return href
	}
	p, frag, _ := strings.Cut(href, "#")
	if frag != "" {
		frag = "#" + frag
	}
	rel := path.Clean(path.Join(dir, p)) // relative to the dsl root
	switch {
	case path.Dir(rel) == "tutorial" && tutChapterFileRe.MatchString(path.Base(rel)):
		return c.URL("tutorial/"+strings.TrimSuffix(path.Base(rel), ".md")+"/") + frag
	case rel == "tutorial/README.md":
		return c.URL("tutorial/") + frag
	case path.Dir(rel) == "tutorial/guides" && strings.HasSuffix(rel, ".md"):
		return c.URL("guides/"+strings.TrimSuffix(path.Base(rel), ".md")+"/") + frag
	case rel == "tutorial/guides":
		return c.URL("guides/") + frag
	}
	if r, ok := rewriteHref(c, page, "../"+rel+frag); ok {
		return r
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return href
	}
	return "https://github.com/synodlang/dsl/blob/" + c.V.DSL.Tag + "/" + rel + frag
}
