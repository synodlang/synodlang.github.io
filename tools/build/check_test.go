package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// site writes files (slash path to text) under a fresh directory.
func siteFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustHave(t *testing.T, p *checkProblems, want ...string) {
	t.Helper()
	all := strings.Join(p.list, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("problems lack %q:\n%s", w, all)
		}
	}
}

func mustBeClean(t *testing.T, p *checkProblems) {
	t.Helper()
	if len(p.list) > 0 {
		t.Errorf("unexpected problems:\n%s", strings.Join(p.list, "\n"))
	}
}

const page = `<!doctype html><title>x</title><body><h2 id="a">A</h2>%s</body>`

func TestCheckLinks(t *testing.T) {
	good := siteFiles(t, map[string]string{
		"index.html":               `<a href="/docs/v0.2/x/">x</a><a href="https://example.com/nope">ext</a>`,
		"docs/index.html":          `<meta http-equiv="refresh" content="0; url=/docs/v0.2/"><a href="/docs/v0.2/">d</a>`,
		"docs/v0.2/index.html":     `<a href="x/#a">x</a><a href="x/y/">y</a><a href="assets/a.js">js</a><a href="#top">t</a>`,
		"docs/v0.2/x/index.html":   `<h2 id="a">A</h2><a href="#a">self</a><a href="../">up</a><a href="y/#b">y</a><script>href="/nope/"</script><!-- <a href="/gone/"> -->`,
		"docs/v0.2/x/y/index.html": `<p id="b">B</p><a href="/docs/v0.1/old/">other version</a>`,
		"docs/v0.2/assets/a.js":    "x",
		"docs/v0.1/old/index.html": `<a href="/missing/">not checked</a>`,
	})
	var p checkProblems
	checkLinks(good, "v0.2", true, &p)
	mustBeClean(t, &p)

	bad := siteFiles(t, map[string]string{
		"index.html":             `<a href="/docs/v0.2/nowhere/">x</a>`,
		"docs/index.html":        `<a href="/docs/v0.2/">d</a>`,
		"docs/v0.2/index.html":   `<a href="x/#zzz">x</a><a href="x/">ok</a><img src="assets/none.png">`,
		"docs/v0.2/x/index.html": `<h2 id="a">A</h2><a href="#b">self</a><a href="./missing.html">m</a>`,
	})
	p = checkProblems{}
	checkLinks(bad, "v0.2", true, &p)
	mustHave(t, &p, `"/docs/v0.2/nowhere/"`, "no element #zzz", "assets/none.png", "no element #b", "missing.html")

	// Not the latest: the pages outside /docs/ are left to the latest version.
	p = checkProblems{}
	checkLinks(bad, "v0.2", false, &p)
	if strings.Contains(strings.Join(p.list, "\n"), "nowhere") {
		t.Errorf("home page checked for a non-latest version: %v", p.list)
	}
}

func TestCheckMedia(t *testing.T) {
	var p checkProblems
	checkMedia(siteFiles(t, map[string]string{"a/index.html": "<p>ok</p>", "a/b.png": "x"}), &p)
	mustBeClean(t, &p)
	checkMedia(siteFiles(t, map[string]string{
		"a/index.html": `<VIDEO src="x"></VIDEO>`, "a/demo.gif": "x", "b/c.WEBM": "x", "b/d.mp4": "x", "b/e.mov": "x", "b/f.apng": "x"}), &p)
	mustHave(t, &p, "a/index.html: <video>", "a/demo.gif", "b/c.WEBM", "b/d.mp4", "b/e.mov", "b/f.apng")
}

func widgetPage(prog, files string) string {
	return `<div class="synod-program" id="prog-f1" data-program="` + prog + `" data-op="run" data-files="#f1" data-show="x"><script type="application/json" id="f1">` + files + `</script></div>`
}

func TestCheckWidgets(t *testing.T) {
	out := siteFiles(t, map[string]string{
		"docs/v0.2/ok/index.html":     widgetPage("hello", `{"hello/hello.synod":"# package main\n"}`),
		"docs/v0.2/wrong/index.html":  widgetPage("hello", `{"other/other.synod":"x"}`),
		"docs/v0.2/nofile/index.html": `<div class="synod-program" data-program="raft" data-files-url="assets/none.json"></div>`,
		"docs/v0.2/url/index.html":    `<div class="synod-program" data-program="raft" data-files-url="assets/ex.json" data-files-prefix="raft/"></div>`,
		"docs/v0.2/url2/index.html":   `<div class="synod-program" data-program="ring" data-files-url="assets/ex.json" data-files-prefix="raft/"></div>`,
		"docs/v0.2/whole/index.html":  `<div class="synod-program" data-program="" data-op="check" data-files="#f1"><script type="application/json" id="f1">{"p/p.synod":"x"}</script></div>`,
		"docs/v0.2/str/index.html":    `<div class="synod-program" data-program="p" data-files="#f1"><script type="application/json" id="f1">"{}"</script></div>`,
		"docs/v0.2/assets/ex.json":    `{"synod.mod":"module m","raft/raft.synod":"x","ring/ring.synod":"y"}`,
	})
	var p checkProblems
	checkWidgets(out, filepath.Join(out, "docs", "v0.2"), &p)
	mustHave(t, &p, "wrong/index.html", "nofile/index.html", "url2/index.html", "str/index.html")
	if len(p.list) != 4 {
		t.Errorf("problems: %v", p.list)
	}
}

