package main

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// buildExamples builds examples/ and examples/<folder>/ from the examples
// checkout (plan §3, Examples): a row of examples/README.md gives Topic,
// What to watch and Seed; the folder's files and its module-local imports
// become editable tabs of a program widget. It writes
// assets/examples-<tag>.json (the module's synod.mod and *.synod files) with
// c.WriteDocs; every example is run natively and an unexpected exit fails
// the build.
func buildExamples(c *Ctx) error {
	src := c.Source("examples")
	files, err := exampleFiles(src)
	if err != nil {
		return err
	}
	mod, err := exampleModule(files)
	if err != nil {
		return err
	}
	readme, err := os.ReadFile(filepath.Join(src, "README.md"))
	if err != nil {
		return err
	}
	rows, err := exampleRows(string(readme))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("examples/README.md has no example table")
	}

	data, err := json.Marshal(files)
	if err != nil {
		return err
	}
	assetName := "examples-" + c.V.Examples.Tag + ".json"
	if err := c.WriteDocs("assets/"+assetName, data); err != nil {
		return err
	}

	var toks map[string][]Token
	err = WithCopy(src, func(dir string) error {
		var e error
		toks, e = moduleTokens(c, dir)
		return e
	})
	if err != nil {
		return err
	}

	for i := range rows {
		r := &rows[i]
		if _, ok := files[r.Folder+"/demo.synod"]; !ok {
			return fmt.Errorf("examples/README.md: %s has no demo.synod", r.Folder)
		}
		r.Show = exampleShow(files, mod, r.Folder)
		if err := runExample(c, src, r.Folder); err != nil {
			return err
		}
		body, err := exampleBody(c, r, files, toks, assetName)
		if err != nil {
			return fmt.Errorf("examples/%s: %v", r.Folder, err)
		}
		if err := c.AddPage(&Page{
			URL:         c.URL("examples/" + r.Folder + "/"),
			Title:       r.Title(),
			Section:     "examples",
			Description: plainText(r.Topic),
			Body:        body,
		}); err != nil {
			return err
		}
	}
	return c.AddPage(&Page{
		URL:         c.URL("examples/"),
		Title:       "Examples",
		Section:     "examples",
		Description: "Protocols and systems written in Synod that you can run, edit and break.",
		Body:        examplesIndex(c, rows),
	})
}

// exampleRow is one row of the table in examples/README.md.
type exampleRow struct {
	Folder string   // the program folder, "raft"
	Topic  string   // Markdown
	Watch  string   // Markdown
	Seed   string   // Markdown
	Show   []string // files shown as tabs
}

// Title is the page title: the folder and its topic.
func (r exampleRow) Title() string { return r.Folder + ": " + plainText(r.Topic) }

var (
	exampleFolderRe = regexp.MustCompile("^`([a-z0-9]+)`")
	exampleUseRe    = regexp.MustCompile(`(?m)^use\s+(\S+)`)
	exampleModRe    = regexp.MustCompile(`(?m)^module\s+(\S+)`)
)

// exampleRows reads the example table: rows whose first cell starts with a
// code span (the program folder); a look package named after "+" is not a
// row of its own.
func exampleRows(readme string) ([]exampleRow, error) {
	var rows []exampleRow
	for _, line := range strings.Split(readme, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) < 4 {
			continue
		}
		m := exampleFolderRe.FindStringSubmatch(strings.TrimSpace(cells[0]))
		if m == nil {
			continue
		}
		rows = append(rows, exampleRow{
			Folder: m[1],
			Topic:  strings.TrimSpace(cells[1]),
			Watch:  strings.TrimSpace(cells[2]),
			Seed:   strings.TrimSpace(cells[3]),
		})
	}
	return rows, nil
}

