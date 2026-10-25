package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateMD = flag.Bool("update-md", false, "rewrite the golden files of testdata/md")

// goldenOptions uses every hook: it rewrites spec links, replaces synod
// fences and surfaces directives as elements.
func goldenOptions() mdOptions {
	return mdOptions{
		Link: func(href string) string {
			if rest, ok := strings.CutPrefix(href, "spec/"); ok {
				return "/reference/" + strings.Replace(rest, ".md", "", 1)
			}
			return href
		},
		Code: func(info, text string) (template.HTML, bool) {
			if info != "synod" {
				return "", false
			}
			return template.HTML(fmt.Sprintf("<synod-code lines=%d></synod-code>", strings.Count(text, "\n"))), true
		},
		Directive: func(text string) (template.HTML, error) {
			return template.HTML("<directive>" + esc.Replace(text) + "</directive>"), nil
		},
	}
}

func TestMarkdownGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/md/*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden inputs: %v", err)
	}
	for _, f := range files {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".md"), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := parseMarkdown(f, src)
			if err != nil {
				t.Fatal(err)
			}
			out, err := doc.render(goldenOptions())
			if err != nil {
				t.Fatal(err)
			}
			golden := strings.TrimSuffix(f, ".md") + ".html"
			if *updateMD {
				if err := os.WriteFile(golden, []byte(out), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != string(want) {
				t.Errorf("rendering differs from %s\n--- got\n%s\n--- want\n%s", golden, out, want)
			}
		})
	}
}

func TestMarkdownErrors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"setext", "Title\n=====\n", "x.md:2: setext headings are not supported"},
		{"setext dashes", "ok\n\nTitle\n---\n", "x.md:4: setext headings are not supported"},
		{"indented code", "para\n\n    code\n", "x.md:3: indented code blocks are not supported"},
		{"open fence", "a\n\n```synod\nx\n", "x.md:3: code fence is not closed"},
		{"raw html", "a <b>bold</b> b\n", "x.md:1: raw HTML in text is not supported"},
		{"other block html", "<div>x</div>\n", "x.md:1: raw HTML in text is not supported"},
		{"html in details", "<details open class=x>\n", "x.md:1: raw HTML in text is not supported"},
		{"inline comment", "a <!-- c --> b\n", "x.md:1: raw HTML in text is not supported"},
		{"open comment", "x\n\n<!-- never\n", "x.md:3: HTML comment is not closed"},
		{"text after comment", "<!-- c --> tail\n", "x.md:1: text after an HTML comment"},
		{"wide row", "| a |\n|---|\n| 1 | 2 |\n", "x.md:3: table row has 2 cells, header has 1"},
		{"hard break", "a\\\nb\n", "x.md:1: hard line breaks are not supported"},
		{"later line", "a\n\nb\nc <i>x</i>\n", "x.md:4: raw HTML in text is not supported"},
		{"in list", "- a\n  <u>x</u>\n", "x.md:2: raw HTML in text is not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseMarkdown("x.md", []byte(tt.src))
			if err == nil {
				_, err = doc.render(mdOptions{})
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestMarkdownDirectiveError(t *testing.T) {
	doc, err := parseMarkdown("x.md", []byte("a\n\n<!-- bad -->\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = doc.render(mdOptions{Directive: func(text string) (template.HTML, error) {
		return "", fmt.Errorf("unknown directive")
	}})
	if err == nil || !strings.Contains(err.Error(), `x.md:3: directive "bad": unknown directive`) {
		t.Errorf("error = %v", err)
	}
}

func TestMarkdownHeadings(t *testing.T) {
	src, err := os.ReadFile("testdata/md/headings.md")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseMarkdown("headings.md", src)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range doc.headings(mdOptions{}) {
		got = append(got, fmt.Sprintf("%d:%s:%s:%d", h.Level, h.ID, h.Text, h.Line))
	}
	want := []string{
		"1:same:Same:1",
		"2:same-1:Same:2",
		"3:same-code-x:Same code x:3",
		"4:closing-hashes:Closing hashes:4",
		"5:with-bold--amp:With bold & amp:5",
		"6:33-odd-punctuation:§3.3 Odd, punctuation!:6",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("headings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A caller hook chooses the ids, in render and in headings alike.
	opts := mdOptions{HeadingID: func(level int, text string) string {
		if rest, ok := strings.CutPrefix(text, "§"); ok {
			return "s" + strings.ReplaceAll(strings.Fields(rest)[0], ".", "-")
		}
		return fmt.Sprintf("h%d", level)
	}}
	out, err := doc.render(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<h6 id="s3-3">`) || !strings.Contains(string(out), `<h1 id="h1">`) {
		t.Errorf("hook ids missing in:\n%s", out)
	}
	if hs := doc.headings(opts); hs[5].ID != "s3-3" {
		t.Errorf("headings ids ignore the hook: %+v", hs[5])
	}
}

func TestMarkdownPlainAndDirectives(t *testing.T) {
	src, err := os.ReadFile("testdata/md/directives.md")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseMarkdown("directives.md", src)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(doc.directives(), "|")
	if want := "new|step 2 program=counter|why indent|multi line\ndirective"; got != want {
		t.Errorf("directives = %q, want %q", got, want)
	}
	if want := "4. Title\nSteps\nText right after a directive.\nDone.\n"; doc.plain() != want {
		t.Errorf("plain = %q, want %q", doc.plain(), want)
	}
	if got := doc.excerpt(100); got != "Text right after a directive." {
		t.Errorf("excerpt = %q", got)
	}
	if got := doc.excerpt(12); got != "Text right…" {
		t.Errorf("short excerpt = %q", got)
	}
	// Without a hook, directives are dropped.
	out, err := doc.render(mdOptions{})
	if err != nil || strings.Contains(string(out), "<!--") || strings.Contains(string(out), "multi line") {
		t.Errorf("unhooked render = %q, %v", out, err)
	}
}

func TestMarkdownRenderBlocks(t *testing.T) {
	doc, err := parseMarkdown("x.md", []byte("# A\n\ntext\n\n<!-- cut -->\n\n## B\n\nmore\n"))
	if err != nil {
		t.Fatal(err)
	}
	var after []*mdBlock
	for i, b := range doc.blocks {
		if b.kind == mdkDirective {
			after = doc.blocks[i+1:]
		}
	}
	out, err := doc.renderBlocks(after, mdOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := "<h2 id=\"b\">B</h2>\n<p>more</p>\n"; string(out) != want {
		t.Errorf("renderBlocks = %q, want %q", out, want)
	}
}

// TestMarkdownDSL renders every Markdown file of a dsl checkout.
func TestMarkdownDSL(t *testing.T) {
	root := os.Getenv("SYNOD_DSL")
	if root == "" {
		t.Skip("SYNOD_DSL is not set")
	}
	n := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		n++
		rel, _ := filepath.Rel(root, path)
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		doc, err := parseMarkdown(rel, src)
		if err != nil {
			t.Error(err)
			return nil
		}
		if _, err := doc.render(mdOptions{Directive: func(string) (template.HTML, error) { return "", nil }}); err != nil {
			t.Error(err)
		}
		if len(doc.headings(mdOptions{})) == 0 {
			t.Errorf("%s: no headings", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("no Markdown files under %s", root)
	}
}