func TestSpecCodes(t *testing.T) {
	md := "# 11. Errors\n\n## 11.1 Format\n\n| `skipped` | x |\n\n## 11.2 Compile-time codes\n\n| Code | Meaning |\n| --- | --- |\n| `syntax` | a |\n| `unused-name` / `unused-import` | b |\n\n## 11.3 Run-time\n\n| `nobody` | c |\n| `index-range`, `syntax` | as above |\n"
	got := strings.Join(checkSpecCodes(md), " ")
	if got != "syntax unused-name unused-import nobody index-range" {
		t.Errorf("got %q", got)
	}
}

func TestRealSpecHas41Codes(t *testing.T) {
	data, err := os.ReadFile("../../../dsl/spec/11-errors.md")
	if err != nil {
		t.Skip("no sibling dsl checkout")
	}
	if n := len(checkSpecCodes(string(data))); n != 41 {
		t.Errorf("%d codes", n)
	}
}

func TestExampleProgramsAndLibraryIndex(t *testing.T) {
	md := "| Repository | Its part |\n| --- | --- |\n| x | y |\n\n| Folder | Topic | What | Seed | Run |\n| --- | --- | --- | --- | --- |\n| `tcp` | T | w | 1 | `synod run ./tcp` |\n| `raft` + `raftlook` | R | w | 3 | `synod run ./raft` |\n\ntext\n| `notrow` | q |\n"
	if got := strings.Join(checkExamplePrograms(md), " "); got != "tcp raft" {
		t.Errorf("got %q", got)
	}
	idx := checkLibraryIndex("# Library examples\n\n| Phrase | Examples |\n| --- | --- |\n| `net.Send [m: msg] from [a: node] to [b: node]` | `net/faults`, `net/pingpong` |\n| `sim.Crash [n: node]` | `sim/crash` |\n")
	if len(idx) != 2 || strings.Join(idx["net.Send [m: msg] from [a: node] to [b: node]"], " ") != "net/faults net/pingpong" {
		t.Errorf("got %v", idx)
	}
}

func TestCheckExamplePages(t *testing.T) {
	readme := siteFiles(t, map[string]string{"README.md": "| Folder | Topic | Run |\n| --- | --- | --- |\n| `tcp` | T | `synod run ./tcp` |\n| `raft` | R | `synod run ./raft` |\n"})
	docs := siteFiles(t, map[string]string{
		"examples/index.html":      "x",
		"examples/tcp/index.html":  widgetPage("tcp", `{"tcp/tcp.synod":"x"}`),
		"examples/raft/index.html": widgetPage("other", `{"other/o.synod":"x"}`),
		"examples/ring/index.html": "x",
	})
	var p checkProblems
	checkExamplePages(filepath.Join(readme, "README.md"), docs, &p)
	mustHave(t, &p, "not in examples/README.md: ring", "no widget runs the program raft")
	if len(p.list) != 3 { // plus the page that no row names, which has no siteWidget either
		t.Errorf("problems: %v", p.list)
	}
}

func TestCheckTutorialPages(t *testing.T) {
	note := func(string, ...any) {}
	var p checkProblems
	checkTutorialPages(t.TempDir(), t.TempDir(), &p, note) // no tutorial/: skipped
	mustBeClean(t, &p)

	files := map[string]string{"README.md": "# Tutorial\n", "coverage.md": "x"}
	docs := map[string]string{}
	for i := 1; i <= 15; i++ {
		slug := "c" + string(rune('a'+i-1))
		slug = strings.Replace(slug, "c", "0", 1)
		slug = []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12", "13", "14", "15"}[i-1] + "-" + slug[1:]
		files[slug+".md"] = "# x\n\n<!-- step 1 program=p -->\n\ntext\n\n<!-- step 2 program=p -->\n"
		docs["tutorial/"+slug+"/index.html"] = widgetPage("p", `{"p/p.synod":"x"}`) + widgetPage("p", `{"p/p.synod":"x"}`)
	}
	dir, out := siteFiles(t, files), siteFiles(t, docs)
	checkTutorialPages(dir, out, &p, note)
	mustBeClean(t, &p)

	// one siteWidget missing, one chapter missing
	os.WriteFile(filepath.Join(out, "tutorial", "03-c", "index.html"), []byte(widgetPage("p", `{"p/p.synod":"x"}`)), 0o644)
	os.RemoveAll(filepath.Join(out, "tutorial", "15-o"))
	checkTutorialPages(dir, out, &p, note)
	mustHave(t, &p, "tutorial/03-c: 1 widgets", "15-o")
}

