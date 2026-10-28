package main

import (
	"strings"
	"testing"
)

func TestSnippetKind(t *testing.T) {
	for text, want := range map[string]string{
		"# package main\nuse std/sim\n":                      "program main",
		"\n# package tcp\ndefine X\n":                        "library tcp",
		"# from package gossip\nNode has\n":                  "",
		"define Send [m: msg] to [n: node]\n  emit m by n\n": "",
		"": "",
	} {
		kind, pkg := snippetKind(text)
		if got := strings.TrimSpace(kind + " " + pkg); got != want {
			t.Errorf("snippetKind(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestTaughtIn(t *testing.T) {
	c := refTestCtx(t)
	got, err := refTaughtIn(c)
	if err != nil {
		t.Fatal(err)
	}
	// Chapter 2 and 4 exist; both teach s3-3; chapter 4 also s4-1.
	if g := got["s3-3"]; len(g) != 2 || g[0].N != 2 || g[1].Title != "4. First node" {
		t.Errorf("s3-3 = %+v", g)
	}
	if g := got["s4-1"]; len(g) != 1 || g[0].Slug != "04-first-node" {
		t.Errorf("s4-1 = %+v", g)
	}
	box := refTaughtBox(c, got["s3-3"])
	if !strings.Contains(box, `href="/docs/v0.2/tutorial/02-values/">2. Values</a>, <a`) {
		t.Errorf("box = %s", box)
	}
}

func TestInsertAfterHeading(t *testing.T) {
	in := "<h2 id=\"s3-3\">3.3 <code>x</code></h2>\n<p>a</p>\n"
	got := refInsertAfterHeading(in, "s3-3", "<p>BOX</p>\n")
	if got != "<h2 id=\"s3-3\">3.3 <code>x</code></h2>\n<p>BOX</p>\n<p>a</p>\n" {
		t.Errorf("got %q", got)
	}
	if refInsertAfterHeading(in, "s9-9", "BOX") != in {
		t.Error("changed without the heading")
	}
}

func TestRefSnippetFragment(t *testing.T) {
	s := &refSnippets{c: refTestCtx(t), name: "x.md"}
	h, ok := s.render("synod", "define Send [m: msg]\n  emit m <by> n\n")
	if !ok || !strings.HasPrefix(string(h), `<pre class="synod"><code>define Send`) || !strings.Contains(string(h), "&lt;by&gt;") {
		t.Errorf("fragment = %v %s", ok, h)
	}
	if _, ok := s.render("", "plain"); ok {
		t.Error("untagged block taken")
	}
	if s.n != 0 {
		t.Error("fragment counted as a widget")
	}
}
