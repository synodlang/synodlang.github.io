package main

import (
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"
)

// explainDoc is synod-explain/1.
type explainDoc struct {
	Format string        `json:"format"`
	Codes  []explainCode `json:"codes"`
}

type explainCode struct {
	Code    string `json:"code"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
	Command string `json:"command"`
	Example struct {
		Files   map[string]string `json:"files"`
		Program string            `json:"program"`
	} `json:"example"`
	Diagnostic string `json:"diagnostic"`
	Fix        struct {
		Note    string            `json:"note"`
		Files   map[string]string `json:"files"`
		Deleted []string          `json:"deleted"`
	} `json:"fix"`
}

// errFiles returns the files of an entry's example. Decision 111: example.files
// holds every file the example is run with, synod.mod included.
func errFiles(e explainCode) map[string]string {
	files := map[string]string{}
	for p, t := range e.Example.Files {
		files[p] = t
	}
	return files
}

// errFixed returns the example with the fix applied.
func errFixed(e explainCode, files map[string]string) map[string]string {
	out := map[string]string{}
	for p, t := range files {
		out[p] = t
	}
	for _, p := range e.Fix.Deleted {
		delete(out, p)
	}
	for p, t := range e.Fix.Files {
		out[p] = t
	}
	return out
}

// errRun runs files natively with the entry's command and returns the
// output and the exit code.
func errRun(c *Ctx, e explainCode, files map[string]string) (out string, code int, err error) {
	err = WithFiles(files, func(dir string) error {
		var res Result
		var err error
		if e.Command == "run" {
			res, err = c.Synod.must(dir, []int{0, 1}, "run", e.Example.Program, "--style", "short")
		} else {
			res, err = c.Synod.must(dir, []int{0, 1}, "check", "--style", "short", ".")
		}
		out, code = res.Stdout+res.Stderr, res.Code
		return err
	})
	return
}

// errVerify checks that the example gives the entry's diagnostic and that
// the fix passes.
func errVerify(c *Ctx, e explainCode, files, fixed map[string]string) error {
	out, code, err := errRun(c, e, files)
	if err != nil {
		return fmt.Errorf("%s: %v", e.Code, err)
	}
	if code == 0 || !strings.Contains(out, "error["+e.Code+"]") {
		return fmt.Errorf("%s: the example does not give this code (exit %d):\n%s", e.Code, code, out)
	}
	if want := strings.TrimSpace(e.Diagnostic); !strings.Contains(out, want) {
		return fmt.Errorf("%s: the diagnostic differs from the explain entry:\nwant: %s\ngot: %s", e.Code, want, out)
	}
	out, code, err = errRun(c, e, fixed)
	if err != nil {
		return fmt.Errorf("%s: %v", e.Code, err)
	}
	if code != 0 {
		return fmt.Errorf("%s: the fix does not pass (exit %d):\n%s", e.Code, code, out)
	}
	return nil
}

func errTokens(c *Ctx, files map[string]string) (map[string][]Token, error) {
	var toks map[string][]Token
	err := WithFiles(files, func(dir string) error {
		var err error
		toks, err = moduleTokens(c, dir)
		return err
	})
	return toks, err
}

func errWidget(c *Ctx, id string, e explainCode, files map[string]string, changed map[string][]int) (template.HTML, error) {
	toks, err := errTokens(c, files)
	if err != nil {
		return "", err
	}
	d := ProgramData{ID: id, Files: files, Op: e.Command, Program: e.Example.Program, Predict: true}
	var names []string
	for p := range files {
		if strings.HasSuffix(p, ".synod") {
			names = append(names, p)
		}
	}
	sort.Strings(names)
	for _, p := range names {
		d.Show = append(d.Show, p)
		cf := CodeFile{Name: p, HTML: renderCode(files[p], toks[p], changed[p])}
		for i, n := range changed[p] {
			if i > 0 {
				cf.Changed += ","
			}
			cf.Changed += strconv.Itoa(n)
		}
		d.Code = append(d.Code, cf)
	}
	return c.Layout.Program(d)
}

func buildErrors(c *Ctx) error {
	raw, err := c.Synod.ExplainJSON()
	if err != nil {
		return err
	}
	var doc explainDoc
	if err := jsonDoc("synod explain --json", raw, &doc); err != nil {
		return err
	}
	if doc.Format != "synod-explain/1" || len(doc.Codes) == 0 {
		return fmt.Errorf("synod explain --json: unexpected document %q", doc.Format)
	}
	for i, e := range doc.Codes {
		url := c.URL("errors/" + e.Code + "/")
		files := errFiles(e)
		fixed := errFixed(e, files)
		if err := errVerify(c, e, files, fixed); err != nil {
			return err
		}
		opts := libMD(c, url)
		textDoc, err := parseMarkdown("explain:"+e.Code, []byte(e.Text))
		if err != nil {
			return err
		}
		text, err := textDoc.render(opts)
		if err != nil {
			return err
		}
		changed := map[string][]int{}
		for p, t := range fixed {
			if strings.HasSuffix(p, ".synod") {
				changed[p] = diffLines(files[p], t)
			}
		}
		w1, err := errWidget(c, "f1", e, files, nil)
		if err != nil {
			return fmt.Errorf("%s: %v", e.Code, err)
		}
		w2, err := errWidget(c, "f2", e, fixed, changed)
		if err != nil {
			return fmt.Errorf("%s: %v", e.Code, err)
		}
		verb := "Check"
		if e.Command == "run" {
			verb = "Run"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "<h1><code>%s</code></h1>\n<p class=\"lead\">%s</p>\n<p class=\"meta\">%s error, shown by <code>synod %s</code>.</p>\n",
			libEsc(e.Code), libEsc(e.Summary), libEsc(e.Kind), libEsc(e.Command))
		b.WriteString("<div class=\"explain\">\n" + string(text) + "\n</div>\n")
		fmt.Fprintf(&b, "<h2 id=\"example\">Example</h2>\n<p>%s this program to see the diagnostic.</p>\n%s\n", verb, w1)
		fmt.Fprintf(&b, "<h2 id=\"diagnostic\">Diagnostic</h2>\n<pre class=\"diagnostic\"><code>%s</code></pre>\n", libEsc(strings.TrimRight(e.Diagnostic, "\n")))
		fmt.Fprintf(&b, "<h2 id=\"fix\">Fix</h2>\n<p>%s</p>\n%s\n", libEsc(e.Fix.Note), w2)
		b.WriteString("<nav class=\"prevnext\">")
		if i > 0 {
			p := doc.Codes[i-1]
			fmt.Fprintf(&b, "<a rel=\"prev\" href=\"%s\">&larr; <code>%s</code></a> ", libEsc(c.URL("errors/"+p.Code+"/")), libEsc(p.Code))
		}
		fmt.Fprintf(&b, "<a href=\"%s\">All codes</a>", libEsc(c.URL("errors/")))
		if i+1 < len(doc.Codes) {
			n := doc.Codes[i+1]
			fmt.Fprintf(&b, " <a rel=\"next\" href=\"%s\"><code>%s</code> &rarr;</a>", libEsc(c.URL("errors/"+n.Code+"/")), libEsc(n.Code))
		}
		b.WriteString("</nav>\n")
		toc := []TocItem{{"example", "Example", 2}, {"diagnostic", "Diagnostic", 2}, {"fix", "Fix", 2}}
		if err := c.AddPage(&Page{URL: url, Title: e.Code, Section: "errors", Description: e.Summary, Body: linkBody(c, url, template.HTML(b.String())), Toc: toc}); err != nil {
			return err
		}
	}

	var b strings.Builder
	b.WriteString("<h1>Errors</h1>\n<p>Every diagnostic code of Synod has a page: what it means, a minimal program that triggers it, and the fix. The programs run in your browser.</p>\n")
	for _, kind := range []string{"compile", "run"} {
		title := "Compile-time codes"
		if kind == "run" {
			title = "Run-time codes"
		}
		fmt.Fprintf(&b, "<h2 id=\"%s\">%s</h2>\n<table>\n<thead><tr><th>Code</th><th>Meaning</th></tr></thead>\n<tbody>\n", kind, title)
		for _, e := range doc.Codes {
			if e.Kind == kind {
				fmt.Fprintf(&b, "<tr><td><a href=\"%s\"><code>%s</code></a></td><td>%s</td></tr>\n",
					libEsc(c.URL("errors/"+e.Code+"/")), libEsc(e.Code), libEsc(e.Summary))
			}
		}
		b.WriteString("</tbody>\n</table>\n")
	}
	return c.AddPage(&Page{URL: c.URL("errors/"), Title: "Errors", Section: "errors",
		Description: "Every diagnostic code with a program that triggers it and the fix.", Body: linkBody(c, c.URL("errors/"), template.HTML(b.String()))})
}
