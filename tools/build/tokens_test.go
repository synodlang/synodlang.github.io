package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderCodePlain(t *testing.T) {
	got := string(renderCode("a < b & \"c\"\n", nil, nil))
	want := "a &lt; b &amp; &#34;c&#34;\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderCodeTokensAndChanged(t *testing.T) {
	text := "# hi\nuse std/x\n\n  say \"a\"\n"
	toks := []Token{
		{1, 1, 4, "comment"},
		{2, 1, 3, "language"}, {2, 5, 5, "name"},
		{4, 3, 3, "phrase"}, {4, 7, 3, "value"},
	}
	got := string(renderCode(text, toks, []int{2, 3}))
	want := `<span class="t-comment"># hi</span>` + "\n" +
		`<span class="changed"><span class="t-language">use</span> <span class="t-name">std/x</span></span>` + "\n" +
		`<span class="changed"></span>` + "\n" +
		`  <span class="t-phrase">say</span> <span class="t-value">&#34;a&#34;</span>` + "\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Columns and lengths count code points: bytes and UTF-16 units both get
// "héllo 😀 wörld" wrong.
func TestRenderCodeNonASCII(t *testing.T) {
	text := "say \"héllo 😀 wörld\" now\n"
	toks := []Token{{1, 1, 3, "phrase"}, {1, 5, 15, "value"}, {1, 21, 3, "name"}}
	got := string(renderCode(text, toks, nil))
	want := `<span class="t-phrase">say</span> <span class="t-value">&#34;héllo 😀 wörld&#34;</span> <span class="t-name">now</span>` + "\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRenderCodeEdgeTokens(t *testing.T) {
	// A token past the line end is clipped, an overlapping one loses to the
	// earlier token, a token of an unknown line is ignored, adjacent equal
	// roles merge, markup in a role cannot escape.
	toks := []Token{{1, 1, 2, "name"}, {1, 3, 2, "name"}, {1, 4, 99, "value"}, {1, 1, 1, "<b>"}, {7, 1, 1, "name"}}
	got := string(renderCode("abcde", toks, nil))
	want := `<span class="t-name">abcd</span>` + `<span class="t-value">e</span>`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// testSynod returns a native synod for tests that run the compiler, or skips.
func testSynod(t *testing.T) *Synod {
	t.Helper()
	bin := os.Getenv("SYNOD_BIN")
	if bin == "" {
		p, err := exec.LookPath("synod")
		if err != nil {
			t.Skip("no native synod: set SYNOD_BIN")
		}
		bin = p
	}
	return &Synod{Bin: bin}
}

func TestModuleTokens(t *testing.T) {
	c := &Ctx{Synod: testSynod(t)}
	dir := t.TempDir()
	// "héllo 😀" is 9 code points with the quotes but 14 bytes.
	src := "# package main\nuse std/lesson\n\ndefine main\n  lesson.Note \"héllo 😀\"\n"
	if err := os.MkdirAll(filepath.Join(dir, "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hello", "hello.synod"), []byte(src), 0o644); err != nil {
		t.Fatal(err) // no synod.mod: tutorial chapters 1 to 3
	}
	toks, err := moduleTokens(c, dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := toks["hello/hello.synod"]
	if len(ts) == 0 {
		t.Fatalf("no tokens: %v", toks)
	}
	html := string(renderCode(src, ts, nil))
	for _, want := range []string{
		`<span class="t-comment"># package main</span>`,
		`<span class="t-language">define</span>`,
		`<span class="t-phrase">Note</span>`,
		`<span class="t-value">&#34;héllo 😀&#34;</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Error("moduleTokens wrote into dir")
	}
}
