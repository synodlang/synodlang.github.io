package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Ctx is what a page builder gets: one docs version of the site being built.
// Builders read sources through Source and Synod, add pages through AddPage
// and write files through WriteDocs; they never touch Out directly.
type Ctx struct {
	Root   string            // repository root: templates/, web/, install/
	Out    string            // output directory, _site
	Deps   *Deps             // all of deps.json
	V      *Version          // the docs version being built
	Local  map[string]string // -local overrides (dsl, std, examples, looks, synod, wasm, wasmexec, player), absolute; latest version only
	Synod  *Synod            // the pinned native compiler (or the -local one)
	Fetch  *Fetcher          // where the pinned dependencies live
	Layout *Layout
	Pages  *Registry
	Log    io.Writer
}

// NewCtx makes the context of docs version v.
func NewCtx(root, out string, deps *Deps, v *Version, local map[string]string, fetch *Fetcher, log io.Writer) (*Ctx, error) {
	// Commands run with their own working directory, so every path the
	// context hands out is absolute.
	var err error
	for _, p := range []*string{&root, &out, &fetch.Dir} {
		if *p, err = filepath.Abs(*p); err != nil {
			return nil, err
		}
	}
	l, err := LoadLayout(filepath.Join(root, "templates"))
	if err != nil {
		return nil, err
	}
	c := &Ctx{Root: root, Out: out, Deps: deps, V: v, Local: local, Fetch: fetch, Layout: l, Pages: &Registry{}, Log: log}
	bin := fetch.SynodPath(v)
	if p, ok := local["synod"]; ok {
		bin = p
	}
	if bin, err = filepath.Abs(bin); err != nil {
		return nil, err
	}
	c.Synod = &Synod{Bin: bin}
	return c, nil
}

// IsLocal reports whether any source is a -local override; pages then show
// the "local build" banner.
func (c *Ctx) IsLocal() bool { return len(c.Local) > 0 }

// IsLatest reports whether this version is the one /docs/ and / lead to.
func (c *Ctx) IsLatest() bool { return c.V.Docs == c.Deps.Latest }

// Source is the directory of a pinned repository ("dsl", "std", "examples"),
// or the -local override.
func (c *Ctx) Source(name string) string {
	if p, ok := c.Local[name]; ok {
		return p
	}
	return c.Fetch.RepoPath(c.V, name)
}

// AssetData reads a pinned release asset: the verified download, or for a
// -local build the file or folder entry that replaces it (wasm, wasmexec,
// player).
func (c *Ctx) AssetData(name string) ([]byte, error) {
	var local string
	switch {
	case strings.HasSuffix(name, "-wasm_exec.js"):
		local = c.Local["wasmexec"]
	case strings.HasSuffix(name, ".wasm"):
		local = c.Local["wasm"]
	case strings.HasPrefix(name, "synod-player-fonts-"):
		if d := c.Local["player"]; d != "" {
			local = filepath.Join(d, "synod-player-fonts.tar.gz")
		}
	case strings.HasPrefix(name, "synod-player-") && strings.HasSuffix(name, ".js"):
		if d := c.Local["player"]; d != "" {
			local = filepath.Join(d, "synod-player.js")
		}
	case strings.HasPrefix(name, "synod-player-") && strings.HasSuffix(name, ".d.ts"):
		if d := c.Local["player"]; d != "" {
			local = filepath.Join(d, "synod-player.d.ts")
		}
	}
	if local != "" {
		return os.ReadFile(local)
	}
	data, err := os.ReadFile(c.Fetch.AssetPath(c.V, name))
	if err != nil {
		return nil, fmt.Errorf("%v (run `go run ./tools/build fetch`)", err)
	}
	return data, nil
}

// DocsURL is the site path of this version, ending in a slash.
func (c *Ctx) DocsURL() string { return "/docs/" + c.V.Docs + "/" }

// URL is the site path of the docs-relative path rel ("tutorial/01-hello/").
func (c *Ctx) URL(rel string) string { return c.DocsURL() + strings.TrimPrefix(rel, "/") }

// AssetsURL is where the version's browser assets are served.
func (c *Ctx) AssetsURL() string { return c.URL("assets/") }

