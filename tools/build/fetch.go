package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Fetcher downloads the pinned dependencies of one docs version into
// Dir/<docs>/ (decision 106).
//
//	.deps/<docs>/assets/<name>        verified release assets
//	.deps/<docs>/sums/<repo>-<tag>    the release SHA256SUMS text
//	.deps/<docs>/bin/synod            the host compiler, from the archive
//	.deps/<docs>/repos/<repo>         byte-exact checkouts at the pinned commit
type Fetcher struct {
	Dir    string       // the .deps directory
	Base   string       // https://github.com, replaced in tests
	Client *http.Client // nil means a client with a timeout
	GOOS   string       // host, for choosing the archive; empty means runtime's
	GOARCH string
	Log    io.Writer
	// SkipRepos names what not to fetch although it is pinned: dsl, std,
	// examples (the checkouts) or synod (the host binary). Used by -local
	// and by tests.
	SkipRepos map[string]bool
}

// NewFetcher returns a Fetcher for dir that talks to github.com.
func NewFetcher(dir string, log io.Writer) *Fetcher {
	return &Fetcher{Dir: dir, Base: "https://github.com", Log: log, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
}

func (f *Fetcher) logf(format string, args ...any) {
	if f.Log != nil {
		fmt.Fprintf(f.Log, format+"\n", args...)
	}
}

// VersionDir is where the dependencies of docs version v live.
func (f *Fetcher) VersionDir(v *Version) string { return filepath.Join(f.Dir, v.Docs) }

// AssetPath is the verified copy of a release asset.
func (f *Fetcher) AssetPath(v *Version, name string) string {
	return filepath.Join(f.VersionDir(v), "assets", name)
}

// SynodPath is the extracted host compiler.
func (f *Fetcher) SynodPath(v *Version) string {
	return filepath.Join(f.VersionDir(v), "bin", "synod")
}

// RepoPath is the checkout of a repository (dsl, std or examples).
func (f *Fetcher) RepoPath(v *Version, repo string) string {
	return filepath.Join(f.VersionDir(v), "repos", repo)
}

// Fetch makes Dir hold everything version v pins, verified. Files that
// already match their pin are not downloaded again.
func (f *Fetcher) Fetch(v *Version) error {
	if err := f.fetchRelease(v, "compiler", v.Compiler); err != nil {
		return err
	}
	if err := f.fetchRelease(v, "player", v.Player); err != nil {
		return err
	}
	if !f.SkipRepos["synod"] {
		if err := f.extractSynod(v); err != nil {
			return err
		}
	}
	for _, r := range []struct {
		name string
		repo Repo
	}{{"dsl", v.DSL}, {"std", v.Std}, {"examples", v.Examples}} {
		if f.SkipRepos[r.name] {
			continue
		}
		if err := f.checkout(v, r.name, r.repo); err != nil {
			return err
		}
	}
	return nil
}

// fetchRelease verifies and caches every pinned asset of one release.
func (f *Fetcher) fetchRelease(v *Version, repo string, r Release) error {
	sums, err := f.sums(v, repo, r)
	if err != nil {
		return err
	}
	for _, name := range sortedAssets(r) {
		if err := f.asset(v, repo, r, name, sums); err != nil {
			return err
		}
	}
	return nil
}

func (f *Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (f *Fetcher) download(url string) ([]byte, error) {
	resp, err := f.client().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (f *Fetcher) releaseURL(repo, tag, name string) string {
	return fmt.Sprintf("%s/synodlang/%s/releases/download/%s/%s", f.Base, repo, tag, name)
}

// sums returns the release's SHA256SUMS, downloading it once.
func (f *Fetcher) sums(v *Version, repo string, r Release) (map[string]string, error) {
	cache := filepath.Join(f.VersionDir(v), "sums", repo+"-"+r.Tag)
	data, err := os.ReadFile(cache)
	if err != nil {
		data, err = f.download(f.releaseURL(repo, r.Tag, "SHA256SUMS"))
		if err != nil {
			return nil, err
		}
		if err := writeFile(cache, data, 0o644); err != nil {
			return nil, err
		}
	}
	return ParseSums(string(data))
}

// ParseSums reads sha256sum output: "<hex>  <name>" per line.
func ParseSums(text string) (map[string]string, error) {
	out := map[string]string{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sum, name, ok := strings.Cut(line, " ")
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if !ok || !sha256Re.MatchString(sum) || name == "" {
			return nil, fmt.Errorf("SHA256SUMS line %d is malformed: %q", i+1, line)
		}
		out[name] = sum
	}
	return out, nil
}

// SumHex is the SHA-256 of data in hex.
func SumHex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// VerifyAsset checks data against both the release's SHA256SUMS and the pin.
func VerifyAsset(name string, data []byte, sums map[string]string, pin string) error {
	line, ok := sums[name]
	if !ok {
		return fmt.Errorf("%s: not listed in the release SHA256SUMS", name)
	}
	if line != pin {
		return fmt.Errorf("%s: release SHA256SUMS says %s but deps.json pins %s", name, line, pin)
	}
	if got := SumHex(data); got != pin {
		return fmt.Errorf("%s: SHA-256 is %s, expected %s", name, got, pin)
	}
	return nil
}

func (f *Fetcher) asset(v *Version, repo string, r Release, name string, sums map[string]string) error {
	pin := r.Assets[name]
	dest := f.AssetPath(v, name)
	if data, err := os.ReadFile(dest); err == nil && VerifyAsset(name, data, sums, pin) == nil {
		return nil
	}
	f.logf("fetch %s/%s %s", repo, r.Tag, name)
	data, err := f.download(f.releaseURL(repo, r.Tag, name))
	if err != nil {
		return err
	}
	if err := VerifyAsset(name, data, sums, pin); err != nil {
		return err
	}
	return writeFile(dest, data, 0o644)
}

// extractSynod unpacks the host archive and checks `synod version`.
func (f *Fetcher) extractSynod(v *Version) error {
	archive, err := v.HostArchive(f.GOOS, f.GOARCH)
	if err != nil {
		return err
	}
	bin := f.SynodPath(v)
	if CheckSynodVersion(bin, v.Compiler.Tag) == nil {
		return nil
	}
	data, err := os.ReadFile(f.AssetPath(v, archive))
	if err != nil {
		return err
	}
	exe, err := untarMember(data, "synod")
	if err != nil {
		return fmt.Errorf("%s: %v", archive, err)
	}
	if err := writeFile(bin, exe, 0o755); err != nil {
		return err
	}
	return CheckSynodVersion(bin, v.Compiler.Tag)
}

// untarMember returns the regular file named name (at any depth) of a .tar.gz.
func untarMember(gz []byte, name string) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("no %s inside", name)
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == name {
			return io.ReadAll(io.LimitReader(tr, 256<<20))
		}
	}
}

// CheckSynodVersion runs `bin version` and requires its compiler line to be tag.
func CheckSynodVersion(bin, tag string) error {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return fmt.Errorf("%s version: %v", bin, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if fs := strings.Fields(line); len(fs) >= 2 && fs[0] == "compiler" {
			if fs[1] != tag {
				return fmt.Errorf("%s reports compiler %s, the pin is %s", bin, fs[1], tag)
			}
			return nil
		}
	}
	return fmt.Errorf("%s version: no compiler line in %q", bin, out)
}

// checkout makes Dir/.../repos/<name> a byte-exact checkout of the pinned
// tag, using the settings of decision 95 (compiler internal/get).
func (f *Fetcher) checkout(v *Version, name string, r Repo) error {
	dir := f.RepoPath(v, name)
	if head, err := gitOut(dir, "rev-parse", "HEAD"); err == nil && head == r.Commit {
		return nil
	}
	f.logf("clone %s %s", name, r.Tag)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	cfg := []string{"-c", "core.autocrlf=false", "-c", "core.eol=lf", "-c", "core.symlinks=false", "-c", "core.longpaths=true"}
	url := f.Base + "/synodlang/" + name + ".git"
	if out, err := gitOut("", append(cfg, "clone", "--quiet", "--depth", "1", "--no-tags", "--no-checkout", "--branch", r.Tag, "--", url, dir)...); err != nil {
		return fmt.Errorf("%s: git clone of %s at %s failed: %s", name, url, r.Tag, out)
	}
	attrs := filepath.Join(dir, ".git", "info", "attributes")
	if err := writeFile(attrs, []byte("* -text -eol -ident -filter\n"), 0o644); err != nil {
		return err
	}
	if out, err := gitOut(dir, append(cfg, "checkout", "--quiet", "HEAD")...); err != nil {
		return fmt.Errorf("%s: git checkout of %s failed: %s", name, r.Tag, out)
	}
	head, err := gitOut(dir, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("%s: cannot read the commit of %s: %s", name, r.Tag, head)
	}
	if head != r.Commit {
		os.RemoveAll(dir)
		return fmt.Errorf("%s: tag %s is commit %s, deps.json pins %s", name, r.Tag, head, r.Commit)
	}
	return nil
}

// gitOut runs git in dir (empty means the current one) and returns its
// trimmed output; on failure the output holds stderr too.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// writeFile writes data to path, creating its directory, through a temp file
// so an interrupted write never leaves a file that looks complete.
func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
