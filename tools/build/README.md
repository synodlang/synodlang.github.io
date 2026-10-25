# Site build

`go run ./tools/build fetch|build|check|serve [-out _site] [-local dsl=../dsl,examples=../examples,synod=path]`

Standard library only. `fetch` downloads what `deps.json` pins into `.deps/<docs>/`
(release assets checked against the release `SHA256SUMS` and the pin, repositories
checked out byte-exact at the pinned commit, the host `synod` extracted and its
`synod version` compared with the pin). `build` writes `_site/`; `check` verifies it;
`serve` serves it on 127.0.0.1:8000. `-local` replaces a source by a sibling tree
(`dsl`, `std`, `examples`) or the compiler binary (`synod`) and adds a "local build"
banner to every page.

## Files

| File | Owns |
| --- | --- |
| main.go | commands and flags, the builder order |
| deps.go, fetch.go | deps.json, fetch and verification |
| synod.go | the native `synod` run in temporary copies |
| ctx.go, layout.go, gens.go | `Ctx`, page registry, shell templates, generated top-level files |
| md.go | Markdown subset to HTML |
| pages_*.go, diff.go, tokens.go, links.go, search.go, check.go | filled in by phase D |

Templates are in `templates/` (`layout.html` shell, `page.html` article, `program.html` widget).

## Signatures phase D fills in (do not change them)

```go
func buildHome(c *Ctx) error      // pages_home.go: "/" (latest only), home-raft.json scene, docs home
func buildTutorial(c *Ctx) error  // pages_tutorial.go
func buildReference(c *Ctx) error // pages_reference.go
func buildDesign(c *Ctx) error    // pages_design.go
func buildLibrary(c *Ctx) error   // pages_library.go
func buildExamples(c *Ctx) error  // pages_examples.go
func buildErrors(c *Ctx) error    // pages_errors.go
func buildGuides(c *Ctx) error    // pages_guides.go
func buildTribute(c *Ctx) error   // pages_tribute.go (latest only)

func diffLines(prev, cur string) []int                                         // diff.go
func moduleTokens(c *Ctx, dir string) (map[string][]Token, error)              // tokens.go
func renderCode(text string, toks []Token, changed []int) template.HTML        // tokens.go
func rewriteHref(c *Ctx, page, href string) (rewritten string, ok bool)        // links.go
func buildSearch(c *Ctx) error                                                 // search.go
func checkSite(c *Ctx) error                                                   // check.go
```

`checkSite` already verifies `install/`, `install.sh` and the asset hashes; extend it.
`genDocsIndex` writes a plain page list at `/docs/<version>/` unless a builder added a
page with that URL first.

## What a builder uses

- `c.Source("dsl"|"std"|"examples")` the checkout (or `-local`), `c.Synod` the native compiler
  (`StdPackages`, `DocJSON`, `DocScope`, `ExplainJSON`, `CheckTokens`, `CheckRich`, `RunScene`,
  `RunSummary`, `Get`; `WithCopy`, `WithFiles` for temporary copies).
- `c.AddPage(&Page{URL: c.URL("tutorial/01-hello/"), Title, Section, Body})` renders, writes and
  registers; `c.WriteDocs("assets/x.json", data)` writes a file under the version;
  `c.Layout.Program(ProgramData{...})` renders a program widget.
- `c.IsLatest()` and `c.IsLocal()`; `c.V` is the version entry, `c.Pages` the registry.
