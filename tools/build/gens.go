package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// genVersionAssets writes what every docs version has: the pinned browser assets,
// the site's own web/ files in assets/, and a generic index of the pages.
// The index is written last, from the registry.
func genVersionAssets(c *Ctx) error {
	// Release assets keep their release names (wasm, wasm_exec, player ESM
	// and typings); the compiler archives are only for the build.
	for _, rel := range []Release{c.V.Compiler, c.V.Player} {
		for _, name := range sortedAssets(rel) {
			if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
				continue
			}
			if strings.HasPrefix(name, "synod-player-fonts-") {
				continue // unpacked and compared with web/fonts below
			}
			data, err := c.AssetData(name)
			if err != nil {
				return err
			}
			if err := c.WriteDocs("assets/"+name, data); err != nil {
				return err
			}
		}
	}
	if err := verifyFonts(c); err != nil {
		return err
	}
	web := filepath.Join(c.Root, "web")
	if _, err := os.Stat(web); err != nil {
		return nil
	}
	return filepath.WalkDir(web, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(web, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return c.WriteDocs("assets/"+filepath.ToSlash(rel), data)
	})
}

// genDocsIndex writes /docs/<version>/ as a list of the registered pages by
// section, unless a section page already took that URL.
func genDocsIndex(c *Ctx) error {
	if _, ok := c.Pages.Get(c.DocsURL()); ok {
		return nil
	}
	var b strings.Builder
	b.WriteString("<h1>Synod documentation " + template.HTMLEscapeString(c.V.Docs) + "</h1>\n<ul>\n")
	for _, p := range c.Pages.All() {
		if p.URL == c.DocsURL() || !strings.HasPrefix(p.URL, c.DocsURL()) {
			continue
		}
		fmt.Fprintf(&b, "<li><a href=\"%s\">%s</a></li>\n", template.HTMLEscapeString(p.URL), template.HTMLEscapeString(p.Title))
	}
	b.WriteString("</ul>")
	return c.AddPage(&Page{URL: c.DocsURL(), Title: "Documentation", Body: template.HTML(b.String())})
}

// versionsDoc is /docs/versions.json, read by the version switcher.
type versionsDoc struct {
	Latest   string         `json:"latest"`
	Versions []versionEntry `json:"versions"`
}

type versionEntry struct {
	Docs string `json:"docs"`
	URL  string `json:"url"`
}

// genTop writes the files that belong to no version: /docs/ (a redirect to
// the latest docs), /docs/versions.json, and install/ with install.sh copied
// byte for byte.
func genTop(root, out string, deps *Deps) error {
	doc := versionsDoc{Latest: deps.Latest}
	for _, v := range deps.Versions {
		doc.Versions = append(doc.Versions, versionEntry{v.Docs, "/docs/" + v.Docs + "/"})
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(out, "docs", "versions.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	target := "/docs/" + deps.Latest + "/"
	redirect := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Synod documentation</title>
<meta http-equiv="refresh" content="0; url=%[1]s">
<link rel="canonical" href="%[1]s">
</head>
<body><p><a href="%[1]s">Synod documentation</a></p></body>
</html>
`, target)
	if err := writeFile(filepath.Join(out, "docs", "index.html"), []byte(redirect), 0o644); err != nil {
		return err
	}
	return copyInstall(root, out)
}

// copyInstall copies install/ and install.sh to out unchanged.
func copyInstall(root, out string) error {
	if err := copyExact(filepath.Join(root, "install.sh"), filepath.Join(out, "install.sh")); err != nil {
		return err
	}
	return CopyTree(filepath.Join(root, "install"), filepath.Join(out, "install"))
}

// copyExact copies one file byte for byte, keeping its permission bits.
func copyExact(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return writeFile(dst, data, info.Mode().Perm())
}

// sameFile reports whether two files hold the same bytes.
func sameFile(a, b string) (bool, error) {
	x, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	y, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(x, y), nil
}

// verifyFonts checks that web/fonts, which the site serves for its own pages
// and as the player's fontBase, holds exactly the files of the pinned player
// fonts asset (decision 118). A version without a fonts asset has nothing to
// compare.
func verifyFonts(c *Ctx) error {
	name := c.V.FontsAsset()
	if name == "" {
		return nil
	}
	gz, err := c.AssetData(name)
	if err != nil {
		return err
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	tr := tar.NewReader(zr)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		base := path.Base(h.Name)
		if !strings.HasSuffix(base, ".woff2") && !strings.HasPrefix(base, "OFL-") {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		mine, err := os.ReadFile(filepath.Join(c.Root, "web", "fonts", base))
		if err != nil {
			return fmt.Errorf("web/fonts/%s is missing; the player fonts asset has it: %v", base, err)
		}
		if !bytes.Equal(mine, data) {
			return fmt.Errorf("web/fonts/%s differs from %s (decision 118: one font set); copy the asset's files", base, name)
		}
		n++
	}
	if n < 8 {
		return fmt.Errorf("%s holds %d font files, expected the 4 woff2 and 4 OFL texts", name, n)
	}
	return nil
}