// exampleFiles reads synod.mod and every .synod file of the checkout.
func exampleFiles(src string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "out" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "synod.mod" && !strings.HasSuffix(rel, ".synod") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[rel] = string(b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := files["synod.mod"]; !ok {
		return nil, fmt.Errorf("%s has no synod.mod", src)
	}
	return files, nil
}

func exampleModule(files map[string]string) (string, error) {
	m := exampleModRe.FindStringSubmatch(files["synod.mod"])
	if m == nil {
		return "", fmt.Errorf("examples synod.mod has no module line")
	}
	return m[1], nil
}

// exampleShow lists the files of the program folder (demo.synod first) and
// then those of the module-local packages it imports, transitively (their
// own demo.synod files are other programs and stay out).
func exampleShow(files map[string]string, mod, folder string) []string {
	var show []string
	seen := map[string]bool{}
	var visit func(f string)
	visit = func(f string) {
		if seen[f] {
			return
		}
		seen[f] = true
		var names []string
		for p := range files {
			if strings.HasSuffix(p, ".synod") && filepath.ToSlash(filepath.Dir(p)) == f {
				// another program's main file is not part of this one
				if f != folder && strings.HasSuffix(p, "/demo.synod") {
					continue
				}
				names = append(names, p)
			}
		}
		sort.Slice(names, func(i, j int) bool {
			di, dj := strings.HasSuffix(names[i], "/demo.synod"), strings.HasSuffix(names[j], "/demo.synod")
			if di != dj {
				return di
			}
			return names[i] < names[j]
		})
		show = append(show, names...)
		for _, n := range names {
			for _, m := range exampleUseRe.FindAllStringSubmatch(files[n], -1) {
				if strings.HasPrefix(m[1], mod+"/") {
					visit(strings.TrimPrefix(m[1], mod+"/"))
				}
			}
		}
	}
	visit(folder)
	return show
}

// runExample runs the program natively in a copy of the checkout; the run
// must end normally (decision 93: nothing shown here can drift).
func runExample(c *Ctx, src, folder string) error {
	return WithCopy(src, func(dir string) error {
		res, err := c.Synod.RunSummary(dir, folder)
		if err != nil {
			return fmt.Errorf("examples/%s: %v", folder, err)
		}
		if res.Code != 0 {
			return fmt.Errorf("examples/%s: synod run exited %d: %s", folder, res.Code, strings.TrimSpace(res.Stderr+res.Stdout))
		}
		return nil
	})
}

func exampleBody(c *Ctx, r *exampleRow, files map[string]string, toks map[string][]Token, assetName string) (template.HTML, error) {
	var code []CodeFile
	for _, f := range r.Show {
		code = append(code, CodeFile{Name: f, HTML: renderCode(files[f], toks[f], nil)})
	}
	widget, err := c.Layout.Program(ProgramData{
		ID: "ex-" + r.Folder, Program: r.Folder, Op: "run",
		FilesURL: "assets/" + assetName, Show: r.Show, Predict: true, Code: code,
	})
	if err != nil {
		return "", err
	}
	topic, err := mdRenderInline("examples/README.md", r.Topic, mdOptions{})
	if err != nil {
		return "", err
	}
	watch, err := mdRenderInline("examples/README.md", r.Watch, mdOptions{})
	if err != nil {
		return "", err
	}
	seed, err := mdRenderInline("examples/README.md", r.Seed, mdOptions{})
	if err != nil {
		return "", err
	}
	repo := "https://github.com/synodlang/examples/tree/" + c.V.Examples.Tag + "/" + r.Folder
	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(r.Folder))
	fmt.Fprintf(&b, "<p class=\"lead\">%s</p>\n", topic)
	fmt.Fprintf(&b, "<h2 id=\"what-to-watch\">What to watch</h2>\n<p>%s</p>\n", watch)
	fmt.Fprintf(&b, "<p>Seed: %s. The program asks a question at the interesting moment; with Predict on, the run stops there so you can answer before you see what happens.</p>\n", seed)
	fmt.Fprintf(&b, "<h2 id=\"run-it\">Run it</h2>\n<p>Edit any file and press Run. The tabs hold the program and the packages it uses.</p>\n%s\n", widget)
	fmt.Fprintf(&b, "<p>On your own machine: <code>synod run ./%s</code>. <a href=\"%s\">Source on GitHub</a> · <a href=\"%s\">all examples</a></p>\n",
		html.EscapeString(r.Folder), html.EscapeString(repo), html.EscapeString(c.URL("examples/")))
	return template.HTML(b.String()), nil
}

func examplesIndex(c *Ctx, rows []exampleRow) template.HTML {
	var b strings.Builder
	b.WriteString("<h1>Examples</h1>\n<p class=\"lead\">Protocols and systems written in Synod. Each page holds the real source: run it in the browser, change it, and break it.</p>\n")
	b.WriteString("<table>\n<thead><tr><th>Example</th><th>Topic</th><th>What to watch</th></tr></thead>\n<tbody>\n")
	for _, r := range rows {
		topic, _ := mdRenderInline("examples/README.md", r.Topic, mdOptions{})
		watch, _ := mdRenderInline("examples/README.md", r.Watch, mdOptions{})
		fmt.Fprintf(&b, "<tr><td><a href=\"%s\"><code>%s</code></a></td><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(c.URL("examples/"+r.Folder+"/")), html.EscapeString(r.Folder), topic, watch)
	}
	b.WriteString("</tbody>\n</table>\n")
	fmt.Fprintf(&b, "<p>The programs are the acceptance tests of the language; they live in <a href=\"https://github.com/synodlang/examples/tree/%s\">synodlang/examples</a>.</p>\n", html.EscapeString(c.V.Examples.Tag))
	return template.HTML(b.String())
}

// plainText strips Markdown code marks and emphasis from a table cell.
func plainText(s string) string {
	return strings.NewReplacer("`", "", "**", "", "*", "").Replace(s)
}
