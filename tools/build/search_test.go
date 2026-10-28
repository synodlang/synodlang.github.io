package main

import (
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHTMLText(t *testing.T) {
	got := searchText(`<p>Hello <b>big</b>  &amp; <code>x</code></p><pre class="synod"><code>skip me</code></pre><script type="application/json">{"a":1}</script><p>end</p>`)
	if got != "Hello big & x end" {
		t.Errorf("got %q", got)
	}
}

func TestAfterID(t *testing.T) {
	body := `<h2 id="a">Alpha</h2><p>first text</p><h3 id="b">Beta</h3><p>second text</p>` +
		`<section id="c"><h3>Gamma</h3><p>third</p></section>`
	for id, want := range map[string]string{"a": "first text", "b": "second text", "c": "third"} {
		if got := searchText(searchAfterID(body, id)); got != want {
			t.Errorf("%s: got %q, want %q", id, got, want)
		}
	}
	if searchAfterID(body, "zzz") != "" {
		t.Error("unknown id gave text")
	}
}

func TestExcerptLimit(t *testing.T) {
	long := strings.Repeat("wörd ", 200)
	x := searchExcerpt(long)
	if n := utf8.RuneCountInString(x); n > searchExcerptMax || !strings.HasSuffix(x, "…") {
		t.Errorf("%d runes: %q", n, x)
	}
	if searchExcerpt("short") != "short" {
		t.Error("short text changed")
	}
}

func TestSearchEntries(t *testing.T) {
	pages := []*Page{
		{URL: "/", Title: "Home", Body: "<p>home</p>"},
		{URL: "/docs/v0.2/library/std-net/", Title: "std/net", Section: "library",
			Body: template.HTML(`<h2 id="p-send">net.Send</h2><p>Send a message.</p>`),
			Toc:  []TocItem{{ID: "p-send", Title: "net.Send", Level: 2}}},
		{URL: "/docs/v0.2/errors/unknown-phrase/", Title: "unknown-phrase", Section: "errors", Body: "<p>No phrase matches.</p>"},
		{URL: "/docs/v0.1/errors/syntax/", Title: "syntax", Section: "errors", Body: "<p>old</p>"},
		{URL: "/tribute/", Title: "Tribute", Body: "<p>Lamport</p>"},
	}
	es := searchEntries(pages, "/docs/v0.2/")
	var urls []string
	for _, e := range es {
		urls = append(urls, e.U)
		if e.T == "" || e.U == "" {
			t.Errorf("empty field: %+v", e)
		}
	}
	want := "/docs/v0.2/library/std-net/ /docs/v0.2/library/std-net/#p-send /docs/v0.2/errors/unknown-phrase/ /tribute/"
	if strings.Join(urls, " ") != want {
		t.Errorf("urls %v", urls)
	}
	if es[1].X != "Send a message." || es[1].S != "Library: std/net" || es[2].S != "Errors" {
		t.Errorf("%+v", es[1:3])
	}
}

func TestBuildSearchWritesJSON(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	c, err := NewCtx("../..", out, d, &d.Versions[0], nil, &Fetcher{Dir: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddPage(&Page{URL: c.URL("errors/syntax/"), Title: "syntax", Section: "errors", Body: "<p>One &amp; two.</p>"}); err != nil {
		t.Fatal(err)
	}
	if err := buildSearch(c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "docs", d.Versions[0].Docs, "search.json"))
	if err != nil {
		t.Fatal(err)
	}
	var es []searchEntry
	if err := json.Unmarshal(data, &es); err != nil || len(es) != 1 || es[0].X != "One & two." {
		t.Errorf("%v %+v", err, es)
	}
}
