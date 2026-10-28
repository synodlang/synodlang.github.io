package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// tutorialCtx makes a Ctx over testdata/tutorial/dsl.
func tutorialCtx(t *testing.T, dsl string) *Ctx {
	t.Helper()
	deps, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	v, ok := deps.Find(deps.Latest)
	if !ok {
		t.Fatal("no latest version")
	}
	abs, err := filepath.Abs(dsl)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCtx("../..", t.TempDir(), deps, v, map[string]string{"dsl": abs}, NewFetcher(t.TempDir(), io.Discard), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	c.Synod = testSynod(t)
	return c
}

func readOut(t *testing.T, c *Ctx, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(c.Out, "docs", c.V.Docs, filepath.FromSlash(rel), "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBuildTutorialAndGuides(t *testing.T) {
	c := tutorialCtx(t, "testdata/tutorial/dsl")
	if err := buildTutorial(c); err != nil {
		t.Fatal(err)
	}
	if err := buildGuides(c); err != nil {
		t.Fatal(err)
	}
	ch1 := readOut(t, c, "tutorial/01-hello/")
	for _, want := range []string{
		// widget: step 1 is all new, step 2 changes line 4 only
		`id="prog-s1" data-program="hello" data-op="run"`,
		`data-show="hello/hello.synod"`,
		`data-changed="1,2,3,4,5"`,
		`data-changed="4"`,
		`<span class="changed"><span class="t-comment"># Say hello.</span></span>`,
		// new list with Reference and Library links
		`href="/docs/v0.2/reference/06-packages-and-programs/#s6-1"`,
		`href="/docs/v0.2/library/std-lesson/"`,
		// links
		`href="/docs/v0.2/tutorial/02-values/"`,
		`href="/docs/v0.2/guides/install/"`,
		// why: the captured diagnostic and the Errors page
		`error[indent]: tabs are not allowed`,
		`<a href="/docs/v0.2/errors/indent/"><code>indent</code></a>`,
		// pager
		`rel="prev" href="/docs/v0.2/tutorial/"`,
		`rel="next" href="/docs/v0.2/tutorial/02-values/"`,
	} {
		if !strings.Contains(ch1, want) {
			t.Errorf("chapter 1 lacks %s", want)
		}
	}
	// Chapter 2, step 1 is compared with chapter 1's last step.
	ch2 := readOut(t, c, "tutorial/02-values/")
	if !strings.Contains(ch2, `data-changed="3,5,7,9"`) {
		t.Errorf("chapter 2 step 1 should change lines 3, 5, 7 and 9")
	}
	if !strings.Contains(ch2, `rel="prev" href="/docs/v0.2/tutorial/01-hello/"`) {
		t.Errorf("chapter 2 lacks the previous link")
	}
	if !strings.Contains(readOut(t, c, "tutorial/"), `href="/docs/v0.2/tutorial/01-hello/"`) {
		t.Error("index does not link chapter 1")
	}
	g := readOut(t, c, "guides/install/")
	for _, want := range []string{`href="/install/"`, `href="/docs/v0.2/tutorial/"`, `href="/docs/v0.2/tutorial/01-hello/"`} {
		if !strings.Contains(g, want) {
			t.Errorf("guide lacks %s", want)
		}
	}
	if !strings.Contains(readOut(t, c, "guides/"), `href="/docs/v0.2/guides/install/"`) {
		t.Error("guides index does not link the guide")
	}
}

func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := CopyTree("testdata/tutorial/dsl", dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestBuildTutorialFailsOnSummaryMismatch(t *testing.T) {
	dsl := copyFixture(t)
	f := filepath.Join(dsl, "tutorial/code/01-hello/step1.summary")
	if err := os.WriteFile(f, []byte("run hello seed 1\nend: wrong\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := buildTutorial(tutorialCtx(t, dsl))
	if err == nil || !strings.Contains(err.Error(), "differs from step1.summary") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildTutorialFailsOnWrongWhyCode(t *testing.T) {
	dsl := copyFixture(t)
	f := filepath.Join(dsl, "tutorial/code/01-hello/why-indent/expect")
	if err := os.WriteFile(f, []byte("unknown-phrase\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := buildTutorial(tutorialCtx(t, dsl))
	if err == nil || !strings.Contains(err.Error(), "why indent") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildTutorialSkipsWithoutTutorial(t *testing.T) {
	dsl := t.TempDir()
	c := tutorialCtx(t, dsl)
	if err := buildTutorial(c); err != nil {
		t.Fatal(err)
	}
	if err := buildGuides(c); err != nil {
		t.Fatal(err)
	}
	if n := len(c.Pages.All()); n != 0 {
		t.Errorf("%d pages", n)
	}
}

func TestBaselinesDetectRename(t *testing.T) {
	prev := map[string]string{
		"synod.mod":               "module a\n",
		"tutorial/tutorial.synod": "l1\nl2\nl3\nl4\n",
		"tutorial/demo.synod":     "d1\nd2\n",
	}
	cur := map[string]string{
		"synod.mod":             "module a\n",
		"counter/counter.synod": "l1\nl2\nl3\nl4\nl5\n",
		"counter/demo.synod":    "d1\nd2\nd3\n",
		"counter/extra.synod":   "x\n",
	}
	b := baselines(prev, cur)
	if b["counter/counter.synod"] != prev["tutorial/tutorial.synod"] || b["counter/demo.synod"] != prev["tutorial/demo.synod"] {
		t.Errorf("renames not found: %v", b)
	}
	if _, ok := b["counter/extra.synod"]; ok {
		t.Error("unrelated new file got a baseline")
	}
	got := changedFiles(b, cur)
	want := []string{"counter/counter.synod", "counter/demo.synod", "counter/extra.synod"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changedFiles = %v, want %v", got, want)
	}
	if d := diffLines(b["counter/counter.synod"], cur["counter/counter.synod"]); !reflect.DeepEqual(d, []int{5}) {
		t.Errorf("rename diff = %v", d)
	}
}

func TestChangedFilesUnchangedShowsSources(t *testing.T) {
	files := map[string]string{"synod.mod": "m\n", "a/a.synod": "x\n", "vendor/z/z.synod": "y\n"}
	got := changedFiles(files, files)
	if !reflect.DeepEqual(got, []string{"a/a.synod"}) {
		t.Errorf("got %v", got)
	}
}
