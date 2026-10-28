package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// checkProblems collects what a check finds, so one run reports all of it.
type checkProblems struct{ list []string }

func (p *checkProblems) addf(format string, args ...any) {
	p.list = append(p.list, fmt.Sprintf(format, args...))
}

func (p *checkProblems) err() error {
	if len(p.list) == 0 {
		return nil
	}
	const show = 200
	lines := p.list
	more := ""
	if len(lines) > show {
		more = fmt.Sprintf("\n  ... and %d more", len(lines)-show)
		lines = lines[:show]
	}
	return fmt.Errorf("%d problem(s):\n  %s%s", len(p.list), strings.Join(lines, "\n  "), more)
}

// checkChapterCount is the number of tutorial chapters (spec 11-documentation).
const checkChapterCount = 15

// checkSite verifies a built site of one version (plan §3, check): internal
// hrefs and anchors resolve; Errors pages match spec/11 and explain; the
// tutorial, example and Library pages are complete; no gif, mp4, webm, mov,
// apng or <video>; install/ and install.sh byte-identical; asset hashes
// equal the pins. Checks of content the pinned dsl does not have yet (the
// tutorial, the Library example index) are skipped with a note.
func checkSite(c *Ctx) error {
	var p checkProblems
	if err := checkInstall(c.Root, c.Out); err != nil {
		p.addf("%v", err)
	}
	if err := checkAssets(c); err != nil {
		p.addf("%v", err)
	}
	docs := filepath.Join(c.Out, "docs", c.V.Docs)
	if _, err := os.Stat(docs); err != nil {
		p.addf("no docs for %s in %s", c.V.Docs, c.Out)
		return p.err()
	}
	note := func(format string, args ...any) {
		if c.Log != nil {
			fmt.Fprintf(c.Log, "check: "+format+"\n", args...)
		}
	}
	checkMedia(c.Out, &p)
	checkLinks(c.Out, c.V.Docs, c.IsLatest(), &p)
	checkWidgets(c.Out, docs, &p)
	checkErrorPages(c, docs, &p)
	checkTutorialPages(filepath.Join(c.Source("dsl"), "tutorial"), docs, &p, note)
	checkExamplePages(filepath.Join(c.Source("examples"), "README.md"), docs, &p)
	checkLibraryPages(c, docs, &p, note)
	checkFonts(c, docs, &p)
	checkLooksPages(c, docs, &p)
	return p.err()
}

// checkInstall requires out/install and out/install.sh to equal the sources.
func checkInstall(root, out string) error {
	files := []string{"install.sh"}
	entries, err := os.ReadDir(filepath.Join(root, "install"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		files = append(files, "install/"+e.Name())
	}
	for _, f := range files {
		same, err := sameFile(filepath.Join(root, f), filepath.Join(out, f))
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("%s in the site differs from the source", f)
		}
	}
	return nil
}

// checkAssets requires every browser asset in the site to hash to its pin.
func checkAssets(c *Ctx) error {
	for _, rel := range []Release{c.V.Compiler, c.V.Player} {
		for _, name := range sortedAssets(rel) {
			if hasArchiveSuffix(name) {
				continue
			}
			if c.IsLocal() && strings.Trim(rel.Assets[name], "0") == "" {
				continue // a placeholder pin: the asset came from a -local path
			}
			data, err := os.ReadFile(filepath.Join(c.Out, "docs", c.V.Docs, "assets", name))
			if err != nil {
				return err
			}
			if got := SumHex(data); got != rel.Assets[name] {
				return fmt.Errorf("asset %s: SHA-256 %s, pinned %s", name, got, rel.Assets[name])
			}
		}
	}
	return nil
}

func hasArchiveSuffix(name string) bool {
	n := len(name)
	return n > 7 && name[n-7:] == ".tar.gz" || n > 4 && name[n-4:] == ".zip"
}

// ---------------------------------------------------------------- media

