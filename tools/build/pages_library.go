package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// libEsc escapes text for HTML.
func libEsc(s string) string { return template.HTMLEscapeString(s) }

// docDoc is synod-doc/1, the part of it the Library uses.
type docDoc struct {
	Format   string       `json:"format"`
	Packages []docPackage `json:"packages"`
}

type docPackage struct {
	Path    string      `json:"path"`
	Name    string      `json:"name"`
	Title   string      `json:"title"`
	Phrases []docPhrase `json:"phrases"`
	Records []docRecord `json:"records"`
	Node    *docNode    `json:"node"`
	Kind    string      `json:"kind"` // "theme" for a theme package
	Values  []docValue  `json:"values"`
}

// docValue is one name of a theme package (synod-doc/1, kind theme).
type docValue struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value any    `json:"value"`
	Doc   string `json:"doc"`
}

type docPhrase struct {
	Pattern  string `json:"pattern"`
	Receiver *struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"receiver"`
	Gives    string   `json:"gives"`
	Contexts []string `json:"contexts"`
	Kind     string   `json:"kind"`
	Native   bool     `json:"native"`
	Exported bool     `json:"exported"`
	Doc      string   `json:"doc"`
	Def      struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"def"`
}

type docField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Exported bool   `json:"exported"`
}

type docRecord struct {
	Name   string     `json:"name"`
	Doc    string     `json:"doc"`
	Fields []docField `json:"fields"`
}

type docNode struct {
	Doc    string     `json:"doc"`
	Fields []docField `json:"fields"`
}

// libExample is one folder dsl/std/examples/<package>/<name>/.
type libExample struct {
	Key    string   // "net/faults": the program folder in the library module
	Name   string   // "faults"
	Desc   string   // the second line of demo.synod when it is a comment
	Files  []string // slash paths in the module, demo.synod first
	Expect string   // diagnostic code the run stops with on purpose, or ""
	ID     string   // anchor
}

// libPageURL is the docs-relative URL of the page of the package pkg.
func libPageURL(pkg string) string {
	return "library/" + strings.ReplaceAll(pkg, "/", "-") + "/"
}

// libExampleDir is the folder of a package inside dsl/std/examples
// (std/look/basic is lookbasic).
func libExampleDir(pkg string) string {
	return strings.ReplaceAll(strings.TrimPrefix(pkg, "std/"), "/", "")
}

// libSlug makes an anchor from a pattern.
func libSlug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// libModuleFiles reads synod.mod and every *.synod of the library module.
func libModuleFiles(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "synod.mod" || strings.HasSuffix(rel, ".synod") {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files[rel] = string(data)
		}
		return nil
	})
	return files, err
}

// libExamples lists the examples of a package folder.
func libExamples(root, dir string, files map[string]string) []*libExample {
	var out []*libExample
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if err != nil {
		return nil
	}
	for _, e := range entries {
		key := dir + "/" + e.Name()
		demo, ok := files[key+"/demo.synod"]
		if !e.IsDir() || !ok {
			continue
		}
		ex := &libExample{Key: key, Name: e.Name(), ID: "ex-" + libSlug(dir+"-"+e.Name())}
		lines := strings.Split(demo, "\n")
		if len(lines) > 1 && strings.HasPrefix(lines[1], "# ") {
			ex.Desc = strings.TrimPrefix(lines[1], "# ")
		}
		var names []string
		for p := range files {
			if strings.HasPrefix(p, key+"/") && !strings.Contains(p[len(key)+1:], "/") {
				names = append(names, p)
			}
		}
		sort.Slice(names, func(i, j int) bool {
			di, dj := names[i] == key+"/demo.synod", names[j] == key+"/demo.synod"
			if di != dj {
				return di
			}
			return names[i] < names[j]
		})
		ex.Files = names
		if data, err := os.ReadFile(filepath.Join(root, key, "expect")); err == nil {
			ex.Expect = strings.TrimSpace(string(data))
		}
		out = append(out, ex)
	}
	return out
}

// libVerify checks the library module natively: it checks, and every example
// runs with exit 0, or stops with its expect code.
func libVerify(c *Ctx, root string, exs []*libExample) error {
	return WithCopy(root, func(dir string) error {
		if res, err := c.Synod.CheckRich(dir); err != nil {
			return err
		} else if res.Code != 0 {
			return fmt.Errorf("library examples do not check:\n%s%s", res.Stdout, res.Stderr)
		}
		for _, ex := range exs {
			res, err := c.Synod.RunSummary(dir, ex.Key, "--seed", "1")
			if err != nil {
				return err
			}
			out := res.Stdout + res.Stderr
			switch {
			case ex.Expect == "" && res.Code != 0:
				return fmt.Errorf("example %s fails:\n%s", ex.Key, out)
			case ex.Expect != "" && (res.Code == 0 || !strings.Contains(out, "error["+ex.Expect+"]")):
				return fmt.Errorf("example %s must stop with %s, exit %d:\n%s", ex.Key, ex.Expect, res.Code, out)
			}
		}
		return nil
	})
}

