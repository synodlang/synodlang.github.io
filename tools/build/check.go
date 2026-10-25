package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// checkSite verifies a built site of one version (plan §3, check): internal
// hrefs and anchors resolve; Errors pages match spec/11 and explain; the
// tutorial, example and Library pages are complete; no gif, mp4, webm, mov,
// apng or <video>; install/ and install.sh byte-identical; asset hashes
// equal the pins. Only the last two are done so far.
func checkSite(c *Ctx) error {
	if err := checkInstall(c.Root, c.Out); err != nil {
		return err
	}
	return checkAssets(c)
}

// checkInstall requires out/install and out/install.sh to equal the sources.
func checkInstall(root, out string) error {
	files := []string{"install.sh"}
	entries, err := os.ReadDir(filepath.Join(root, "install"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		files = append(files, "install/"+e.Name())
	}
	for _, f := range files {
		same, err := sameFile(filepath.Join(root, f), filepath.Join(out, f))
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("%s in the site differs from the source", f)
		}
	}
	return nil
}

// checkAssets requires every browser asset in the site to hash to its pin.
func checkAssets(c *Ctx) error {
	for _, rel := range []Release{c.V.Compiler, c.V.Player} {
		for _, name := range sortedAssets(rel) {
			if hasArchiveSuffix(name) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(c.Out, "docs", c.V.Docs, "assets", name))
			if err != nil {
				return err
			}
			if got := SumHex(data); got != rel.Assets[name] {
				return fmt.Errorf("asset %s: SHA-256 %s, pinned %s", name, got, rel.Assets[name])
			}
		}
	}
	return nil
}

func hasArchiveSuffix(name string) bool {
	n := len(name)
	return n > 7 && name[n-7:] == ".tar.gz" || n > 4 && name[n-4:] == ".zip"
}
