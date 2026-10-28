package main

import (
	"os"
	"strings"
	"testing"
)

func TestDecisionsTable(t *testing.T) {
	c := refTestCtx(t)
	src, err := os.ReadFile("testdata/reference/dsl/spec/decisions.md")
	if err != nil {
		t.Fatal(err)
	}
	d, err := parseMarkdown("decisions.md", src)
	if err != nil {
		t.Fatal(err)
	}
	rows := decisionRows(d)
	if len(rows) != 2 || rows[1].N != 8 {
		t.Fatalf("rows = %+v", rows)
	}
	page := c.URL("design/decisions/")
	table, err := decisionsTable(rows, designOptions(c, page))
	if err != nil {
		t.Fatal(err)
	}
	table = linkifyHTML(c, page, table)
	for _, want := range []string{
		`<tr id="d7">`, `<td><a href="#d7">7</a></td>`, `<tr id="d8">`,
		`<code>use … as</code>`, `(decision <a href="#d8">8</a>)`,
	} {
		if !strings.Contains(table, want) {
			t.Errorf("missing %s in %s", want, table)
		}
	}
}

func TestSectionBlocks(t *testing.T) {
	d, err := parseMarkdown("r.md", []byte("# T\n\n## A\n\npa\n\n## Why Synod\n\npw\n\n```synod\nx\n```\n\n## Z\n\npz\n"))
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := sectionBlocks(d, "Why Synod")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 3 || blocks[0].kind != mdkHeading || blocks[2].kind != mdkFence {
		t.Errorf("blocks = %d", len(blocks))
	}
	last, err := sectionBlocks(d, "Z")
	if err != nil || len(last) != 2 {
		t.Errorf("last = %d, %v", len(last), err)
	}
	if _, err := sectionBlocks(d, "Missing"); err == nil {
		t.Error("missing section accepted")
	}
}