// libIndex reads the generated `| Phrase | Examples |` index: phrase to
// example keys. present is false when the file or its rows are missing.
func libIndex(root string, byDir map[string][]*libExample) (rows map[string][]string, present bool, err error) {
	data, rerr := os.ReadFile(filepath.Join(root, "README.md"))
	if rerr != nil {
		return nil, false, nil
	}
	doc, err := parseMarkdown("std/examples/README.md", data)
	if err != nil {
		return nil, false, err
	}
	rows = map[string][]string{}
	all := map[string]bool{}
	for _, l := range byDir {
		for _, e := range l {
			all[e.Key] = true
		}
	}
	for _, b := range doc.blocks {
		if b.kind != mdkTable || len(b.head) < 2 || strings.TrimSpace(b.head[0]) != "Phrase" {
			continue
		}
		for _, r := range b.rows {
			if len(r) < 2 {
				continue
			}
			phrase := strings.TrimSpace(strings.Trim(strings.TrimSpace(r[0]), "`"))
			for _, part := range strings.FieldsFunc(r[1], func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
				name := strings.TrimSpace(part)
				if i := strings.Index(name, "]("); strings.HasPrefix(name, "[") && i > 0 {
					name = name[1:i]
				}
				name = strings.Trim(name, "` ")
				if name == "" {
					continue
				}
				key := name
				if !strings.Contains(key, "/") {
					var found []string
					for k := range all {
						if strings.HasSuffix(k, "/"+key) {
							found = append(found, k)
						}
					}
					if len(found) != 1 {
						return nil, false, fmt.Errorf("std/examples/README.md: example %q of %q is not one example", name, phrase)
					}
					key = found[0]
				}
				key = strings.TrimSuffix(key, "/")
				if !all[key] {
					return nil, false, fmt.Errorf("std/examples/README.md: %q names unknown example %q", phrase, name)
				}
				rows[phrase] = append(rows[phrase], key)
			}
			present = true
		}
	}
	return rows, present, nil
}

// libPhraseExamples finds the index entry of a phrase: its full pattern or
// its words alone.
func libPhraseExamples(rows map[string][]string, ph docPhrase) []string {
	if v, ok := rows[ph.Pattern]; ok {
		return v
	}
	var words []string
	for _, f := range strings.Fields(ph.Pattern) {
		if !strings.HasPrefix(f, "[") && !strings.HasSuffix(f, "]") && !strings.Contains(f, ":") {
			words = append(words, f)
		}
	}
	return rows[strings.Join(words, " ")]
}

// libSections splits dsl/std/README.md at its `## std/...` headings.
func libSections(dsl string) (map[string][]*mdBlock, *mdDoc, error) {
	name := filepath.Join(dsl, "std", "README.md")
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, nil, err
	}
	doc, err := parseMarkdown("std/README.md", data)
	if err != nil {
		return nil, nil, err
	}
	out := map[string][]*mdBlock{}
	cur := ""
	for _, b := range doc.blocks {
		if b.kind == mdkHeading && b.level == 2 {
			cur = ""
			if t := mdPlainInline(b.text); strings.HasPrefix(t, "std/") {
				cur = strings.Fields(t)[0]
				out[cur] = nil
			}
			continue
		}
		if cur != "" {
			out[cur] = append(out[cur], b)
		}
	}
	return out, doc, nil
}

func libMD(c *Ctx, page string) mdOptions {
	return mdOptions{
		Link: func(h string) string {
			if r, ok := rewriteHref(c, page, h); ok {
				return r
			}
			return h
		},
		Code: func(info, text string) (template.HTML, bool) {
			if info != "synod" {
				return "", false
			}
			return template.HTML("<pre class=\"synod\"><code>" + string(renderCode(text, nil, nil)) + "</code></pre>"), true
		},
	}
}

func libInline(c *Ctx, page, src string) template.HTML {
	h, err := mdRenderInline("doc", src, libMD(c, page))
	if err != nil {
		return template.HTML(libEsc(src))
	}
	return h
}