func TestStepDirectiveOrder(t *testing.T) {
	dir := siteFiles(t, map[string]string{"01-hello.md": "<!-- step 1 program=a -->\n<!-- step 3 program=a -->\n"})
	if _, err := checkTutorialChapters(dir); err == nil {
		t.Error("step numbers out of order accepted")
	}
}

// fakeSynod writes a shell script that answers explain, doc and doc --json.
func fakeSynod(t *testing.T, explain string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"\"explain --json\") cat <<'EOF'\n" + explain + "\nEOF\n;;\n" +
		"\"doc\") printf 'std/net  the network\\nstd/sim  the sim\\nother/x  y\\n';;\n" +
		"\"doc --json std/net\") echo '{\"packages\":[{\"phrases\":[{\"pattern\":\"net.Send\",\"exported\":true},{\"pattern\":\"hidden\",\"exported\":false}]}]}';;\n" +
		"\"doc --json std/sim\") echo '{\"packages\":[{\"phrases\":[{\"pattern\":\"sim.Crash\",\"exported\":true}]}]}';;\n" +
		"*) echo \"unexpected: $*\" >&2; exit 2;;\nesac\n"
	p := filepath.Join(dir, "synod")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func fixtureCtx(t *testing.T, dsl, synod string) *Ctx {
	t.Helper()
	return &Ctx{Local: map[string]string{"dsl": dsl}, Synod: &Synod{Bin: synod}}
}

func TestCheckErrorPages(t *testing.T) {
	spec := "## 11.2 Compile-time codes\n\n| `syntax` | a |\n| `nobody` | b |\n"
	dsl := siteFiles(t, map[string]string{"spec/11-errors.md": spec})
	explain := `{"format":"synod-explain/1","codes":[{"code":"syntax"},{"code":"nobody"}]}`
	docs := siteFiles(t, map[string]string{"errors/index.html": "x", "errors/syntax/index.html": "x", "errors/nobody/index.html": "x"})
	c := fixtureCtx(t, dsl, fakeSynod(t, explain))
	var p checkProblems
	checkErrorPages(c, docs, &p)
	mustBeClean(t, &p)

	os.RemoveAll(filepath.Join(docs, "errors", "nobody"))
	checkErrorPages(c, docs, &p)
	mustHave(t, &p, "missing from the site: nobody")
	if len(p.list) != 2 { // explain and spec both differ
		t.Errorf("problems: %v", p.list)
	}
}

func TestCheckLibraryPages(t *testing.T) {
	note := func(string, ...any) {}
	synod := fakeSynod(t, "{}")
	noIndex := fixtureCtx(t, siteFiles(t, nil), synod)
	docs := siteFiles(t, map[string]string{
		"library/std-net/index.html": widgetPage("net/pingpong", `{"net/pingpong/p.synod":"x"}`),
		"library/std-sim/index.html": "x",
	})
	var p checkProblems
	checkLibraryPages(noIndex, docs, &p, note) // no std/examples/README.md: only the page set
	mustBeClean(t, &p)

	idx := "| Phrase | Examples |\n| --- | --- |\n| `net.Send` | `net/pingpong` |\n"
	c := fixtureCtx(t, siteFiles(t, map[string]string{"std/examples/README.md": idx}), synod)
	checkLibraryPages(c, docs, &p, note)
	mustHave(t, &p, `no example uses "sim.Crash"`)
	if len(p.list) != 1 {
		t.Errorf("problems: %v", p.list)
	}

	p = checkProblems{}
	idx += "| `sim.Crash` | `sim/crash` |\n"
	c = fixtureCtx(t, siteFiles(t, map[string]string{"std/examples/README.md": idx}), synod)
	checkLibraryPages(c, docs, &p, note)
	mustHave(t, &p, "library/std-sim: no widget runs the example sim/crash")

	os.RemoveAll(filepath.Join(docs, "library", "std-sim"))
	p = checkProblems{}
	checkLibraryPages(c, docs, &p, note)
	mustHave(t, &p, "missing from the site: std-sim")
}

func TestProblemsErrLimit(t *testing.T) {
	var p checkProblems
	if p.err() != nil {
		t.Error("empty problems gave an error")
	}
	for i := 0; i < 250; i++ {
		p.addf("p%d", i)
	}
	if e := p.err(); e == nil || !strings.Contains(e.Error(), "250 problem") || !strings.Contains(e.Error(), "and 50 more") {
		t.Errorf("%v", e)
	}
}
