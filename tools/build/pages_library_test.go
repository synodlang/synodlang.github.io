package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLibSlug(t *testing.T) {
	for in, want := range map[string]string{
		"sim.Seed [n: number]": "sim-seed-n-number",
		"net.Loss [p: number]": "net-loss-p-number",
		"  Heal ":              "heal",
	} {
		if got := libSlug(in); got != want {
			t.Errorf("libSlug(%q) = %q, want %q", in, got, want)
		}
	}
	if libPageURL("std/look/basic") != "library/std-look-basic/" || libExampleDir("std/look/basic") != "lookbasic" {
		t.Error("look/basic naming")
	}
}

func TestLibIndex(t *testing.T) {
	dir := t.TempDir()
	byDir := map[string][]*libExample{"net": {{Key: "net/faults"}}}
	if rows, present, err := libIndex(dir, byDir); err != nil || present || rows != nil {
		t.Fatalf("absent index: %v %v %v", rows, present, err)
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("# x\n\n| Phrase | Examples |\n| --- | --- |\n")
	if _, present, err := libIndex(dir, byDir); err != nil || present {
		t.Fatalf("empty index: %v %v", present, err)
	}
	write("# x\n\n| Phrase | Examples |\n| --- | --- |\n| `net.Heal` | `net/faults` |\n")
	rows, present, err := libIndex(dir, byDir)
	if err != nil || !present || !reflect.DeepEqual(rows["net.Heal"], []string{"net/faults"}) {
		t.Fatalf("index: %v %v %v", rows, present, err)
	}
	write("# x\n\n| Phrase | Examples |\n| --- | --- |\n| `net.Heal` | `net/nope` |\n")
	if _, _, err := libIndex(dir, byDir); err == nil {
		t.Error("unknown example accepted")
	}
	var ph docPhrase
	ph.Pattern = "net.Heal"
	if len(libPhraseExamples(rows, ph)) != 1 {
		t.Error("phrase lookup")
	}
}
