package main

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Looks & Themes (M12, spec 09-site-and-org): /docs/v0.3/looks/ shows every
// package of github.com/synodlang/looks and the std look and theme packages
// with source, values and an editable demo; /docs/v0.3/looks/gallery/ plays
// every look x theme pair. The module is read from the looks checkout; its
// demos run natively during the build, and the gallery scenes are made by
// the native `synod run` (the wasm embeds only std, G52).

const (
	galleryPace   = "3"
	galleryDemoMS = "6s"
)

// galleryLooks and galleryThemes are the matrix of the gallery (the pairs
// scripts/check.sh of the looks module runs). "basic" and "default" are the
// std packages that a program without an import uses.
var (
	galleryLooks  = []string{"basic", "lumen", "doodle"}
	galleryThemes = []string{"default", "solarized/light", "solarized/dark", "terminal", "sketch", "chalk"}
	// pairings the looks README names as good ones.
	galleryBest = map[string]bool{"lumen/default": true, "doodle/sketch": true, "lumen/chalk": true}
)

var looksExamplesReqRe = regexp.MustCompile(`(?m)^require github\.com/synodlang/examples\s+(\S+)`)

// looksModuleFiles reads synod.mod and the *.synod files of the looks checkout
// (vendor/ and out/ left out). Its require of the examples module is set to
// the version the docs pin, so the vendored copy the page adds matches it.
func looksModuleFiles(c *Ctx) (map[string]string, error) {
	files, err := exampleFiles(c.Source("looks"))
	if err != nil {
		return nil, err
	}
	m := looksExamplesReqRe.FindStringSubmatch(files["synod.mod"])
	if m == nil {
		return nil, fmt.Errorf("looks synod.mod requires no examples module")
	}
	if m[1] != c.V.Examples.Tag {
		if !c.IsLocal() {
			return nil, fmt.Errorf("looks synod.mod requires examples %s but %s pins %s", m[1], c.V.Docs, c.V.Examples.Tag)
		}
		files["synod.mod"] = looksExamplesReqRe.ReplaceAllString(files["synod.mod"], "require github.com/synodlang/examples "+c.V.Examples.Tag)
	}
	return files, nil
}

// looksWork runs fn in a temporary copy of the looks module with the examples
// module vendored, as a program on the site carries it.
func looksWork(c *Ctx, fn func(dir string, files map[string]string) error) error {
	files, err := looksModuleFiles(c)
	if err != nil {
		return err
	}
	ex, err := exampleFiles(c.Source("examples"))
	if err != nil {
		return err
	}
	all := map[string]string{}
	for n, t := range files {
		all[n] = t
	}
	for n, t := range ex {
		all["vendor/github.com/synodlang/examples@"+c.V.Examples.Tag+"/"+n] = t
	}
	return WithFiles(all, func(dir string) error { return fn(dir, files) })
}

// looksPkg is one package of the looks module that has a demo.
type looksPkg struct {
	Path  string // "lumen", "solarized/light"
	Name  string // last element
	Theme bool
	Desc  string // README cell
	Lead  string // the leading comment lines of the package file
}

func (p looksPkg) slug() string { return strings.ReplaceAll(p.Path, "/", "-") }

// looksPackages lists the packages of the module: looks first, in the order
// of the README tables, then themes.
func looksPackages(files map[string]string, readme string) ([]looksPkg, error) {
	descs := map[string]string{}
	order := map[string]int{}
	for i, line := range strings.Split(readme, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		m := exampleFolderRe2.FindStringSubmatch(strings.TrimSpace(cells[0]))
		if m == nil || len(cells) < 2 {
			continue
		}
		descs[m[1]] = strings.TrimSpace(cells[len(cells)-1])
		order[m[1]] = i
	}
	var pkgs []looksPkg
	for n := range files {
		dir, ok := strings.CutSuffix(n, "/demo.synod")
		if !ok {
			continue
		}
		name := dir[strings.LastIndex(dir, "/")+1:]
		text, ok := files[dir+"/"+name+".synod"]
		if !ok {
			return nil, fmt.Errorf("looks: %s has a demo but no %s.synod", dir, name)
		}
		p := looksPkg{Path: dir, Name: name, Theme: strings.HasPrefix(text, "# theme "), Desc: descs[dir]}
		var lead []string
		for _, l := range strings.Split(text, "\n")[1:] {
			if !strings.HasPrefix(l, "# ") {
				break
			}
			lead = append(lead, strings.TrimPrefix(l, "# "))
		}
		p.Lead = strings.Join(lead, " ")
		if p.Desc == "" {
			return nil, fmt.Errorf("looks README.md has no table row for `%s`", dir)
		}
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		a, b := pkgs[i], pkgs[j]
		if a.Theme != b.Theme {
			return !a.Theme
		}
		return order[a.Path] < order[b.Path]
	})
	return pkgs, nil
}

