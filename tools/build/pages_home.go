package main

import (
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// homeScenePace speeds the Raft run up so that one loop lasts under a minute.
const homeScenePace = "3"

// buildHome builds the home page "/" (latest version only), the scene
// assets/home-raft.json it plays, and the docs home of every version from
// content/docs-home.md. It makes the scene with the native `synod run raft
// --scene` of the examples checkout (decision 110).
func buildHome(c *Ctx) error {
	if c.IsLatest() {
		if err := buildHomePage(c); err != nil {
			return err
		}
	}
	return buildDocsHome(c)
}

func buildHomePage(c *Ctx) error {
	var scene []byte
	err := WithCopy(c.Source("examples"), func(dir string) error {
		out := filepath.Join(dir, "home-raft.json")
		if err := c.Synod.RunScene(dir, "raft", out, "--pace", homeScenePace); err != nil {
			return fmt.Errorf("home scene: %v", err)
		}
		var e error
		scene, e = os.ReadFile(out)
		return e
	})
	if err != nil {
		return err
	}
	if err := c.WriteDocs("assets/home-raft.json", scene); err != nil {
		return err
	}
	logo, err := os.ReadFile(filepath.Join(c.Source("dsl"), "assets", "logo-512.png"))
	if err != nil {
		return err
	}
	if err := c.WriteDocs("assets/logo-512.png", logo); err != nil {
		return err
	}
	player, err := c.V.PlayerAsset()
	if err != nil {
		return err
	}
	lang := strings.TrimPrefix(c.V.Docs, "v")
	esc := html.EscapeString
	body := fmt.Sprintf(homeHTML,
		esc(c.AssetsURL()+"logo-512.png"),
		esc(c.URL("tutorial/")), esc("/install/"), esc(c.URL("reference/")),
		esc(c.URL("examples/")), esc(lang),
		esc(c.AssetsURL()+"home-raft.json"), esc(c.AssetsURL()+player))
	return c.AddPage(&Page{
		URL:         "/",
		Title:       "",
		Description: "A small language for watching distributed systems think.",
		Body:        template.HTML(body),
		Raw:         true,
	})
}

// homeHTML takes: logo, tutorial, install, reference, examples URLs, the
// language version, the scene URL and the player URL.
const homeHTML = `<style>
.home{max-width:60rem;margin:0 auto;padding:2.5rem 16px 3rem;text-align:center}
.home img{width:96px;height:96px;display:block;margin:0 auto 1rem}
.home h1{margin:0 0 .5rem;font-size:2.25rem;color:var(--accent)}
.home .lead{margin:0 auto 1.5rem;max-width:34rem;font-size:1.15rem}
.home .actions{display:flex;flex-wrap:wrap;gap:.75rem;justify-content:center;margin:0 0 2rem}
.home .actions a{display:inline-block;padding:.6rem 1.1rem;border:2px solid var(--accent);border-radius:6px;font-weight:700;text-decoration:none;color:var(--accent)}
.home .actions a.primary{background:var(--accent);color:var(--bg)}
.home .actions a:focus-visible{outline:2px solid var(--focus);outline-offset:3px}
.home canvas{display:block;width:100%%;aspect-ratio:16/10;background:#151a2e;border-radius:8px;border:1px solid var(--line)}
.home .meta{margin:1.5rem 0 0;color:var(--muted);font-size:.9rem}
</style>
<div class="home">
<img src="%s" width="96" height="96" alt="Synod logo">
<h1>Synod</h1>
<p class="lead">A small language for watching distributed systems think.</p>
<p class="actions">
<a class="primary" href="%s">Start the tutorial</a>
<a href="%s">Install</a>
<a href="%s">Read the reference</a>
</p>
<canvas id="home-raft" width="1600" height="1000" role="img" aria-label="A Raft cluster elects a leader, loses it, and elects another"></canvas>
<p class="meta">A Raft election, run by Synod and drawn live. <a href="%s">More examples</a> · Implements Synod %s · <a href="https://github.com/synodlang">GitHub</a></p>
</div>
<script type="module">
import { createPlayer } from "%[8]s";
const canvas = document.getElementById("home-raft");
const reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
fetch("%[7]s").then((r) => r.text()).then((scene) => {
  createPlayer(canvas, scene, { autoplay: !reduce, loop: true });
}).catch(() => {});
</script>`

func buildDocsHome(c *Ctx) error {
	intro, err := renderContent(c, "docs-home.md")
	if err != nil {
		return err
	}
	cards := []struct{ Title, Path, Text string }{
		{"Tutorial", "tutorial/", "Fifteen short chapters, from one sentence to a working Raft election."},
		{"Reference", "reference/", "The specification of the language, one page per chapter."},
		{"Library", "library/", "Every standard package and phrase, each with a runnable example."},
		{"Examples", "examples/", "Raft, Paxos, two-phase commit and more. Run them, edit them, break them."},
		{"Guides", "guides/", "Install Synod, write a look, publish a package, use agents."},
		{"Errors", "errors/", "Every diagnostic code: what it means, a program that shows it, and the fix."},
		{"Design", "design/", "The principles behind the language and the decisions that shaped it."},
		{"Tribute", "/tribute/", "Leslie Lamport's Part-Time Parliament and why we call it Synod."},
	}
	var b strings.Builder
	b.WriteString(string(intro))
	b.WriteString("\n<ul class=\"cards\">\n")
	for _, k := range cards {
		url := c.URL(k.Path)
		if strings.HasPrefix(k.Path, "/") {
			url = k.Path
		}
		fmt.Fprintf(&b, "<li><h2><a href=\"%s\">%s</a></h2><p>%s</p></li>\n",
			html.EscapeString(url), html.EscapeString(k.Title), html.EscapeString(k.Text))
	}
	b.WriteString("</ul>\n")
	return c.AddPage(&Page{
		URL:         c.DocsURL(),
		Title:       "Documentation",
		Description: "The Synod documentation: tutorial, reference, library, examples, guides, errors and design.",
		Body:        template.HTML(b.String()),
	})
}
