package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Deps is deps.json: what each docs version of the site is built from
// (decision 106).
type Deps struct {
	Latest   string    `json:"latest"`
	Versions []Version `json:"versions"`
}

// Version is one docs version, served at /docs/<Docs>/ (decision 107).
type Version struct {
	Docs     string  `json:"docs"`
	DSL      Repo    `json:"dsl"`
	Std      Repo    `json:"std"`
	Examples Repo    `json:"examples"`
	Compiler Release `json:"compiler"`
	Player   Release `json:"player"`
}

// Repo pins a repository by tag and the commit the tag must name.
type Repo struct {
	Tag    string `json:"tag"`
	Commit string `json:"commit"`
}

// Release pins the assets used from a release by SHA-256 (hex).
type Release struct {
	Tag    string            `json:"tag"`
	Assets map[string]string `json:"assets"`
}

var (
	docsRe   = regexp.MustCompile(`^v\d+\.\d+$`)
	tagRe    = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// LoadDeps reads and validates the deps file at path.
func LoadDeps(path string) (*Deps, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseDeps(data)
}

// ParseDeps decodes deps.json strictly: unknown fields are an error.
func ParseDeps(data []byte) (*Deps, error) {
	var d Deps
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("deps.json: %v", err)
	}
	if err := d.validate(); err != nil {
		return nil, fmt.Errorf("deps.json: %v", err)
	}
	return &d, nil
}

func (d *Deps) validate() error {
	if len(d.Versions) == 0 {
		return fmt.Errorf("no versions")
	}
	seen := map[string]bool{}
	for i := range d.Versions {
		v := &d.Versions[i]
		if !docsRe.MatchString(v.Docs) {
			return fmt.Errorf("version %d: docs %q is not vMAJOR.MINOR", i, v.Docs)
		}
		if seen[v.Docs] {
			return fmt.Errorf("docs version %s is listed twice", v.Docs)
		}
		seen[v.Docs] = true
		if err := v.validate(); err != nil {
			return fmt.Errorf("%s: %v", v.Docs, err)
		}
	}
	if !seen[d.Latest] {
		return fmt.Errorf("latest %q is not a listed version", d.Latest)
	}
	return nil
}

func (v *Version) validate() error {
	for name, r := range map[string]Repo{"dsl": v.DSL, "std": v.Std, "examples": v.Examples} {
		if !tagRe.MatchString(r.Tag) {
			return fmt.Errorf("%s: tag %q is not vX.Y.Z", name, r.Tag)
		}
		if !commitRe.MatchString(r.Commit) {
			return fmt.Errorf("%s: commit %q is not 40 lowercase hex digits", name, r.Commit)
		}
	}
	for name, r := range map[string]Release{"compiler": v.Compiler, "player": v.Player} {
		if !tagRe.MatchString(r.Tag) {
			return fmt.Errorf("%s: tag %q is not vX.Y.Z", name, r.Tag)
		}
		if len(r.Assets) == 0 {
			return fmt.Errorf("%s: no assets", name)
		}
		for a, sum := range r.Assets {
			if !strings.Contains(a, r.Tag) || strings.ContainsAny(a, "/\\") {
				return fmt.Errorf("%s: asset %q does not belong to %s", name, a, r.Tag)
			}
			if !sha256Re.MatchString(sum) {
				return fmt.Errorf("%s: asset %q: sha256 %q is not 64 lowercase hex digits", name, a, sum)
			}
		}
	}
	return nil
}

// Find returns the entry for a docs version.
func (d *Deps) Find(docs string) (*Version, bool) {
	for i := range d.Versions {
		if d.Versions[i].Docs == docs {
			return &d.Versions[i], true
		}
	}
	return nil, false
}

// HostArchive is the compiler archive holding the synod binary that runs on
// this machine: linux-amd64 on Linux, darwin-universal on macOS.
func (v *Version) HostArchive(goos, goarch string) (string, error) {
	var kind string
	switch {
	case goos == "linux" && goarch == "amd64":
		kind = "linux-amd64"
	case goos == "darwin":
		kind = "darwin-universal"
	default:
		return "", fmt.Errorf("no pinned synod for %s/%s; use -local synod=path", goos, goarch)
	}
	return v.asset(v.Compiler, "synod-"+v.Compiler.Tag+"-"+kind+".tar.gz")
}

// WasmAsset, WasmExecAsset and PlayerAsset name the pinned browser assets.
func (v *Version) WasmAsset() (string, error) {
	return v.asset(v.Compiler, "synod-"+v.Compiler.Tag+".wasm")
}

func (v *Version) WasmExecAsset() (string, error) {
	return v.asset(v.Compiler, "synod-"+v.Compiler.Tag+"-wasm_exec.js")
}

func (v *Version) PlayerAsset() (string, error) {
	return v.asset(v.Player, "synod-player-"+v.Player.Tag+".js")
}

func (v *Version) asset(r Release, name string) (string, error) {
	if _, ok := r.Assets[name]; !ok {
		return "", fmt.Errorf("deps.json: %s: %s has no asset %s", v.Docs, r.Tag, name)
	}
	return name, nil
}

// sortedAssets lists a release's asset names in a fixed order.
func sortedAssets(r Release) []string {
	names := make([]string, 0, len(r.Assets))
	for n := range r.Assets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
