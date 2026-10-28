package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contentCtx(t *testing.T) *Ctx {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return &Ctx{Root: root, V: &Version{Docs: "v0.2"}, Deps: &Deps{Latest: "v0.2"}}
}

func TestTributeContent(t *testing.T) {
	c := contentCtx(t)
	html, err := renderContent(c, "tribute.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	for _, want := range []string{"The Part-Time Parliament", "ACM Transactions on Computer Systems", `href="/docs/v0.2/examples/paxos/"`, "Turing Award"} {
		if !strings.Contains(s, want) {
			t.Errorf("tribute lacks %q", want)
		}
	}
	if strings.Contains(s, "docs:") {
		t.Error("an unrewritten docs: link remains")
	}
}

func TestDocsHomeContent(t *testing.T) {
	c := contentCtx(t)
	if _, err := renderContent(c, "docs-home.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Root, "content", "docs-home.md")); err != nil {
		t.Fatal(err)
	}
}
