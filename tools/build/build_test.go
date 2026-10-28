package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDepsRepoFile(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	v, ok := d.Find(d.Latest)
	if !ok || v.Compiler.Tag == "" {
		t.Fatalf("latest %q not found", d.Latest)
	}
	for _, get := range []func() (string, error){v.WasmAsset, v.WasmExecAsset, v.PlayerAsset} {
		if _, err := get(); err != nil {
			t.Error(err)
		}
	}
	for _, p := range [][2]string{{"linux", "amd64"}, {"darwin", "arm64"}} {
		if _, err := v.HostArchive(p[0], p[1]); err != nil {
			t.Error(err)
		}
	}
	if _, err := v.HostArchive("windows", "amd64"); err == nil {
		t.Error("windows should have no pinned synod")
	}
}

func TestParseDepsRejects(t *testing.T) {
	good, err := os.ReadFile("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string][2]string{
		"short commit": {"f994b9ef5b26619b88f7d2e0458a9fda57d6a2e0", "f994b9"},
		"bad latest":   {`"latest": "v0.2"`, `"latest": "v0.3"`},
		"unknown":      {`"latest"`, `"lastest"`},
		"short sha":    {"56e81b456582bd02845bbbe50b3586fc74cefe689e7e5baf29464501098ffb78", "56e8"},
	} {
		bad := strings.Replace(string(good), edit[0], edit[1], 1)
		if bad == string(good) {
			t.Fatalf("%s: edit did not apply", name)
		}
		if _, err := ParseDeps([]byte(bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestVerifyAsset(t *testing.T) {
	data := []byte("hello")
	sum := SumHex(data)
	sums := map[string]string{"a": sum}
	if err := VerifyAsset("a", data, sums, sum); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAsset("a", []byte("tampered"), sums, sum); err == nil {
		t.Error("tampered bytes passed")
	}
	if err := VerifyAsset("a", data, map[string]string{"a": strings.Repeat("0", 64)}, sum); err == nil {
		t.Error("SHA256SUMS disagreeing with the pin passed")
	}
	if err := VerifyAsset("b", data, sums, sum); err == nil {
		t.Error("unlisted asset passed")
	}
}

func TestParseSums(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	m, err := ParseSums(sum + "  one.txt\n" + sum + " *two.bin\n\n")
	if err != nil || m["one.txt"] != sum || m["two.bin"] != sum {
		t.Fatalf("got %v, %v", m, err)
	}
	if _, err := ParseSums("nothex  x\n"); err == nil {
		t.Error("malformed line accepted")
	}
}

// fakeRelease serves one release of repo "compiler" whose archive holds a
// non-executable synod, so Fetch gets as far as running it.
func fakeRelease(t *testing.T, tamper bool) (*httptest.Server, *Version) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	body := []byte("#!/bin/sh\necho 'compiler v9.9.9'\n")
	tw.WriteHeader(&tar.Header{Name: "synod", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	zw.Close()
	archive := buf.Bytes()
	wasm := []byte("wasm")
	archName := "synod-v9.9.9-linux-amd64.tar.gz"
	sums := SumHex(archive) + "  " + archName + "\n" + SumHex(wasm) + "  synod-v9.9.9.wasm\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			w.Write([]byte(sums))
		case strings.HasSuffix(r.URL.Path, archName):
			w.Write(archive)
		case strings.HasSuffix(r.URL.Path, ".wasm"):
			if tamper {
				w.Write([]byte("evil"))
			} else {
				w.Write(wasm)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	v := &Version{Docs: "v9.9", Compiler: Release{Tag: "v9.9.9", Assets: map[string]string{
		archName: SumHex(archive), "synod-v9.9.9.wasm": SumHex(wasm),
	}}, Player: Release{Tag: "v9.9.9", Assets: map[string]string{}}}
	return srv, v
}

func TestFetchVerifies(t *testing.T) {
	srv, v := fakeRelease(t, false)
	f := &Fetcher{Dir: t.TempDir(), Base: srv.URL, GOOS: "linux", GOARCH: "amd64",
		SkipRepos: map[string]bool{"dsl": true, "std": true, "examples": true}}
	// The player release has no assets and no SHA256SUMS on the fake server.
	if err := f.fetchRelease(v, "compiler", v.Compiler); err != nil {
		t.Fatal(err)
	}
	if err := f.extractSynod(v); err != nil {
		t.Fatal(err)
	}
}

func TestFetchTamperedFails(t *testing.T) {
	srv, v := fakeRelease(t, true)
	f := &Fetcher{Dir: t.TempDir(), Base: srv.URL, GOOS: "linux", GOARCH: "amd64"}
	err := f.fetchRelease(v, "compiler", v.Compiler)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("tampered download was not refused: %v", err)
	}
	if _, statErr := os.Stat(f.AssetPath(v, "synod-v9.9.9.wasm")); statErr == nil {
		t.Error("tampered file was kept")
	}
}

func TestFetchPinMismatchFails(t *testing.T) {
	srv, v := fakeRelease(t, false)
	v.Compiler.Assets["synod-v9.9.9.wasm"] = strings.Repeat("0", 64)
	f := &Fetcher{Dir: t.TempDir(), Base: srv.URL, GOOS: "linux", GOARCH: "amd64"}
	if err := f.fetchRelease(v, "compiler", v.Compiler); err == nil {
		t.Fatal("a pin that disagrees with SHA256SUMS passed")
	}
}

func TestCopyInstallByteExact(t *testing.T) {
	out := t.TempDir()
	if err := copyInstall("../..", out); err != nil {
		t.Fatal(err)
	}
	if err := checkInstall("../..", out); err != nil {
		t.Fatal(err)
	}
	// A changed byte is noticed.
	p := filepath.Join(out, "install.sh")
	data, _ := os.ReadFile(p)
	os.WriteFile(p, append(data, '\n'), 0o644)
	if err := checkInstall("../..", out); err == nil {
		t.Error("changed install.sh not noticed")
	}
}

func TestGenTop(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := genTop("../..", out, d); err != nil {
		t.Fatal(err)
	}
	redirect, _ := os.ReadFile(filepath.Join(out, "docs", "index.html"))
	if !strings.Contains(string(redirect), `url=/docs/`+d.Latest+`/`) {
		t.Errorf("redirect: %s", redirect)
	}
	vs, _ := os.ReadFile(filepath.Join(out, "docs", "versions.json"))
	if !strings.Contains(string(vs), `"latest": "`+d.Latest+`"`) {
		t.Errorf("versions.json: %s", vs)
	}
}

func TestRenderShell(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &Fetcher{Dir: t.TempDir()}
	out := t.TempDir()
	c, err := NewCtx("../..", out, d, &d.Versions[0], map[string]string{"dsl": "/x"}, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := c.Layout.Program(ProgramData{ID: "f1", Program: "hello", Op: "run",
		Files: map[string]string{"hello/hello.synod": "# package main\n</script>"}, Show: []string{"hello/hello.synod"},
		Code: []CodeFile{{Name: "hello/hello.synod", HTML: renderCode("# package main\n", nil, nil)}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(prog), "</script>\"") || !strings.Contains(string(prog), `data-files="#f1"`) {
		t.Errorf("program: %s", prog)
	}
	if err := c.AddPage(&Page{URL: c.URL("x/"), Title: "X", Section: "tutorial", Body: prog}); err != nil {
		t.Fatal(err)
	}
	if err := c.AddPage(&Page{URL: c.URL("x/"), Title: "X"}); err == nil {
		t.Error("duplicate URL accepted")
	}
	html, err := os.ReadFile(filepath.Join(out, "docs", "v0.2", "x", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"local build", `name="synod"`, `aria-current="page"`, "Errors", "version-switch", "/docs/versions.json"} {
		if !strings.Contains(string(html), want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

// TestSynodHelpers runs the native helpers when `fetch` has been done.
func TestSynodHelpers(t *testing.T) {
	bin, err := filepath.Abs("../../.deps/v0.2/bin/synod")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("run `go run ./tools/build fetch` first")
	}
	s := &Synod{Bin: bin}
	pkgs, err := s.StdPackages()
	if err != nil || len(pkgs) < 10 {
		t.Fatalf("std packages: %v, %v", pkgs, err)
	}
	doc, err := s.DocJSON("std/net")
	if err != nil || !strings.Contains(string(doc), "synod-doc/1") {
		t.Fatalf("doc: %v", err)
	}
	if _, err := s.must("", nil, "check", "/no/such/dir"); err == nil || !strings.Contains(err.Error(), "exit") {
		t.Errorf("failure should report the exit code and output: %v", err)
	}
}

func TestCtxPathsAbsolute(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &Fetcher{Dir: ".deps"}
	c, err := NewCtx("../..", "_site", d, &d.Versions[0], nil, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{c.Synod.Bin, c.Root, c.Out, c.Source("dsl")} {
		if !filepath.IsAbs(p) {
			t.Errorf("%q is not absolute", p)
		}
	}
}

func TestFetchLockSerialises(t *testing.T) {
	f := &Fetcher{Dir: t.TempDir()}
	unlock, err := f.lock()
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		u, err := f.lock()
		if err == nil {
			u()
		}
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("second lock did not wait")
	case <-time.After(300 * time.Millisecond):
	}
	unlock()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("second lock never acquired")
	}
}

func TestProgramFilesJSON(t *testing.T) {
	d, err := LoadDeps("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCtx("../..", t.TempDir(), d, &d.Versions[0], nil, &Fetcher{Dir: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"p/main.synod": "# package main\n</script><b>&\"q\"\n", "p/synod.mod": "module x\n"}
	html, err := c.Layout.Program(ProgramData{ID: "f1", Program: "p", Op: "run", Files: files, Show: []string{"p/main.synod"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	start := strings.Index(s, `id="f1">`) + len(`id="f1">`)
	end := strings.Index(s[start:], "</script>")
	if start < len(`id="f1">`) || end < 0 {
		t.Fatalf("no script body in %s", s)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(s[start:start+end]), &got); err != nil {
		t.Fatalf("script body is not the JSON object: %v\n%s", err, s)
	}
	if got["p/main.synod"] != files["p/main.synod"] || len(got) != 2 {
		t.Errorf("got %v", got)
	}
}