var bannedMedia = map[string]bool{".gif": true, ".mp4": true, ".webm": true, ".mov": true, ".apng": true}

var videoTag = regexp.MustCompile(`(?i)<video\b`)

// checkMedia requires a site with no recordings (decision 42): no gif, mp4,
// webm, mov or apng file and no <video> element.
func checkMedia(out string, p *checkProblems) {
	checkWalkFiles(out, func(rel, abs string) {
		if bannedMedia[strings.ToLower(path.Ext(rel))] {
			p.addf("%s: recordings are not allowed (decision 42)", rel)
			return
		}
		if strings.HasSuffix(rel, ".html") {
			if data, err := os.ReadFile(abs); err == nil && videoTag.Match(data) {
				p.addf("%s: <video> is not allowed (decision 42)", rel)
			}
		}
	})
}

// checkWalkFiles calls fn for every regular file below root, in name order, with
// its slash path relative to root.
func checkWalkFiles(root string, fn func(rel, abs string)) {
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			abs := filepath.Join(dir, e.Name())
			if e.IsDir() {
				walk(abs)
				continue
			}
			rel, err := filepath.Rel(root, abs)
			if err == nil {
				fn(filepath.ToSlash(rel), abs)
			}
		}
	}
	walk(root)
}

// ---------------------------------------------------------------- links

var (
	checkCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	checkScriptRe  = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
	checkHrefRe    = regexp.MustCompile(`(?i)\b(?:href|src)\s*=\s*"([^"]*)"`)
	checkIDRe      = regexp.MustCompile(`(?i)\b(?:id|name)\s*=\s*"([^"]+)"`)
	refreshRe      = regexp.MustCompile(`(?i)http-equiv\s*=\s*"refresh"[^>]*content\s*=\s*"[^"]*?url=([^"]+)"`)
	checkSchemeRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	widgetOpen     = regexp.MustCompile(`<div\b[^>]*\bclass="synod-program"[^>]*>`)
	dataAttrRe     = regexp.MustCompile(`\bdata-([a-z-]+)="([^"]*)"`)
	jsonBlockRe    = regexp.MustCompile(`(?s)<script type="application/json" id="([^"]+)">(.*?)</script>`)
)

type htmlInfo struct {
	ids   map[string]bool
	links []string
}

func readHTMLInfo(abs string) (*htmlInfo, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	s := checkCommentRe.ReplaceAllString(string(data), "")
	info := &htmlInfo{ids: map[string]bool{}}
	for _, m := range refreshRe.FindAllStringSubmatch(s, -1) {
		info.links = append(info.links, m[1])
	}
	s = checkScriptRe.ReplaceAllString(s, "")
	for _, m := range checkIDRe.FindAllStringSubmatch(s, -1) {
		info.ids[html.UnescapeString(m[1])] = true
	}
	for _, m := range checkHrefRe.FindAllStringSubmatch(s, -1) {
		info.links = append(info.links, html.UnescapeString(m[1]))
	}
	return info, nil
}

// checkLinks requires every internal href, src and #anchor of the pages of
// docs version ver to resolve inside out. The pages outside /docs/ (home,
// tribute, install) belong to the latest version's run.
func checkLinks(out, ver string, latest bool, p *checkProblems) {
	cache := map[string]*htmlInfo{}
	info := func(abs string) *htmlInfo {
		if v, ok := cache[abs]; ok {
			return v
		}
		v, err := readHTMLInfo(abs)
		if err != nil {
			v = nil
		}
		cache[abs] = v
		return v
	}
	own := "docs/" + ver + "/"
	type broken struct {
		pages []string
	}
	found := map[string]*broken{}
	var order []string
	checkWalkFiles(out, func(rel, abs string) {
		if !strings.HasSuffix(rel, ".html") {
			return
		}
		switch {
		case strings.HasPrefix(rel, own):
		case strings.HasPrefix(rel, "docs/") && rel != "docs/index.html":
			return // another version
		case !latest:
			return
		}
		src := info(abs)
		if src == nil {
			return
		}
		seen := map[string]bool{}
		for _, href := range src.links {
			if seen[href] {
				continue
			}
			seen[href] = true
			if msg := resolveLink(out, rel, href, src, info); msg != "" {
				key := fmt.Sprintf("%q: %s", href, msg)
				if found[key] == nil {
					found[key] = &broken{}
					order = append(order, key)
				}
				found[key].pages = append(found[key].pages, rel)
			}
		}
	})
	// A link in the page shell breaks on every page; report it once.
	for _, key := range order {
		pages := found[key].pages
		if len(pages) == 1 {
			p.addf("%s: %s", pages[0], key)
			continue
		}
		p.addf("%s (on %d pages, e.g. %s)", key, len(pages), strings.Join(pages[:min(3, len(pages))], ", "))
	}
}

