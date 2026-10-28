package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Synod runs the pinned native compiler. Every command runs in a temporary
// copy of its input, so the checkouts are never written to (`synod get`
// makes vendor/ and synod.sum).
type Synod struct {
	Bin string
}

// Result is one finished command. A nonzero Code is not an error: check and
// run exit 1 on diagnostics, which pages show.
type Result struct {
	Stdout string
	Stderr string
	Code   int
}

// Exec runs `synod args...` in dir.
func (s *Synod) Exec(dir string, args ...string) (Result, error) {
	cmd := exec.Command(s.Bin, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Minute):
		cmd.Process.Kill()
		<-done
		return Result{}, fmt.Errorf("synod %s: timed out", strings.Join(args, " "))
	}
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		res.Code = ee.ExitCode()
	default:
		return res, err
	}
	return res, nil
}

// must runs a command that may only exit with one of ok (default 0) and
// returns its stdout; any other exit is an error that includes stderr.
func (s *Synod) must(dir string, ok []int, args ...string) (Result, error) {
	res, err := s.Exec(dir, args...)
	if err != nil {
		return res, fmt.Errorf("synod %s: %v", strings.Join(args, " "), err)
	}
	if len(ok) == 0 {
		ok = []int{0}
	}
	for _, c := range ok {
		if res.Code == c {
			return res, nil
		}
	}
	return res, fmt.Errorf("synod %s: exit %d: %s", strings.Join(args, " "), res.Code, strings.TrimSpace(res.Stderr+res.Stdout))
}

// WithCopy copies the tree src to a temporary directory, calls fn with it
// and removes it. fn gets the copy's root.
func WithCopy(src string, fn func(dir string) error) error {
	tmp, err := os.MkdirTemp("", "synod-site-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := CopyTree(src, tmp); err != nil {
		return err
	}
	return fn(tmp)
}

// WithFiles writes files (slash path to text) to a temporary directory,
// calls fn with it and removes it.
func WithFiles(files map[string]string, fn func(dir string) error) error {
	tmp, err := os.MkdirTemp("", "synod-site-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for p, text := range files {
		if err := writeFile(filepath.Join(tmp, filepath.FromSlash(p)), []byte(text), 0o644); err != nil {
			return err
		}
	}
	return fn(tmp)
}

// CopyTree copies the files of src into dst byte for byte, without .git.
// Symbolic links are refused.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s is a symbolic link", p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return writeFile(target, data, info.Mode().Perm())
	})
}

// StdPackages lists the std packages, e.g. "std/net": the `std/` lines of
// plain `synod doc` run in an empty directory.
func (s *Synod) StdPackages() ([]string, error) {
	var pkgs []string
	err := WithFiles(nil, func(dir string) error {
		res, err := s.must(dir, nil, "doc")
		if err != nil {
			return err
		}
		for _, line := range strings.Split(res.Stdout, "\n") {
			if f := strings.Fields(line); len(f) > 0 && strings.HasPrefix(f[0], "std/") {
				pkgs = append(pkgs, f[0])
			}
		}
		return nil
	})
	sort.Strings(pkgs)
	return pkgs, err
}

// DocJSON returns `synod doc --json <pkg>` (synod-doc/1) for a std package.
func (s *Synod) DocJSON(pkg string) ([]byte, error) {
	var out []byte
	err := WithFiles(nil, func(dir string) error {
		res, err := s.must(dir, nil, "doc", "--json", pkg)
		out = []byte(res.Stdout)
		return err
	})
	return out, err
}

// DocScope returns `synod doc --json --scope file` run in the module copy at
// root (synod-doc/1 of the packages visible from file).
func (s *Synod) DocScope(root, file string) ([]byte, error) {
	var out []byte
	err := WithCopy(root, func(dir string) error {
		res, err := s.must(dir, nil, "doc", "--json", "--scope", file)
		out = []byte(res.Stdout)
		return err
	})
	return out, err
}

// ExplainJSON returns `synod explain --json` (synod-explain/1, all codes).
func (s *Synod) ExplainJSON() ([]byte, error) {
	var out []byte
	err := WithFiles(nil, func(dir string) error {
		res, err := s.must(dir, nil, "explain", "--json")
		out = []byte(res.Stdout)
		return err
	})
	return out, err
}

// CheckTokens runs `check --json --tokens --style short` on the module in
// dir and returns the synod-check/1 document (exit 0 and 1 both give one).
func (s *Synod) CheckTokens(dir string) ([]byte, error) {
	res, err := s.must(dir, []int{0, 1}, "check", "--json", "--tokens", "--style", "short", ".")
	return []byte(res.Stdout), err
}

// CheckRich runs `check --style rich --color never` on dir and returns the
// rendered diagnostics with the exit code.
func (s *Synod) CheckRich(dir string) (Result, error) {
	return s.must(dir, []int{0, 1}, "check", "--style", "rich", "--color", "never", ".")
}

// RunScene runs program in dir and writes its scene to scene.
func (s *Synod) RunScene(dir, program, scene string, extra ...string) error {
	args := append([]string{"run", program, "--scene", scene}, extra...)
	_, err := s.must(dir, []int{0}, args...)
	return err
}

// RunSummary runs `run <program> --summary --style short` and returns the
// result; exit 0 and 1 are both normal (a failing run has a diagnostic).
func (s *Synod) RunSummary(dir, program string, extra ...string) (Result, error) {
	args := append([]string{"run", program, "--summary", "--style", "short"}, extra...)
	return s.must(dir, []int{0, 1}, args...)
}

// Get runs `synod get` in dir, vendoring the modules of its synod.sum.
func (s *Synod) Get(dir string) error {
	_, err := s.must(dir, nil, "get")
	return err
}

// Version runs `synod version`.
func (s *Synod) Version() (string, error) {
	res, err := s.must("", nil, "version")
	return res.Stdout, err
}

// jsonDoc decodes a document into v, naming what it is on error.
func jsonDoc(what string, data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %v", what, err)
	}
	return nil
}

// DocIn returns `synod doc --json <pkg>` run in the module at dir.
func (s *Synod) DocIn(dir, pkg string) ([]byte, error) {
	res, err := s.must(dir, nil, "doc", "--json", pkg)
	return []byte(res.Stdout), err
}
