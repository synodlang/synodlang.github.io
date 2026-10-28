// Command build makes the Synod site (decision 106): it fetches the pinned
// dependencies of deps.json, builds _site/ from them and checks the result.
//
//	go run ./tools/build fetch
//	go run ./tools/build build [-out _site] [-local dsl=../dsl,examples=../examples,synod=path]
//	go run ./tools/build check [-out _site]
//	go run ./tools/build serve [-out _site]
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const usage = `usage: build <command> [flags]

commands:
  fetch    download and verify what deps.json pins into .deps/
  build    build the site into -out (default _site)
  check    check a built site
  serve    serve -out on 127.0.0.1:8000

flags:
  -deps file     deps file (default deps.json)
  -deps-dir dir  where fetch puts the dependencies (default .deps)
  -out dir       output directory (default _site)
  -local list    build/fetch/check, for the latest docs version only: dsl, std, examples, looks
                 (checkouts), synod (binary), wasm, wasmexec (files), player (the dist folder
                 with synod-player.js, .d.ts and synod-player-fonts.tar.gz)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
}

type options struct {
	deps, depsDir, out, root string
	local                    map[string]string
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("no command")
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	var o options
	var local string
	fs.StringVar(&o.deps, "deps", "deps.json", "deps file")
	fs.StringVar(&o.depsDir, "deps-dir", ".deps", "dependency directory")
	fs.StringVar(&o.out, "out", "_site", "output directory")
	fs.StringVar(&o.root, "root", ".", "repository root")
	fs.StringVar(&local, "local", "", "local overrides")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var err error
	if o.local, err = parseLocal(local); err != nil {
		return err
	}
	switch cmd {
	case "fetch":
		return cmdFetch(o)
	case "build":
		return cmdBuild(o)
	case "check":
		return cmdCheck(o)
	case "serve":
		return cmdServe(o)
	}
	fmt.Fprint(os.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

// parseLocal reads "dsl=../dsl,synod=path" into absolute paths.
func parseLocal(s string) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, item := range strings.Split(s, ",") {
		name, p, ok := strings.Cut(item, "=")
		if !ok || p == "" {
			return nil, fmt.Errorf("-local: %q is not name=path", item)
		}
		switch name {
		case "dsl", "std", "examples", "looks", "synod", "wasm", "wasmexec", "player":
		default:
			return nil, fmt.Errorf("-local: unknown name %q (dsl, std, examples, looks, synod, wasm, wasmexec, player)", name)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("-local %s: %v", name, err)
		}
		out[name] = abs
	}
	return out, nil
}

// localFor gives the -local overrides that apply to version v: only the
// latest docs version takes them, so older versions still build from their pins.
func (o options) localFor(deps *Deps, v *Version) map[string]string {
	if v.Docs != deps.Latest {
		return nil
	}
	return o.local
}

func (o options) fetcher() *Fetcher { return NewFetcher(o.depsDir, os.Stderr) }

func cmdFetch(o options) error {
	deps, err := LoadDeps(o.deps)
	if err != nil {
		return err
	}
	f := o.fetcher()
	for i := range deps.Versions {
		v := &deps.Versions[i]
		if len(o.localFor(deps, v)) > 0 && len(v.Placeholders()) > 0 {
			fmt.Fprintf(os.Stderr, "fetch: skipping %s (placeholder pins, built with -local)\n", v.Docs)
			continue
		}
		f.SkipRepos = map[string]bool{}
		for name := range o.localFor(deps, v) {
			f.SkipRepos[name] = true
		}
		if err := f.Fetch(&deps.Versions[i]); err != nil {
			return fmt.Errorf("%s: %v", deps.Versions[i].Docs, err)
		}
	}
	return nil
}

// builders run in this order for every docs version; search goes last.
var builders = []struct {
	name string
	fn   func(*Ctx) error
}{
	{"home", buildHome},
	{"tutorial", buildTutorial},
	{"reference", buildReference},
	{"design", buildDesign},
	{"library", buildLibrary},
	{"examples", buildExamples},
	{"looks", buildLooks},
	{"errors", buildErrors},
	{"guides", buildGuides},
	{"tribute", buildTribute},
	{"docs index", genDocsIndex},
	{"search", buildSearch},
}

func cmdBuild(o options) error {
	deps, err := LoadDeps(o.deps)
	if err != nil {
		return err
	}
	if err := removeOut(o.root, o.out); err != nil {
		return err
	}
	f := o.fetcher()
	for i := range deps.Versions {
		c, err := NewCtx(o.root, o.out, deps, &deps.Versions[i], o.localFor(deps, &deps.Versions[i]), f, os.Stderr)
		if err != nil {
			return err
		}
		if err := genVersionAssets(c); err != nil {
			return err
		}
		for _, b := range builders {
			if err := b.fn(c); err != nil {
				return fmt.Errorf("%s: %s: %v", c.V.Docs, b.name, err)
			}
		}
		fmt.Fprintf(os.Stderr, "built %s: %d pages\n", c.V.Docs, len(c.Pages.All()))
	}
	return genTop(o.root, o.out, deps)
}

func cmdCheck(o options) error {
	deps, err := LoadDeps(o.deps)
	if err != nil {
		return err
	}
	f := o.fetcher()
	for i := range deps.Versions {
		c, err := NewCtx(o.root, o.out, deps, &deps.Versions[i], o.localFor(deps, &deps.Versions[i]), f, os.Stderr)
		if err != nil {
			return err
		}
		if err := checkSite(c); err != nil {
			return fmt.Errorf("%s: %v", c.V.Docs, err)
		}
	}
	fmt.Fprintln(os.Stderr, "check: ok")
	return nil
}

func cmdServe(o options) error {
	const addr = "127.0.0.1:8000"
	if _, err := os.Stat(o.out); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "serving %s on http://%s/\n", o.out, addr)
	return http.ListenAndServe(addr, http.FileServer(http.Dir(o.out)))
}