// resolveLink explains why href, written in the page at rel, does not resolve;
// "" when it does or when it is not internal.
func resolveLink(out, rel, href string, self *htmlInfo, info func(abs string) *htmlInfo) string {
	if href == "" || checkSchemeRe.MatchString(href) || strings.HasPrefix(href, "//") {
		return ""
	}
	target, frag, _ := strings.Cut(href, "#")
	target, _, _ = strings.Cut(target, "?")
	if f, err := url.PathUnescape(frag); err == nil {
		frag = f
	}
	var abs string
	var targetInfo *htmlInfo
	if target == "" {
		targetInfo = self
		abs = rel
	} else {
		if t, err := url.PathUnescape(target); err == nil {
			target = t
		}
		var site string
		if strings.HasPrefix(target, "/") {
			site = path.Clean(target)
		} else {
			site = path.Join(path.Dir("/"+rel), target)
		}
		if strings.HasSuffix(target, "/") || strings.HasSuffix(site, "/") {
			site = path.Join(site, "index.html")
		}
		abs = strings.TrimPrefix(site, "/")
		fs := filepath.Join(out, filepath.FromSlash(abs))
		st, err := os.Stat(fs)
		if err != nil {
			return "no such file in the site"
		}
		if st.IsDir() {
			abs = path.Join(abs, "index.html")
			fs = filepath.Join(out, filepath.FromSlash(abs))
			if _, err := os.Stat(fs); err != nil {
				return "folder has no index.html"
			}
		}
		if frag == "" || !strings.HasSuffix(abs, ".html") {
			return ""
		}
		targetInfo = info(fs)
	}
	if frag == "" || frag == "top" {
		return ""
	}
	if targetInfo == nil || !targetInfo.ids[frag] {
		return "no element #" + frag + " in " + abs
	}
	return ""
}

// ---------------------------------------------------------------- widgets

// siteWidget is one program widget of a page.
type siteWidget struct {
	Program, FilesID, FilesURL, Prefix, Op string
	VendorURL, VendorPrefix                string
}

// checkPageWidgets lists the program widgets of an HTML page, in order.
func checkPageWidgets(page string) []siteWidget {
	var out []siteWidget
	for _, tag := range widgetOpen.FindAllString(page, -1) {
		var w siteWidget
		for _, m := range dataAttrRe.FindAllStringSubmatch(tag, -1) {
			v := html.UnescapeString(m[2])
			switch m[1] {
			case "program":
				w.Program = v
			case "files":
				w.FilesID = strings.TrimPrefix(v, "#")
			case "files-url":
				w.FilesURL = v
			case "vendor-url":
				w.VendorURL = v
			case "vendor-prefix":
				w.VendorPrefix = v
			case "files-prefix":
				w.Prefix = v
			case "op":
				w.Op = v
			}
		}
		out = append(out, w)
	}
	return out
}

