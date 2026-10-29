package main

import (
	"strings"
	"testing"
)

func TestDepsV03(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	if d.Latest != "v0.3" {
		t.Fatalf("latest %s", d.Latest)
	}
	v3, _ := d.Find("v0.3")
	v2, ok := d.Find("v0.2")
	if !ok || v2.HasLooks() || v2.FontsAsset() != "" {
		t.Error("v0.2 must stay as it was: no looks, no fonts")
	}
	if !v3.HasLooks() || v3.FontsAsset() == "" {
		t.Error("v0.3 pins looks and the fonts asset")
	}
	if len(v3.Placeholders()) != 0 || len(v2.Placeholders()) != 0 {
		t.Errorf("released pins only, found placeholders %v %v", v2.Placeholders(), v3.Placeholders())
	}
}

func TestFetchRefusesPlaceholders(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	v3, _ := d.Find("v0.3")
	held := *v3
	held.Std.Commit = zeroCommit
	f := NewFetcher(t.TempDir(), nil)
	if err := f.Fetch(&held); err == nil || !strings.Contains(err.Error(), "placeholders") {
		t.Fatalf("fetch of placeholder pins: %v", err)
	}
}

func TestParseLocalNames(t *testing.T) {
	dir := t.TempDir()
	got, err := parseLocal("looks=" + dir + ",wasm=" + dir + ",wasmexec=" + dir + ",player=" + dir)
	if err != nil || len(got) != 4 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := parseLocal("nope=" + dir); err == nil {
		t.Error("unknown local name accepted")
	}
}

func TestLooksGalleryMain(t *testing.T) {
	got := looksGalleryMain("doodle", "solarized/light")
	for _, want := range []string{
		"use github.com/synodlang/examples/raft\nuse github.com/synodlang/looks/doodle\nuse github.com/synodlang/looks/solarized/light\nuse std/net\nuse std/sim\n",
		"sim.StopAt 6s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("gallery main lacks %q:\n%s", want, got)
		}
	}
	basic := looksGalleryMain("basic", "default")
	if strings.Contains(basic, "looks/") {
		t.Errorf("basic x default imports a looks package:\n%s", basic)
	}
	// canonical order: chalk sorts before lumen
	if c := looksGalleryMain("lumen", "chalk"); strings.Index(c, "looks/chalk") > strings.Index(c, "looks/lumen") {
		t.Errorf("use lines are not sorted:\n%s", c)
	}
}

func TestLooksPackages(t *testing.T) {
	files := map[string]string{
		"lumen/demo.synod": "# package main\n", "lumen/lumen.synod": "# package lumen\n# Draws rings.\nuse std/look\n",
		"solarized/light/demo.synod": "x", "solarized/light/light.synod": "# theme light\n",
	}
	readme := "| `lumen` | A ring. |\n| `solarized/light` | `import` | Soft. |\n"
	pkgs, err := looksPackages(files, readme)
	if err != nil || len(pkgs) != 2 {
		t.Fatalf("%v %v", pkgs, err)
	}
	if pkgs[0].Path != "lumen" || pkgs[0].Theme || pkgs[0].Lead != "Draws rings." || pkgs[1].Path != "solarized/light" || !pkgs[1].Theme || pkgs[1].Desc != "Soft." {
		t.Errorf("%+v", pkgs)
	}
	if _, err := looksPackages(files, "| `lumen` | A ring. |\n"); err == nil {
		t.Error("a package without a README row was accepted")
	}
}