var exampleFolderRe2 = regexp.MustCompile("^`([a-z0-9/]+)`")

// looksGalleryMain writes the main file of a gallery cell (the recipe of
// scripts/check.sh in the looks module); use lines are in canonical order.
func looksGalleryMain(look, theme string) string {
	uses := []string{"github.com/synodlang/examples/raft", "std/net", "std/sim"}
	if look != "basic" {
		uses = append(uses, "github.com/synodlang/looks/"+look)
	}
	if theme != "default" {
		uses = append(uses, "github.com/synodlang/looks/"+theme)
	}
	sort.Strings(uses)
	var b strings.Builder
	b.WriteString("# package main\n")
	for _, u := range uses {
		b.WriteString("use " + u + "\n")
	}
	b.WriteString("\ndefine main\n  sim.Seed 3\n  net.Latency between 5ms and 15ms\n  set nodes to (sim.Cluster of 5 running raft)\n  raft.Meet nodes\n  after 2s\n    sim.Crash (item 0 of raft.Leaders)\n  sim.StopAt " + galleryDemoMS + "\n")
	return b.String()
}

func galleryFile(look, theme string) string {
	return "gallery/" + look + "_" + strings.ReplaceAll(theme, "/", "-") + ".json"
}

func buildLooks(c *Ctx) error {
	if !c.V.HasLooks() {
		return nil
	}
	files, err := looksModuleFiles(c)
	if err != nil {
		return err
	}
	readme, err := os.ReadFile(filepath.Join(c.Source("looks"), "README.md"))
	if err != nil {
		return err
	}
	pkgs, err := looksPackages(files, string(readme))
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("looks: no package with a demo.synod")
	}
	assetName := "looks-" + c.V.Looks.Tag + ".json"
	if err := writeLooksAsset(c, assetName, files); err != nil {
		return err
	}

	var toks map[string][]Token
	docs := map[string]docPackage{}
	err = looksWork(c, func(dir string, _ map[string]string) error {
		if res, err := c.Synod.CheckRich(dir); err != nil {
			return err
		} else if res.Code != 0 {
			return fmt.Errorf("looks module does not check:\n%s%s", res.Stdout, res.Stderr)
		}
		for _, p := range pkgs {
			res, err := c.Synod.RunSummary(dir, p.Path)
			if err != nil {
				return err
			}
			if res.Code != 0 {
				return fmt.Errorf("looks/%s: synod run exited %d:\n%s%s", p.Path, res.Code, res.Stdout, res.Stderr)
			}
			if p.Theme {
				pd, err := looksDoc(c, dir, "github.com/synodlang/looks/"+p.Path)
				if err != nil {
					return err
				}
				docs[p.Path] = pd
			}
		}
		var e error
		toks, e = moduleTokens(c, dir)
		return e
	})
	if err != nil {
		return err
	}
	var def docPackage
	if raw, err := c.Synod.DocJSON("std/theme/default"); err != nil {
		return err
	} else if def, err = firstDocPackage("std/theme/default", raw); err != nil {
		return err
	}
	if len(def.Values) == 0 {
		return fmt.Errorf("synod doc --json std/theme/default has no values")
	}

	if err := buildGallery(c); err != nil {
		return err
	}
	return buildLooksPage(c, pkgs, files, toks, docs, def, assetName)
}

func firstDocPackage(what string, raw []byte) (docPackage, error) {
	var doc docDoc
	if err := jsonDoc("synod doc --json "+what, raw, &doc); err != nil {
		return docPackage{}, err
	}
	if doc.Format != "synod-doc/1" || len(doc.Packages) != 1 {
		return docPackage{}, fmt.Errorf("synod doc --json %s: unexpected document", what)
	}
	return doc.Packages[0], nil
}

