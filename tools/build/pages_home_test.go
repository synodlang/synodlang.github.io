package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestHomeHTML(t *testing.T) {
	s := fmt.Sprintf(homeHTML, "/logo.png", "/t/", "/install/", "/r/", "/e/", "0.2", "/scene.json", "/player.js", "/fonts/")
	if strings.Contains(s, "%!") {
		t.Fatalf("bad format verbs: %s", s)
	}
	for _, want := range []string{`href="/t/"`, `href="/install/"`, `href="/r/"`, `import { createPlayer } from "/player.js"`, `fetch("/scene.json")`, "loop: true", `fontBase = "/fonts/"`, "fontsReady", "Implements Synod 0.2"} {
		if !strings.Contains(s, want) {
			t.Errorf("home lacks %q", want)
		}
	}
}
