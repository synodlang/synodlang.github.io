package main

import (
	"reflect"
	"testing"
)

const exampleReadme = "intro\n\n" +
	"| Folder | Topic | What to watch | Seed | Run |\n| --- | --- | --- | --- | --- |\n" +
	"| `tcp` | TCP handshake | SYN and ACK | 1 (default) | `synod run ./tcp` |\n" +
	"| `raft` + `raftlook` | Raft **election** | votes | 3 | `synod run ./raft` |\n" +
	"| `wal` | Write-ahead log | survivors | 1 | `synod run ./wal` |\n"

func TestExampleRows(t *testing.T) {
	rows, err := exampleRows(exampleReadme)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rows {
		got = append(got, r.Folder)
	}
	if want := []string{"tcp", "raft", "wal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("folders %v, want %v", got, want)
	}
	if rows[1].Seed != "3" || rows[1].Title() != "raft: Raft election" {
		t.Fatalf("row %+v title %q", rows[1], rows[1].Title())
	}
}

func TestExampleShow(t *testing.T) {
	files := map[string]string{
		"synod.mod":         "module example.com/m\nsynod 0.2\n",
		"a/a.synod":         "# package a\nuse std/net\n",
		"a/demo.synod":      "# package main\nuse example.com/m/a\nuse example.com/m/b\nuse std/sim\n",
		"b/demo.synod":      "# package main\n",
		"b/b.synod":         "# package b\nuse example.com/m/c\n",
		"c/c.synod":         "# package c\n",
		"unrelated/u.synod": "# package u\n",
	}
	got := exampleShow(files, "example.com/m", "a")
	want := []string{"a/demo.synod", "a/a.synod", "b/b.synod", "c/c.synod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("show %v, want %v", got, want)
	}
}