func looksDoc(c *Ctx, dir, pkg string) (docPackage, error) {
	raw, err := c.Synod.DocIn(dir, pkg)
	if err != nil {
		return docPackage{}, err
	}
	p, err := firstDocPackage(pkg, raw)
	if err != nil {
		return p, err
	}
	if p.Kind != "theme" || len(p.Values) == 0 {
		return p, fmt.Errorf("synod doc --json %s: not a theme package with values", pkg)
	}
	return p, nil
}

// buildGallery makes the scene of every look x theme pair with the native
// `synod run` and writes the gallery page.
func buildGallery(c *Ctx) error {
	total := 0
	err := looksWork(c, func(dir string, files map[string]string) error {
		for _, look := range galleryLooks {
			for _, theme := range galleryThemes {
				prog := "gallery-" + look + "-" + strings.ReplaceAll(theme, "/", "-")
				main := filepath.Join(dir, prog, "demo.synod")
				if err := writeFile(main, []byte(looksGalleryMain(look, theme)), 0o644); err != nil {
					return err
				}
				out := filepath.Join(dir, prog+".scene.json")
				if err := c.Synod.RunScene(dir, prog, out, "--pace", galleryPace); err != nil {
					return fmt.Errorf("gallery %s x %s: %v", look, theme, err)
				}
				data, err := os.ReadFile(out)
				if err != nil {
					return err
				}
				total += len(data)
				if err := c.WriteDocs("assets/"+galleryFile(look, theme), data); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(c.Log, "gallery: %d scenes, %d KB\n", len(galleryLooks)*len(galleryThemes), total/1024)

	var b strings.Builder
	b.WriteString("<h1>Gallery</h1>\n<p class=\"lead\">Every look drawn in every theme. Each cell is the same Raft election, run once by <code>synod run</code> when this site was built and played here by the player. The events of a run never change; only how it is drawn does.</p>\n")
	fmt.Fprintf(&b, "<p>Cells start playing when they scroll into view. A pairing marked <em>recommended</em> is one the looks module suggests. <a href=\"%s\">Looks &amp; Themes</a> has the source of each package and a demo you can edit.</p>\n", html.EscapeString(c.URL("looks/")))
	b.WriteString("<div class=\"gallery\" data-gallery>\n")
	for _, look := range galleryLooks {
		lookName := look
		if look == "basic" {
			lookName = "std/look/basic"
		}
		fmt.Fprintf(&b, "<h2 id=\"look-%s\"><code>%s</code></h2>\n<div class=\"gal-grid\">\n", html.EscapeString(look), html.EscapeString(lookName))
		for _, theme := range galleryThemes {
			themeName := theme
			if theme == "default" {
				themeName = "std/theme/default"
			}
			rec := ""
			if galleryBest[look+"/"+theme] {
				rec = " <span class=\"gal-tag\">recommended</span>"
			}
			id := "cell-" + look + "-" + strings.ReplaceAll(theme, "/", "-")
			fmt.Fprintf(&b, "<figure class=\"gal-cell\" id=\"%s\" data-scene=\"assets/%s\" data-state=\"idle\">\n<canvas width=\"1600\" height=\"1000\" role=\"img\" aria-label=\"Raft election drawn by %s in %s\"></canvas>\n<figcaption><span><code>%s</code> × <code>%s</code>%s</span> <button type=\"button\" class=\"synod-btn synod-btn-quiet gal-toggle\" aria-label=\"Pause or play %s in %s\">Pause</button></figcaption>\n</figure>\n",
				id, galleryFile(look, theme), html.EscapeString(lookName), html.EscapeString(themeName),
				html.EscapeString(lookName), html.EscapeString(themeName), rec, html.EscapeString(lookName), html.EscapeString(themeName))
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</div>\n<script type=\"module\" src=\"" + html.EscapeString(c.AssetsURL()+"js/gallery.js") + "\"></script>\n")
	return c.AddPage(&Page{
		URL:         c.URL("looks/gallery/"),
		Title:       "Looks and themes gallery",
		Section:     "looks",
		Description: "Every look drawn in every theme, playing the same Raft election.",
		Body:        linkBody(c, c.URL("looks/gallery/"), template.HTML(b.String())),
	})
}

func swatch(typ string, v any) string {
	s := fmt.Sprint(v)
	if typ == "color" && strings.HasPrefix(s, "#") {
		return "<span class=\"swatch\" style=\"background:" + html.EscapeString(s) + "\" title=\"" + html.EscapeString(s) + "\"></span> "
	}
	return ""
}

func valueText(v any) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%g", f)
	}
	return fmt.Sprint(v)
}

// themeValueMatrix is the table of every theme's values, one column per theme.
func themeValueMatrix(def docPackage, themes []looksPkg, docs map[string]docPackage) string {
	cols := []string{"std/theme/default"}
	byCol := []docPackage{def}
	for _, p := range themes {
		cols = append(cols, p.Path)
		byCol = append(byCol, docs[p.Path])
	}
	var b strings.Builder
	b.WriteString("<div class=\"table-scroll\"><table class=\"theme-values\">\n<thead><tr><th>Name</th>")
	for _, c := range cols {
		b.WriteString("<th><code>" + html.EscapeString(c) + "</code></th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for i, v := range def.Values {
		fmt.Fprintf(&b, "<tr><th scope=\"row\"><code>%s</code></th>", html.EscapeString(v.Name))
		for _, col := range byCol {
			if i < len(col.Values) && col.Values[i].Name == v.Name {
				cv := col.Values[i]
				b.WriteString("<td>" + swatch(cv.Type, cv.Value) + "<code>" + html.EscapeString(valueText(cv.Value)) + "</code></td>")
			} else {
				b.WriteString("<td></td>")
			}
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table></div>\n")
	return b.String()
}

func buildLooksPage(c *Ctx, pkgs []looksPkg, files map[string]string, toks map[string][]Token, docs map[string]docPackage, def docPackage, assetName string) error {
	page := c.URL("looks/")
	esc := html.EscapeString
	var b strings.Builder
	var toc []TocItem
	b.WriteString("<h1>Looks &amp; Themes</h1>\n")
	b.WriteString("<p class=\"lead\">A <strong>look</strong> says what is drawn and how it moves. A <strong>theme</strong> says how it is skinned: colors, type, stroke, hand-drawn wobble, glow, node shape. Any look works with any theme, and neither changes the run.</p>\n")
	fmt.Fprintf(&b, "<p>Looks and themes beyond the defaults live in the module <code>github.com/synodlang/looks</code> (decision 122). A main file imports one look and at most one theme; a program that imports no look draws with <code>std/look/basic</code> and one that imports no theme uses <code>std/theme/default</code>. See <a href=\"%s\">the gallery</a> for all pairs playing at once.</p>\n", esc(c.URL("looks/gallery/")))
	b.WriteString("<pre class=\"synod\"><code>" + string(renderCode("# package main\nuse github.com/synodlang/examples/raft\nuse github.com/synodlang/looks/doodle\nuse github.com/synodlang/looks/sketch\nuse std/sim\n", nil, nil)) + "</code></pre>\n")
	fmt.Fprintf(&b, "<p>Fetch the module with <code>synod get</code> after adding <code>require github.com/synodlang/looks %s</code> to <code>synod.mod</code>. Source: <a href=\"https://github.com/synodlang/looks/tree/%s\">synodlang/looks</a>.</p>\n", esc(c.V.Looks.Tag), esc(c.V.Looks.Tag))

	var themes []looksPkg
	for _, p := range pkgs {
		if p.Theme {
			themes = append(themes, p)
		}
	}

	section := func(id, title, sub string) {
		toc = append(toc, TocItem{ID: id, Title: title, Level: 2})
		fmt.Fprintf(&b, "<h2 id=\"%s\">%s</h2>\n", id, title)
		if sub != "" {
			b.WriteString(sub)
		}
	}

	// Looks.
	section("looks", "Looks", "")
	fmt.Fprintf(&b, "<h3 id=\"std-look-basic\"><code>std/look/basic</code></h3>\n<p>The default look: cards, links and the messages that fly between them. Its source and every phrase it gives are in the Library: <a href=\"%s\"><code>std/look/basic</code></a>, <a href=\"%s\"><code>std/look</code></a>.</p>\n",
		esc(c.URL(libPageURL("std/look/basic"))), esc(c.URL(libPageURL("std/look"))))
	n := 0
	for _, p := range pkgs {
		if p.Theme {
			continue
		}
		toc = append(toc, TocItem{ID: "look-" + p.slug(), Title: p.Path, Level: 3})
		n++
		if err := looksPackageHTML(c, &b, p, files, toks, docs, assetName, n); err != nil {
			return err
		}
	}

	// Themes.
	section("themes", "Themes", "")
	fmt.Fprintf(&b, "<p>Every theme gives all %d names; to make your own, copy <code>std/theme/default</code> or one of these into a folder, set its first line to <code># theme &lt;folder name&gt;</code> and change the values. The Library has <a href=\"%s\"><code>std/theme</code></a> (the phrases a look reads) and <a href=\"%s\"><code>std/theme/default</code></a>.</p>\n",
		len(def.Values), esc(c.URL(libPageURL("std/theme"))), esc(c.URL(libPageURL("std/theme/default"))))
	toc = append(toc, TocItem{ID: "theme-values", Title: "Theme values compared", Level: 3})
	b.WriteString("<h3 id=\"theme-values\">Theme values compared</h3>\n" + themeValueMatrix(def, themes, docs))
	for _, p := range pkgs {
		if !p.Theme {
			continue
		}
		toc = append(toc, TocItem{ID: "theme-" + p.slug(), Title: p.Path, Level: 3})
		n++
		if err := looksPackageHTML(c, &b, p, files, toks, docs, assetName, n); err != nil {
			return err
		}
	}

	return c.AddPage(&Page{
		URL:         page,
		Title:       "Looks and Themes",
		Section:     "looks",
		Description: "The looks and themes of github.com/synodlang/looks and the std defaults, with source and an editable demo.",
		Body:        linkBody(c, page, template.HTML(b.String())),
		Toc:         toc,
	})
}

func looksPackageHTML(c *Ctx, b *strings.Builder, p looksPkg, files map[string]string, toks map[string][]Token, docs map[string]docPackage, assetName string, n int) error {
	esc := html.EscapeString
	kind := "look"
	if p.Theme {
		kind = "theme"
	}
	desc, err := mdRenderInline("looks/README.md", p.Desc, mdOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "<section class=\"looks-pkg\" id=\"%s-%s\">\n<h3><code>%s</code></h3>\n", kind, p.slug(), esc(p.Path))
	fmt.Fprintf(b, "<p>%s</p>\n", desc)
	if p.Lead != "" {
		fmt.Fprintf(b, "<p>%s</p>\n", esc(p.Lead))
	}
	fmt.Fprintf(b, "<p>Import: <code>use github.com/synodlang/looks/%s</code> · <a href=\"https://github.com/synodlang/looks/blob/%s/%s/%s.synod\">source on GitHub</a></p>\n",
		esc(p.Path), esc(c.V.Looks.Tag), esc(p.Path), esc(p.Name))
	srcName := p.Path + "/" + p.Name + ".synod"
	fmt.Fprintf(b, "<details class=\"looks-source\"><summary>Source of <code>%s</code></summary>\n<pre class=\"synod\" data-file=\"%s\"><code>%s</code></pre>\n</details>\n",
		esc(srcName), esc(srcName), renderCode(files[srcName], toks[srcName], nil))
	if pd, ok := docs[p.Path]; ok {
		b.WriteString("<details class=\"looks-values\"><summary>The " + fmt.Sprint(len(pd.Values)) + " values of this theme</summary>\n<table>\n<thead><tr><th>Name</th><th>Type</th><th>Value</th><th>Meaning</th></tr></thead>\n<tbody>\n")
		for _, v := range pd.Values {
			fmt.Fprintf(b, "<tr><td><code>%s</code></td><td>%s</td><td>%s<code>%s</code></td><td>%s</td></tr>\n",
				esc(v.Name), esc(v.Type), swatch(v.Type, v.Value), esc(valueText(v.Value)), esc(v.Doc))
		}
		b.WriteString("</tbody>\n</table>\n</details>\n")
	}
	demo := p.Path + "/demo.synod"
	d := ProgramData{
		ID: "demo-" + p.slug(), Program: p.Path, Op: "run",
		FilesURL: "assets/" + assetName, VendorURL: "assets/examples-" + c.V.Examples.Tag + ".json",
		VendorPrefix: "vendor/github.com/synodlang/examples@" + c.V.Examples.Tag + "/",
		Show:         []string{demo, srcName}, Predict: true,
	}
	for _, f := range d.Show {
		d.Code = append(d.Code, CodeFile{Name: f, HTML: renderCode(files[f], toks[f], nil)})
	}
	w, err := c.Layout.Program(d)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "<p>Edit the demo and press Run. Change the <code>use</code> lines to try another look or theme.</p>\n%s\n</section>\n", w)
	return nil
}

var _ = json.Marshal