// WriteDocs writes data to out/docs/<version>/<rel>.
func (c *Ctx) WriteDocs(rel string, data []byte) error {
	return c.WriteSite(path.Join("docs", c.V.Docs, rel), data)
}

// WriteSite writes data to out/<rel>; rel is a slash path inside Out.
func (c *Ctx) WriteSite(rel string, data []byte) error {
	clean := path.Clean("/" + rel)
	if strings.Contains(rel, "..") {
		return fmt.Errorf("output path %q leaves the output directory", rel)
	}
	return writeFile(filepath.Join(c.Out, filepath.FromSlash(clean)), data, 0o644)
}

// AddPage renders p into the shell, writes it to <URL>index.html and
// registers it. A second page with the same URL is an error.
func (c *Ctx) AddPage(p *Page) error {
	if !strings.HasPrefix(p.URL, "/") || !strings.HasSuffix(p.URL, "/") {
		return fmt.Errorf("page URL %q must start and end with a slash", p.URL)
	}
	if err := c.Pages.Add(p); err != nil {
		return err
	}
	html, err := c.Layout.Render(c, p)
	if err != nil {
		return fmt.Errorf("%s: %v", p.URL, err)
	}
	return c.WriteSite(path.Join(p.URL, "index.html"), html)
}

// Meta is the content of <meta name="synod">: where the page widgets find
// the wasm, the player and the Errors pages (plan §3, widget contract).
func (c *Ctx) Meta() (string, error) {
	var m struct {
		Assets   string `json:"assets"`
		Wasm     string `json:"wasm"`
		WasmExec string `json:"wasmExec"`
		Player   string `json:"player"`
		Errors   string `json:"errors"`
		FontBase string `json:"fontBase,omitempty"`
	}
	var err error
	m.Assets = c.AssetsURL()
	m.Errors = c.URL("errors/")
	if c.V.FontsAsset() != "" {
		m.FontBase = c.AssetsURL() + "fonts/" // web/fonts, compared with the player fonts asset
	}
	for _, a := range []struct {
		dst *string
		get func() (string, error)
	}{{&m.Wasm, c.V.WasmAsset}, {&m.WasmExec, c.V.WasmExecAsset}, {&m.Player, c.V.PlayerAsset}} {
		if *a.dst, err = a.get(); err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(m)
	return string(data), err
}

// Page is one generated HTML page.
type Page struct {
	URL         string // site path, "/docs/v0.2/tutorial/01-hello/"; "/" for the home page
	Title       string
	Section     string // nav key: tutorial, reference, library, examples, guides, errors, design, or ""
	Description string
	Body        template.HTML // trusted HTML, built by the section
	Raw         bool          // true: Body is the whole <main>, no article wrapper
	Toc         []TocItem     // optional table of contents, shown beside the article
}

// TocItem is one entry of a page's table of contents.
type TocItem struct {
	ID    string
	Title string
	Level int // 2 or 3
}

// Registry holds every page of one version, in the order added.
type Registry struct {
	pages []*Page
	byURL map[string]*Page
}

// Add registers p.
func (r *Registry) Add(p *Page) error {
	if r.byURL == nil {
		r.byURL = map[string]*Page{}
	}
	if _, dup := r.byURL[p.URL]; dup {
		return fmt.Errorf("page %s is built twice", p.URL)
	}
	r.byURL[p.URL] = p
	r.pages = append(r.pages, p)
	return nil
}

// Get returns the page at url.
func (r *Registry) Get(url string) (*Page, bool) {
	p, ok := r.byURL[url]
	return p, ok
}

// All lists the pages sorted by URL.
func (r *Registry) All() []*Page {
	out := append([]*Page(nil), r.pages...)
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

// removeOut empties dir for a fresh build, refusing anything that is not a
// plain build directory.
func removeOut(root, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rootAbs, _ := filepath.Abs(root)
	if abs == rootAbs || abs == "/" || strings.HasPrefix(rootAbs, abs+string(filepath.Separator)) {
		return fmt.Errorf("refusing to clear %s", dir)
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
		return fmt.Errorf("refusing to clear %s: it holds a go.mod", dir)
	}
	return os.RemoveAll(abs)
}