// readFilesURL reads the JSON file map a data-files-url or data-vendor-url
// names (assets/… is relative to the version, /… to the site, the rest to the page).
func readFilesURL(u, pageAbs, out, docs string) (map[string]string, error) {
	var file string
	switch {
	case strings.HasPrefix(u, "assets/"):
		file = filepath.Join(docs, filepath.FromSlash(u))
	case strings.HasPrefix(u, "/"):
		file = filepath.Join(out, filepath.FromSlash(u))
	default:
		file = filepath.Join(filepath.Dir(pageAbs), filepath.FromSlash(u))
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("files-url %s: %v", u, err)
	}
	var all map[string]string
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("files-url %s is not JSON: %v", u, err)
	}
	return all, nil
}

// widgetFiles returns the file paths a widget would run with.
func widgetFiles(w siteWidget, page, pageAbs, out, docs string) ([]string, error) {
	var all map[string]string
	switch {
	case w.FilesID != "":
		for _, m := range jsonBlockRe.FindAllStringSubmatch(page, -1) {
			if m[1] == w.FilesID {
				if err := json.Unmarshal([]byte(html.UnescapeString(m[2])), &all); err != nil {
					return nil, fmt.Errorf("files #%s are not JSON: %v", w.FilesID, err)
				}
			}
		}
		if all == nil {
			return nil, fmt.Errorf("no <script id=%q> with the files", w.FilesID)
		}
	case w.FilesURL != "":
		var err error
		if all, err = readFilesURL(w.FilesURL, pageAbs, out, docs); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("no files")
	}
	var names []string
	if w.VendorURL != "" { // the shared vendored module, merged under VendorPrefix
		shared, err := readFilesURL(w.VendorURL, pageAbs, out, docs)
		if err != nil {
			return nil, err
		}
		for k := range shared {
			if _, ok := all[w.VendorPrefix+k]; !ok {
				names = append(names, w.VendorPrefix+k)
			}
		}
	}
	for k := range all {
		if w.Prefix == "" || strings.HasPrefix(k, w.Prefix) || k == "synod.mod" || k == "synod.sum" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return names, nil
}

// checkWidgets requires each widget's data-program to be a folder, with a
// .synod file, in the files the widget carries.
func checkWidgets(out, docs string, p *checkProblems) {
	checkWalkFiles(docs, func(rel, abs string) {
		if !strings.HasSuffix(rel, ".html") {
			return
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return
		}
		page := string(data)
		for i, w := range checkPageWidgets(page) {
			where := fmt.Sprintf("%s: widget %d (%q)", rel, i+1, w.Program)
			if w.Program == "" && w.Op != "check" {
				p.addf("%s: no data-program", where)
				continue
			}
			names, err := widgetFiles(w, page, abs, out, docs)
			if err != nil {
				p.addf("%s: %v", where, err)
				continue
			}
			if w.Program == "" { // a Check of the whole module needs only files
				if len(names) == 0 {
					p.addf("%s: no files", where)
				}
				continue
			}
			if !checkHasFolder(names, w.Program) {
				p.addf("%s: its files hold no .synod file in folder %s/", where, w.Program)
			}
		}
	})
}

func checkHasFolder(names []string, folder string) bool {
	prefix := strings.TrimSuffix(strings.TrimPrefix(folder, "./"), "/") + "/"
	for _, n := range names {
		if strings.HasPrefix(n, prefix) && strings.HasSuffix(n, ".synod") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- sets

// checkPageDirs lists the folders of dir that hold an index.html.
func checkPageDirs(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "index.html")); err == nil {
				out = append(out, e.Name())
			}
		}
	}
	sort.Strings(out)
	return out
}

