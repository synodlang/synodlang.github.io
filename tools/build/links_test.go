package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// refTestCtx is a Ctx over testdata/reference/dsl with the diagnostic codes
// given, without running the compiler.
func refTestCtx(t *testing.T, codes ...string) *Ctx {
	t.Helper()
	dsl, err := filepath.Abs("testdata/reference/dsl")
	if err != nil {
		t.Fatal(err)
	}
	c := &Ctx{
		Local: map[string]string{"dsl": dsl},
		V:     &Version{Docs: "v0.2", DSL: Repo{Tag: "v0.2.14"}},
	}
	info, err := loadSpecInfo(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range codes {
		info.codes[code] = true
	}
	linkCache.Store(c, info)
	t.Cleanup(func() { linkCache.Delete(c) })
	return c
}

func TestRewriteHref(t *testing.T) {
	c := refTestCtx(t)
	page := c.URL("tutorial/04-first-node/")
	for href, want := range map[string]string{
		"spec/03":                                "/docs/v0.2/reference/03-phrases/",
		"spec/03 §3.3":                           "/docs/v0.2/reference/03-phrases/#s3-3",
		"spec/03-phrases.md":                     "/docs/v0.2/reference/03-phrases/",
		"../spec/03-phrases.md#33-resolution":    "/docs/v0.2/reference/03-phrases/#s3-3",
		"../spec/03-phrases.md#s3-4":             "/docs/v0.2/reference/03-phrases/#s3-4",
		"03-phrases.md":                          "/docs/v0.2/reference/03-phrases/",
		"§3.3":                                   "/docs/v0.2/reference/03-phrases/#s3-3",
		"§4.1":                                   "/docs/v0.2/reference/04-types/#s4-1",
		"spec/decisions.md":                      "/docs/v0.2/design/decisions/",
		"../spec/decisions.md#x":                 "/docs/v0.2/design/decisions/#x",
		"../implementation/09-site-and-org.md#a": "https://github.com/synodlang/dsl/blob/v0.2.14/implementation/09-site-and-org.md#a",
		"implementation/04-formats.md":           "https://github.com/synodlang/dsl/blob/v0.2.14/implementation/04-formats.md",
		"tutorial/04-first-node.md":              "/docs/v0.2/tutorial/04-first-node/",
		"tutorial/guides/install.md":             "/docs/v0.2/guides/install/",
		"std/README.md":                          "/docs/v0.2/library/",
	} {
		got, ok := rewriteHref(c, page, href)
		if !ok || got != want {
			t.Errorf("rewriteHref(%q) = %q, %v; want %q", href, got, ok, want)
		}
	}
	for _, href := range []string{"", "#x", "https://example.com/spec/03", "/abs", "§9.9", "spec/77", "tutorial/09-x.md", "notes.md", "mailto:a@b.c"} {
		if got, ok := rewriteHref(c, page, href); ok || got != href {
			t.Errorf("rewriteHref(%q) = %q, %v; want unchanged", href, got, ok)
		}
	}
	// A link to the page it is on keeps just the fragment.
	got, _ := rewriteHref(c, c.URL("reference/03-phrases/"), "spec/03 §3.3")
	if got != "#s3-3" {
		t.Errorf("same-page link = %q", got)
	}
}

func TestSpecHeadings(t *testing.T) {
	c := refTestCtx(t)
	_ = c
	src := "# 3. Phrases\n\n## 3.3 Resolution\n\n### Names\n\n### Members\n\n## 3.10 Ten\n\n### Names\n"
	d, err := parseMarkdown("x.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range specHeadings(d) {
		got = append(got, h.ID)
	}
	want := "s3 s3-3 s3-3-names s3-3-members s3-10 s3-10-names"
	if strings.Join(got, " ") != want {
		t.Errorf("ids = %v, want %s", got, want)
	}
}

func TestLinkifyHTML(t *testing.T) {
	c := refTestCtx(t, "unknown-phrase", "syntax")
	page := c.URL("reference/03-phrases/")
	in := `<p>See <code>spec/04</code> §4.1 and §3.3, <code>implementation/04-formats.md</code>, <code>unknown-phrase</code> and decisions 7, 8 and 99.</p>` +
		`<pre><code>decision 7 §3.3 unknown-phrase</code></pre><h2 id="x">§3.3 decision 7</h2>` +
		`<script type="application/json">"decision 7 §3.3"</script><a href="/x">§3.3</a>`
	got := linkifyHTML(c, page, in)
	for _, want := range []string{
		`<a href="/docs/v0.2/reference/04-types/#s4-1"><code>spec/04</code> §4.1</a>`,
		`<a href="#s3-3">§3.3</a>`,
		`<a href="https://github.com/synodlang/dsl/blob/v0.2.14/implementation/04-formats.md"><code>`,
		`<a href="/docs/v0.2/errors/unknown-phrase/"><code>unknown-phrase</code></a>`,
		`decisions <a href="/docs/v0.2/design/decisions/#d7">7</a>, <a href="/docs/v0.2/design/decisions/#d8">8</a> and 99`,
		`<pre><code>decision 7 §3.3 unknown-phrase</code></pre><h2 id="x">§3.3 decision 7</h2>`,
		`<script type="application/json">"decision 7 §3.3"</script><a href="/x">§3.3</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s\nin %s", want, got)
		}
	}
	if strings.Contains(got, "#d99") {
		t.Error("unknown decision 99 linked")
	}
}
