package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"path/filepath"
)

// Layout renders pages into the site shell with the templates in templates/.
type Layout struct {
	t *template.Template
}

// LoadLayout parses templates/*.html in dir.
func LoadLayout(dir string) (*Layout, error) {
	t, err := template.ParseGlob(filepath.Join(dir, "*.html"))
	if err != nil {
		return nil, err
	}
	return &Layout{t: t}, nil
}

// navItems are the header links, in order: key, label, docs-relative path.
var navItems = []struct{ Key, Label, Path string }{
	{"tutorial", "Tutorial", "tutorial/"},
	{"reference", "Reference", "reference/"},
	{"library", "Library", "library/"},
	{"examples", "Examples", "examples/"},
	{"looks", "Looks", "looks/"}, // only for versions that pin the looks module
	{"guides", "Guides", "guides/"},
	{"errors", "Errors", "errors/"},
	{"design", "Design", "design/"},
}

type navLink struct {
	Label, URL string
	Active     bool
}

// shell is the data of layout.html.
type shell struct {
	Page    *Page
	Content template.HTML
	Meta    string
	Nav     []navLink
	Docs    string // version being shown
	Latest  string
	DocsURL string
	Assets  string
	Home    string // where the logo leads
	Local   bool
	Release string // footer: pinned versions
}

// Render wraps p in the shell. Pages of the home page (URL "/") show the
// latest version's navigation.
func (l *Layout) Render(c *Ctx, p *Page) ([]byte, error) {
	meta, err := c.Meta()
	if err != nil {
		return nil, err
	}
	s := shell{
		Page: p, Meta: meta, Docs: c.V.Docs, Latest: c.Deps.Latest, DocsURL: c.DocsURL(),
		Assets: c.AssetsURL(), Home: "/", Local: c.IsLocal(),
		Release: "dsl " + c.V.DSL.Tag + ", compiler " + c.V.Compiler.Tag + ", std " + c.V.Std.Tag + ", examples " + c.V.Examples.Tag + looksPin(c.V),
	}
	for _, n := range navItems {
		if n.Key == "looks" && !c.V.HasLooks() {
			continue
		}
		s.Nav = append(s.Nav, navLink{n.Label, c.URL(n.Path), n.Key == p.Section})
	}
	var body bytes.Buffer
	name := "page"
	if p.Raw {
		name = "raw"
	}
	if err := l.t.ExecuteTemplate(&body, name, p); err != nil {
		return nil, err
	}
	s.Content = template.HTML(body.String())
	var out bytes.Buffer
	if err := l.t.ExecuteTemplate(&out, "layout", s); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ProgramData describes one program widget (plan §3, widget contract).
type ProgramData struct {
	ID           string            // unique on the page, "f1"
	Program      string            // folder to run or check, "raft"
	Op           string            // "run" or "check"
	Files        map[string]string // inline files: slash path to text; empty when FilesURL is set
	FilesURL     string            // assets/examples-v0.2.1.json, instead of inline Files
	FilesPrefix  string            // path prefix of the program inside the files
	VendorURL    string            // shared JSON merged into the files under VendorPrefix
	VendorPrefix string            // vendor/github.com/synodlang/examples@v0.2.1/
	Show         []string          // files shown as tabs, in order
	Predict      bool
	Code         []CodeFile // highlighted views of the shown files
}

// CodeFile is one highlighted file of a widget.
type CodeFile struct {
	Name    string
	HTML    template.HTML // token spans, from tokens.go
	Changed string        // "3,4": lines that differ from the previous step
}

// programView adds what the template cannot compute.
type programView struct {
	ProgramData
	FilesJSON template.JS
	ShowList  string
}

// Program renders a program widget.
func (l *Layout) Program(d ProgramData) (template.HTML, error) {
	v := programView{ProgramData: d}
	if len(d.Files) > 0 {
		data, err := json.Marshal(d.Files) // escapes <, > and & as \u00XX
		if err != nil {
			return "", err
		}
		v.FilesJSON = template.JS(data)
	}
	for i, s := range d.Show {
		if i > 0 {
			v.ShowList += " "
		}
		v.ShowList += s
	}
	var b bytes.Buffer
	if err := l.t.ExecuteTemplate(&b, "program", v); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func looksPin(v *Version) string {
	if v.Looks == nil {
		return ""
	}
	return ", looks " + v.Looks.Tag
}