// libWidget renders one library example as a program widget.
func libWidget(c *Ctx, id string, ex *libExample, toks map[string][]Token, op string) (template.HTML, error) {
	d := ProgramData{ID: id, Program: ex.Key, Op: op, FilesURL: "assets/library-examples.json", Predict: true}
	for _, f := range ex.Files {
		d.Show = append(d.Show, f)
	}
	return libProgram(c, d, ex.Files, nil, toks)
}

// libProgram fills the highlighted views of a widget from texts and tokens.
func libProgram(c *Ctx, d ProgramData, names []string, texts map[string]string, toks map[string][]Token) (template.HTML, error) {
	for _, n := range names {
		d.Code = append(d.Code, CodeFile{Name: n, HTML: renderCode(libText(d, texts, n), toks[n], nil)})
	}
	return c.Layout.Program(d)
}

func libText(d ProgramData, texts map[string]string, n string) string {
	if t, ok := texts[n]; ok {
		return t
	}
	return libCur[n]
}

// libCur holds the library module files while pages are built.
var libCur map[string]string

func buildLibrary(c *Ctx) error {
	pkgs, err := c.Synod.StdPackages()
	if err != nil {
		return err
	}
	dsl := c.Source("dsl")
	sections, _, err := libSections(dsl)
	if err != nil {
		return err
	}
	exRoot := filepath.Join(dsl, "std", "examples")
	files, err := libModuleFiles(exRoot)
	if err != nil {
		return err
	}
	libCur = files
	byDir := map[string][]*libExample{}
	var all []*libExample
	for _, p := range pkgs {
		d := libExampleDir(p)
		byDir[d] = libExamples(exRoot, d, files)
		all = append(all, byDir[d]...)
	}
	if len(all) > 0 {
		if err := libVerify(c, exRoot, all); err != nil {
			return err
		}
		data, err := json.Marshal(files)
		if err != nil {
			return err
		}
		if err := c.WriteDocs("assets/library-examples.json", data); err != nil {
			return err
		}
	}
	var toks map[string][]Token
	if len(all) > 0 {
		err := WithCopy(exRoot, func(dir string) error {
			var err error
			toks, err = moduleTokens(c, dir)
			return err
		})
		if err != nil {
			return err
		}
	}
	rows, indexed, err := libIndex(exRoot, byDir)
	if err != nil {
		return err
	}

	type entry struct {
		pkg             docPackage
		url, base       string
		phrases, nexamp int
	}
	var entries []entry
	for _, p := range pkgs {
		raw, err := c.Synod.DocJSON(p)
		if err != nil {
			return err
		}
		var doc docDoc
		if err := jsonDoc("synod doc --json "+p, raw, &doc); err != nil {
			return err
		}
		if doc.Format != "synod-doc/1" || len(doc.Packages) != 1 {
			return fmt.Errorf("synod doc --json %s: unexpected document", p)
		}
		pkg := doc.Packages[0]
		rel := libPageURL(p)
		page := c.URL(rel)
		var b strings.Builder
		var toc []TocItem
		fmt.Fprintf(&b, "<h1><code>%s</code></h1>\n", libEsc(p))
		if pkg.Title != "" {
			fmt.Fprintf(&b, "<p class=\"lead\">%s</p>\n", libEsc(pkg.Title))
		}

		// Intro prose and whole-file blocks of the README section.
		blocks := sections[p]
		if p == "std/look/basic" {
			blocks = nil
			for _, bl := range sections["std/look"] {
				if bl.kind == mdkParagraph && strings.HasPrefix(bl.text, "`std/look/basic`") {
					blocks = append(blocks, bl)
				}
			}
		}
		var keep []*mdBlock
		for _, bl := range blocks {
			if bl.kind == mdkTable && len(bl.head) > 0 && strings.TrimSpace(bl.head[0]) == "Pattern" {
				continue
			}
			keep = append(keep, bl)
		}
		if len(keep) > 0 {
			doc, _ := parseMarkdown("std/README.md", nil)
			html, err := doc.renderBlocks(keep, libMD(c, page))
			if err != nil {
				return fmt.Errorf("%s: %v", p, err)
			}
			b.WriteString("<div class=\"intro\">\n" + string(html) + "\n</div>\n")
		}

		// Phrases, grouped by kind.
		groups := []struct{ kind, title string }{
			{"phrase", "Phrases"}, {"node-event", "Node events"}, {"trace-event", "Trace events"},
		}
		seen := map[string]int{}
		nph := 0
		for _, g := range groups {
			var list []docPhrase
			for _, ph := range pkg.Phrases {
				if ph.Exported && ph.Kind == g.kind {
					list = append(list, ph)
				}
			}
			if len(list) == 0 {
				continue
			}
			id := "kind-" + g.kind
			toc = append(toc, TocItem{ID: id, Title: g.title, Level: 2})
			fmt.Fprintf(&b, "<h2 id=\"%s\">%s</h2>\n", id, g.title)
			for _, ph := range list {
				nph++
				pid := "p-" + libSlug(ph.Pattern)
				if seen[pid]++; seen[pid] > 1 {
					pid += "-" + strconv.Itoa(seen[pid])
				}
				fmt.Fprintf(&b, "<section class=\"phrase\" id=\"%s\">\n<h3><code>%s</code></h3>\n", pid, libEsc(ph.Pattern))
				if ph.Doc != "" {
					fmt.Fprintf(&b, "<p>%s</p>\n", libInline(c, page, ph.Doc))
				}
				b.WriteString("<dl class=\"facts\">\n")
				fmt.Fprintf(&b, "<dt>Gives</dt><dd>%s</dd>\n", libEsc(ph.Gives))
				fmt.Fprintf(&b, "<dt>Contexts</dt><dd>%s</dd>\n", libEsc(strings.Join(ph.Contexts, ", ")))
				impl := "written in Synod"
				if ph.Native {
					impl = "native"
				}
				fmt.Fprintf(&b, "<dt>Kind</dt><dd>%s, %s</dd>\n", libEsc(strings.ReplaceAll(ph.Kind, "-", " ")), impl)
				if ph.Receiver != nil {
					fmt.Fprintf(&b, "<dt>Receiver</dt><dd><code>%s: %s</code></dd>\n", libEsc(ph.Receiver.Name), libEsc(ph.Receiver.Type))
				}
				src := "https://github.com/synodlang/std/blob/" + c.V.Std.Tag + "/" + strings.TrimPrefix(ph.Def.File, "std/") + "#L" + strconv.Itoa(ph.Def.Line)
				fmt.Fprintf(&b, "<dt>Source</dt><dd><a href=\"%s\">%s:%d</a></dd>\n", libEsc(src), libEsc(strings.TrimPrefix(ph.Def.File, "std/")), ph.Def.Line)
				if keys := libPhraseExamples(rows, ph); len(keys) > 0 {
					b.WriteString("<dt>Examples</dt><dd>")
					for i, k := range keys {
						if i > 0 {
							b.WriteString(", ")
						}
						fmt.Fprintf(&b, "<a href=\"#%s\">%s</a>", libEsc("ex-"+libSlug(strings.ReplaceAll(k, "/", "-"))), libEsc(k))
					}
					b.WriteString("</dd>\n")
				} else if indexed {
					return fmt.Errorf("%s: phrase %q has no example in dsl/std/examples/README.md", p, ph.Pattern)
				}
				b.WriteString("</dl>\n</section>\n")
			}
		}

		// Theme values (a theme package: std/theme/default).
		if pkg.Kind == "theme" && len(pkg.Values) > 0 {
			toc = append(toc, TocItem{ID: "values", Title: "Values", Level: 2})
			b.WriteString("<h2 id=\"values\">Values</h2>\n<p>Copy this package into a folder of your own to make a theme: set its first line to <code># theme &lt;folder name&gt;</code> and change the values. A theme gives every name.</p>\n")
			b.WriteString("<div class=\"table-scroll\"><table class=\"theme-values\">\n<thead><tr><th>Name</th><th>Type</th><th>Value</th><th>Meaning</th></tr></thead>\n<tbody>\n")
			for _, v := range pkg.Values {
				fmt.Fprintf(&b, "<tr id=\"v-%s\"><td><code>%s</code></td><td>%s</td><td>%s<code>%s</code></td><td>%s</td></tr>\n",
					libSlug(v.Name), libEsc(v.Name), libEsc(v.Type), swatch(v.Type, v.Value), libEsc(valueText(v.Value)), libEsc(v.Doc))
			}
			b.WriteString("</tbody>\n</table></div>\n")
		}
		if c.V.HasLooks() && strings.HasPrefix(p, "std/look") || c.V.HasLooks() && strings.HasPrefix(p, "std/theme") {
			fmt.Fprintf(&b, "<p>More looks and themes, with an editable demo each, are on <a href=\"%s\">Looks &amp; Themes</a>; <a href=\"%s\">the gallery</a> plays every pair.</p>\n",
				libEsc(c.URL("looks/")), libEsc(c.URL("looks/gallery/")))
		}

		// Records and the Node.
		if len(pkg.Records) > 0 {
			toc = append(toc, TocItem{ID: "records", Title: "Records", Level: 2})
			b.WriteString("<h2 id=\"records\">Records</h2>\n")
			for _, r := range pkg.Records {
				fmt.Fprintf(&b, "<h3 id=\"record-%s\"><code>%s</code></h3>\n", libSlug(r.Name), libEsc(r.Name))
				if r.Doc != "" {
					fmt.Fprintf(&b, "<p>%s</p>\n", libInline(c, page, r.Doc))
				}
				libFields(&b, r.Fields)
			}
		}
		if pkg.Node != nil && (pkg.Node.Doc != "" || len(pkg.Node.Fields) > 0) {
			toc = append(toc, TocItem{ID: "node", Title: "Node", Level: 2})
			b.WriteString("<h2 id=\"node\">Node</h2>\n")
			if pkg.Node.Doc != "" {
				fmt.Fprintf(&b, "<p>%s</p>\n", libInline(c, page, pkg.Node.Doc))
			}
			libFields(&b, pkg.Node.Fields)
		}

		// Examples, once each.
		exs := byDir[libExampleDir(p)]
		if len(exs) > 0 {
			toc = append(toc, TocItem{ID: "examples", Title: "Examples", Level: 2})
			b.WriteString("<h2 id=\"examples\">Examples</h2>\n")
			for i, ex := range exs {
				fmt.Fprintf(&b, "<section class=\"example\" id=\"%s\">\n<h3>%s</h3>\n", ex.ID, libEsc(ex.Name))
				if ex.Desc != "" {
					fmt.Fprintf(&b, "<p>%s</p>\n", libEsc(ex.Desc))
				}
				if ex.Expect != "" {
					fmt.Fprintf(&b, "<p>This run stops on purpose with the diagnostic <a href=\"%s\"><code>%s</code></a>.</p>\n",
						libEsc(c.URL("errors/"+ex.Expect+"/")), libEsc(ex.Expect))
				}
				w, err := libWidget(c, "w"+strconv.Itoa(i+1), ex, toks, "run")
				if err != nil {
					return err
				}
				b.WriteString(string(w) + "\n</section>\n")
			}
		}

		pg := &Page{URL: page, Title: p, Section: "library", Description: pkg.Title, Body: linkBody(c, page, template.HTML(b.String())), Toc: toc}
		if err := c.AddPage(pg); err != nil {
			return err
		}
		entries = append(entries, entry{pkg: pkg, url: page, base: p, phrases: nph, nexamp: len(exs)})
	}

	var b strings.Builder
	b.WriteString("<h1>Library</h1>\n<p>The standard library is the module <code>github.com/synodlang/std</code>, imported as <code>std/&lt;package&gt;</code>. Every phrase below comes from <code>synod doc --json</code> of the compiler this version is built with.</p>\n<table>\n<thead><tr><th>Package</th><th>About</th><th>Phrases</th><th>Examples</th></tr></thead>\n<tbody>\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "<tr><td><a href=\"%s\"><code>%s</code></a></td><td>%s</td><td>%d</td><td>%d</td></tr>\n",
			libEsc(e.url), libEsc(e.base), libEsc(e.pkg.Title), e.phrases, e.nexamp)
	}
	b.WriteString("</tbody>\n</table>\n")
	if c.V.HasLooks() {
		fmt.Fprintf(&b, "<p>Looks and themes beyond the defaults are in the module <code>github.com/synodlang/looks</code>: see <a href=\"%s\">Looks &amp; Themes</a> and <a href=\"%s\">the gallery</a>.</p>\n", libEsc(c.URL("looks/")), libEsc(c.URL("looks/gallery/")))
	}
	return c.AddPage(&Page{URL: c.URL("library/"), Title: "Library", Section: "library",
		Description: "Every phrase of the standard library, with runnable examples.", Body: linkBody(c, c.URL("library/"), template.HTML(b.String()))})
}

func libFields(b *strings.Builder, fs []docField) {
	if len(fs) == 0 {
		return
	}
	b.WriteString("<table>\n<thead><tr><th>Field</th><th>Type</th></tr></thead>\n<tbody>\n")
	for _, f := range fs {
		fmt.Fprintf(b, "<tr><td><code>%s</code></td><td><code>%s</code></td></tr>\n", libEsc(f.Name), libEsc(f.Type))
	}
	b.WriteString("</tbody>\n</table>\n")
}
