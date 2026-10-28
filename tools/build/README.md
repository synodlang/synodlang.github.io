# Site build

`go run ./tools/build fetch|build|check|serve [-out _site] [-local dsl=../dsl,examples=../examples,synod=path]`

Standard library only. `fetch` downloads what `deps.json` pins into `.deps/<docs>/`
(release assets checked against the release `SHA256SUMS` and the pin, repositories
checked out byte-exact at the pinned commit, the host `synod` extracted and its
`synod version` compared with the pin). `build` writes `_site/`; `check` verifies it;
`serve` serves it on 127.0.0.1:8000. `-local` replaces a source by a sibling tree
(`dsl`, `std`, `examples`, `looks`), the compiler binary (`synod`), the browser files
(`wasm`, `wasmexec`: the .wasm and wasm_exec.js files), or the player's `dist` folder (`player`:
`synod-player.js`, `.d.ts`, `synod-player-fonts.tar.gz`), and adds a "local build" banner to
every page. Overrides apply to the latest docs version only; older versions build from their pins.
The v0.3 entry of deps.json holds all-zero placeholder pins (its `todo` field) until the v0.3.0
release step fills them; `fetch` refuses them, so build v0.3 with `-local` until then:

    go run ./tools/build build -local dsl=../dsl,std=../std,examples=../examples,looks=../looks,synod=PATH,wasm=PATH.wasm,wasmexec=PATH-wasm_exec.js,player=../player/dist

Fonts: `web/fonts` holds the four families of the player fonts asset (Inter, JetBrains Mono,
Source Serif 4, Shantell Sans) and their OFL texts; the build compares them byte for byte with the
pinned asset, serves them as the player's `fontBase` and waits for `fontsReady` before a first paint.
`pages_looks.go` builds `looks/` and `looks/gallery/` (v0.3 and later).

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

## Program widget contract

```html
<meta name="synod" content='{"assets":"/docs/v0.2/assets/","wasm":"…","wasmExec":"…","player":"…","errors":"/docs/v0.2/errors/"}'>
<div class="synod-program" data-program="raft" data-op="run|check" data-show="raft/raft.synod" data-predict="on|off"
     data-files="#f1" | data-files-url="assets/examples-v0.2.1.json" [data-files-prefix="raft/"]
     [data-vendor-url="assets/examples-v0.2.1.json" data-vendor-prefix="vendor/github.com/synodlang/examples@v0.2.1/"]>
  <script type="application/json" id="f1">{"path":"text"}</script>
  <pre class="synod" data-file="raft/raft.synod" data-changed="3,4"><code>…</code></pre>
</div>
```

The program's files come from `data-files` (inline JSON, a `<script>` id) or
`data-files-url` (a shared JSON file; `data-files-prefix` keeps only the files under the
prefix plus `synod.mod` and `synod.sum`). `data-vendor-url` adds a second shared JSON
file: every file of it is merged under `data-vendor-prefix`, the widget's own files
winning on a clash. The build uses it for the vendored copy of the examples module in
tutorial steps and Reference snippets that import it (`shareVendor`, pages_examples.go),
so the module is downloaded once per page (`assets/examples-<tag>.json`, cached) instead
of being inlined into every widget. Vendored files that differ from the module stay inline.
`check` resolves both sources when it verifies that a widget's program is a folder of its files.
