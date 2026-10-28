package main

import "testing"

func TestErrFilesAndFix(t *testing.T) {
	var e explainCode
	e.Example.Files = map[string]string{"p/p.synod": "a\n", "p/q.synod": "q\n"}
	e.Fix.Files = map[string]string{"p/p.synod": "b\n"}
	e.Fix.Deleted = []string{"p/q.synod"}
	e.Example.Files["synod.mod"] = "module m\nsynod 0.2\n"
	files := errFiles(e)
	if files["synod.mod"] != "module m\nsynod 0.2\n" {
		t.Error("the entry's own synod.mod must be kept")
	}
	fixed := errFixed(e, errFiles(e))
	if fixed["p/p.synod"] != "b\n" || len(fixed) != 2 {
		t.Errorf("fixed = %v", fixed)
	}
	if files["p/p.synod"] != "a\n" {
		t.Error("errFixed changed its input")
	}
}