// checkSameSet reports, in one problem, how got differs from want.
func checkSameSet(p *checkProblems, what, gotName, wantName string, got, want []string) {
	g, w := map[string]bool{}, map[string]bool{}
	for _, s := range got {
		g[s] = true
	}
	for _, s := range want {
		w[s] = true
	}
	var missing, extra []string
	for s := range w {
		if !g[s] {
			missing = append(missing, s)
		}
	}
	for s := range g {
		if !w[s] {
			extra = append(extra, s)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		msg := fmt.Sprintf("%s: %s has %d, %s has %d", what, gotName, len(g), wantName, len(w))
		if len(missing) > 0 {
			msg += "; missing from " + gotName + ": " + strings.Join(missing, ", ")
		}
		if len(extra) > 0 {
			msg += "; not in " + wantName + ": " + strings.Join(extra, ", ")
		}
		p.addf("%s", msg)
	}
}

// ---------------------------------------------------------------- errors

var specCodeRe = regexp.MustCompile("`([a-z]+(?:-[a-z]+)*)`")

// checkSpecCodes lists the diagnostic codes of the tables of spec/11-errors.md from
// section 11.2 on (first column of each table row).
func checkSpecCodes(md string) []string {
	seen := map[string]bool{}
	var out []string
	on := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			on = !strings.HasPrefix(line, "## 11.1")
		}
		if !on || !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		for _, m := range specCodeRe.FindAllStringSubmatch(cells[1], -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// checkExplainCodes lists the codes of a synod-explain/1 document.
func checkExplainCodes(doc []byte) ([]string, error) {
	var v struct {
		Format string `json:"format"`
		Codes  []struct {
			Code string `json:"code"`
		} `json:"codes"`
	}
	if err := json.Unmarshal(doc, &v); err != nil {
		return nil, fmt.Errorf("synod explain --json: %v", err)
	}
	var out []string
	for _, c := range v.Codes {
		out = append(out, c.Code)
	}
	return out, nil
}

func checkErrorPages(c *Ctx, docs string, p *checkProblems) {
	pages := checkPageDirs(filepath.Join(docs, "errors"))
	spec, err := os.ReadFile(filepath.Join(c.Source("dsl"), "spec", "11-errors.md"))
	if err != nil {
		p.addf("errors: %v", err)
		return
	}
	doc, err := c.Synod.ExplainJSON()
	if err != nil {
		p.addf("errors: %v", err)
		return
	}
	explain, err := checkExplainCodes(doc)
	if err != nil {
		p.addf("errors: %v", err)
		return
	}
	checkSameSet(p, "Errors pages", "the site", "synod explain", pages, explain)
	checkSameSet(p, "Errors pages", "the site", "spec/11-errors.md", pages, checkSpecCodes(string(spec)))
}

// ---------------------------------------------------------------- tutorial

var (
	checkStepDirective = regexp.MustCompile(`(?m)^<!--\s*step\s+(\d+)\s+program=(\S+?)\s*-->\s*$`)
	checkChapterFile   = regexp.MustCompile(`^(\d\d-[a-z0-9-]+)\.md$`)
)

// checkTutorialChapters maps a chapter slug to the programs of its step directives.
func checkTutorialChapters(dir string) (map[string][]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, e := range entries {
		m := checkChapterFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		steps := []string{}
		for _, d := range checkStepDirective.FindAllStringSubmatch(string(data), -1) {
			if n, _ := strconv.Atoi(d[1]); n != len(steps)+1 {
				return nil, fmt.Errorf("%s: step %s follows step %d", e.Name(), d[1], len(steps))
			}
			steps = append(steps, d[2])
		}
		out[m[1]] = steps
	}
	return out, nil
}

// checkTutorialPages requires 15 chapter pages, each with a widget for every
// step directive of its source, when the pinned dsl has a tutorial/.
func checkTutorialPages(dir, docs string, p *checkProblems, note func(string, ...any)) {
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		note("tutorial: the pinned dsl has no tutorial/; tutorial pages not checked")
		return
	}
	chapters, err := checkTutorialChapters(dir)
	if err != nil {
		p.addf("tutorial: %v", err)
		return
	}
	if len(chapters) != checkChapterCount {
		p.addf("tutorial: dsl tutorial/ has %d chapters, expected %d", len(chapters), checkChapterCount)
	}
	var slugs []string
	for s := range chapters {
		slugs = append(slugs, s)
	}
	checkSameSet(p, "tutorial pages", "the site", "dsl tutorial/", checkPageDirs(filepath.Join(docs, "tutorial")), slugs)
	for _, slug := range slugs {
		data, err := os.ReadFile(filepath.Join(docs, "tutorial", slug, "index.html"))
		if err != nil {
			continue
		}
		var got []string
		for _, w := range checkPageWidgets(string(data)) {
			got = append(got, w.Program)
		}
		want := chapters[slug]
		if strings.Join(got, " ") != strings.Join(want, " ") {
			p.addf("tutorial/%s: %d widgets (%s), the chapter has %d steps (%s)", slug, len(got), strings.Join(got, " "), len(want), strings.Join(want, " "))
		}
	}
}

// ---------------------------------------------------------------- examples

var runFolderRe = regexp.MustCompile(`synod run \.?/?([A-Za-z0-9_-]+)`)

// checkExamplePrograms lists the programs of the table of examples/README.md: the
// folder after `synod run` in each row of the table headed "| Folder |".
func checkExamplePrograms(md string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(md, "\n") {
		switch {
		case strings.HasPrefix(line, "| Folder |"):
			in = true
		case in && strings.HasPrefix(line, "|"):
			if m := runFolderRe.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
		case in && !strings.HasPrefix(line, "|"):
			in = false
		}
	}
	return out
}

func checkExamplePages(readme, docs string, p *checkProblems) {
	data, err := os.ReadFile(readme)
	if err != nil {
		return // no examples source: nothing to compare
	}
	want := checkExamplePrograms(string(data))
	if len(want) == 0 {
		p.addf("examples: no table of programs in %s", readme)
		return
	}
	got := checkPageDirs(filepath.Join(docs, "examples"))
	checkSameSet(p, "example pages", "the site", "examples/README.md", got, want)
	for _, prog := range got {
		page, err := os.ReadFile(filepath.Join(docs, "examples", prog, "index.html"))
		if err != nil {
			continue
		}
		ok := false
		for _, w := range checkPageWidgets(string(page)) {
			ok = ok || w.Program == prog
		}
		if !ok {
			p.addf("examples/%s: no widget runs the program %s", prog, prog)
		}
	}
}

// ---------------------------------------------------------------- library

// checkLibraryIndex reads dsl/std/examples/README.md: pattern to example folders.
func checkLibraryIndex(md string) map[string][]string {
	out := map[string][]string{}
	for _, line := range strings.Split(md, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		pat, ex, ok := strings.Cut(strings.TrimPrefix(line, "| `"), "` | ")
		if !ok {
			continue
		}
		for _, m := range specCodePathRe.FindAllStringSubmatch(ex, -1) {
			out[pat] = append(out[pat], m[1])
		}
	}
	return out
}

var specCodePathRe = regexp.MustCompile("`([^`]+)`")

// checkLibrarySlug is the page folder of a std package: std/look/basic is std-look-basic.
func checkLibrarySlug(pkg string) string { return strings.ReplaceAll(pkg, "/", "-") }

// exportedPatterns lists the exported phrase patterns of a synod-doc/1 document.
func exportedPatterns(doc []byte) ([]string, error) {
	var v struct {
		Packages []struct {
			Phrases []struct {
				Pattern  string `json:"pattern"`
				Exported bool   `json:"exported"`
			} `json:"phrases"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(doc, &v); err != nil {
		return nil, err
	}
	var out []string
	for _, pk := range v.Packages {
		for _, ph := range pk.Phrases {
			if ph.Exported {
				out = append(out, ph.Pattern)
			}
		}
	}
	return out, nil
}

func checkLibraryPages(c *Ctx, docs string, p *checkProblems, note func(string, ...any)) {
	pkgs, err := c.Synod.StdPackages()
	if err != nil {
		p.addf("library: %v", err)
		return
	}
	var slugs []string
	for _, pkg := range pkgs {
		slugs = append(slugs, checkLibrarySlug(pkg))
	}
	checkSameSet(p, "Library pages", "the site", "synod doc", checkPageDirs(filepath.Join(docs, "library")), slugs)

	index, err := os.ReadFile(filepath.Join(c.Source("dsl"), "std", "examples", "README.md"))
	if err != nil {
		note("library: the pinned dsl has no std/examples/README.md; phrase examples not checked")
		return
	}
	byPattern := checkLibraryIndex(string(index))
	for _, pkg := range pkgs {
		doc, err := c.Synod.DocJSON(pkg)
		if err != nil {
			p.addf("library %s: %v", pkg, err)
			continue
		}
		patterns, err := exportedPatterns(doc)
		if err != nil {
			p.addf("library %s: %v", pkg, err)
			continue
		}
		examples := map[string]bool{}
		for _, pat := range patterns {
			if len(byPattern[pat]) == 0 {
				p.addf("library %s: no example uses %q (std/examples/README.md)", pkg, pat)
			}
			for _, ex := range byPattern[pat] {
				examples[ex] = true
			}
		}
		page, err := os.ReadFile(filepath.Join(docs, "library", checkLibrarySlug(pkg), "index.html"))
		if err != nil {
			continue
		}
		programs := map[string]bool{}
		for _, w := range checkPageWidgets(string(page)) {
			programs[w.Program] = true
		}
		for ex := range examples {
			if !programs[ex] && !programs[path.Base(ex)] {
				p.addf("library/%s: no widget runs the example %s", checkLibrarySlug(pkg), ex)
			}
		}
	}
}

// checkFonts requires the four font files and their licences in the assets,
// and the font base in the page metadata of a version that pins a fonts asset.
func checkFonts(c *Ctx, docs string, p *checkProblems) {
	for _, f := range []string{"Inter", "JetBrainsMono", "SourceSerif4", "ShantellSans"} {
		for _, n := range []string{f + ".woff2", "OFL-" + f + ".txt"} {
			if _, err := os.Stat(filepath.Join(docs, "assets", "fonts", n)); err != nil {
				p.addf("assets/fonts/%s is missing", n)
			}
		}
	}
	if c.V.FontsAsset() != "" {
		meta, _ := c.Meta()
		if !strings.Contains(meta, "fontBase") {
			p.addf("the page metadata has no fontBase")
		}
	}
}

// checkLooksPages checks the Looks & Themes page and the gallery of a
// version that pins the looks module: a demo widget for every package, every
// look x theme cell with its scene file, and the shared module assets.
func checkLooksPages(c *Ctx, docs string, p *checkProblems) {
	if !c.V.HasLooks() {
		return
	}
	page, err := os.ReadFile(filepath.Join(docs, "looks", "index.html"))
	if err != nil {
		p.addf("looks/: %v", err)
		return
	}
	ws := checkPageWidgets(string(page))
	if len(ws) < 7 {
		p.addf("looks/: %d demo widgets, expected one for each of the 7 packages", len(ws))
	}
	for _, f := range []string{"looks-" + c.V.Looks.Tag + ".json", "examples-" + c.V.Examples.Tag + ".json"} {
		if _, err := os.Stat(filepath.Join(docs, "assets", f)); err != nil {
			p.addf("assets/%s is missing", f)
		}
	}
	g, err := os.ReadFile(filepath.Join(docs, "looks", "gallery", "index.html"))
	if err != nil {
		p.addf("looks/gallery/: %v", err)
		return
	}
	cells := regexp.MustCompile(`data-scene="([^"]+)"`).FindAllStringSubmatch(string(g), -1)
	if want := len(galleryLooks) * len(galleryThemes); len(cells) != want {
		p.addf("looks/gallery/: %d cells, expected %d", len(cells), want)
	}
	for _, m := range cells {
		if _, err := os.Stat(filepath.Join(docs, filepath.FromSlash(m[1]))); err != nil {
			p.addf("looks/gallery/: scene %s is missing", m[1])
		}
	}
}
