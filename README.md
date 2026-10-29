# synodlang.github.io

The Synod website and documentation, served at https://synodlang.github.io.

[![ci](https://github.com/synodlang/synodlang.github.io/actions/workflows/ci.yml/badge.svg)](https://github.com/synodlang/synodlang.github.io/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

## What this is

The site is a set of static pages that a small Go program builds from what the other repositories published. Nothing is copied by hand: the tutorial, the specification, the standard library pages and the diagnostic codes all come from tagged releases, so the site always matches a released Synod.

| Section | What it is |
| --- | --- |
| Home | The icon, one sentence, a looping Raft election played live by the player, and three buttons |
| Tutorial | Fifteen chapters, bottom up, each step a runnable program, ending with Raft leader election |
| Reference | The specification, one page per chapter |
| Library | One page per standard library package, with a runnable example per phrase |
| Examples | One page per example program, with editable source and a Run button |
| Guides | Install the CLI, use the Mac app, write a look, publish a package, use Synod with an agent |
| Errors | One page per diagnostic code: the message, a program that triggers it, the fix |
| Design | Why Synod is the way it is, and the decisions |
| Tribute | Leslie Lamport's "The Part-Time Parliament" and the name |

Every program has a Run button. It calls `synod.wasm` (check, run, format) in a Web Worker and plays the result with the [player](https://github.com/synodlang/player). There are no recordings. Docs are versioned: `/docs/v0.2/` and `/docs/v0.3/` are each built from that version's pins and `/docs/` leads to the latest. Search is a static `search.json` per version.

`install/` and `install.sh` are served as they are in this repository (the install page and the install script, https://synodlang.github.io/install.sh), and the build checks that the copies are byte-identical.

## How the site is built

`deps.json` says what a docs version is made of: the `dsl`, `std` and `examples` tags with their commits, and the `compiler` and `player` tags with the SHA-256 of each release asset used. The build is standard-library Go in `tools/build` (see its [README](tools/build/README.md)); there is no package manager in it.

```
go run ./tools/build fetch                  # download and verify what deps.json pins into .deps/
go run ./tools/build build -out _site       # build the site
go run ./tools/build check -out _site       # check the built site
go run ./tools/build serve -out _site       # serve it on http://127.0.0.1:8000
```

- `fetch` checks every release asset against the release `SHA256SUMS` and the pin, checks the repositories out byte-exact at the pinned commit, and runs the pinned `synod` (`synod version` must match the pin).
- `build` writes `_site/`. Highlighting, diagnostics, the Errors examples and the home scene come from the native pinned `synod`; editing in the browser uses the pinned `synod.wasm`.
- `check` fails when an internal link or anchor does not resolve, when the Errors pages differ from the codes of `synod explain --json` or of the specification, when a tutorial chapter lacks a widget for a step, when the example pages differ from the table of `examples`, when a Library page lacks an example, when a widget's `data-program` is not a folder in its files, when there is a `.gif`, `.mp4`, `.webm`, `.mov`, `.apng` or `<video>`, when `install/` or `install.sh` differ from the source, or when an asset does not hash to its pin. Checks of content the pinned `dsl` does not have yet are skipped with a note.

### Preview your own changes

To build from sibling checkouts instead of the pins, for example while writing a chapter in `dsl`:

```
go run ./tools/build build -out _site -local dsl=../dsl,std=../std,examples=../examples
go run ./tools/build check -out _site -local dsl=../dsl,std=../std,examples=../examples
go run ./tools/build serve -out _site
```

`-local` also takes `synod=path/to/synod` to use another compiler binary. Every page of a local build shows a "local build" banner. Run `fetch` once first (it still downloads the compiler, the wasm and the player).

### Tests

```
gofmt -l . && go vet ./... && go test ./...
```

The browser tests are Playwright (pinned in `test/browser`). They read the pages of the built `_site`, so a new chapter or example is tested without editing them: on every tutorial and example page each widget is run, a line is edited and it is run again; an Errors page shows its diagnostic; search finds a page; `/docs/` leads to the latest docs.

```
cd test/browser
npm ci
npx playwright install chromium
npx playwright test               # SITE_DIR=... to test another directory
```

### Deployment

`.github/workflows/pages.yml` runs on every push to `main` (and by hand): it fetches, builds, checks and deploys `_site` with GitHub Actions to GitHub Pages. `.github/workflows/ci.yml` checks the text files (line endings, trailing spaces, final newline), `gofmt`, `go vet`, `go test`, builds and checks the site, and runs the browser tests.

## Design

The site uses the brand from the language icon: navy ground `#151A2E`, pale `#EDE9E1`, amber accent `#F4A340`, and JetBrains Mono (self-hosted) for the wordmark and code.

## The organization

| Repository | What it is |
| --- | --- |
| [dsl](https://github.com/synodlang/dsl) | The language specification, the tutorial and the implementation guides |
| [compiler](https://github.com/synodlang/compiler) | The `synod` command and `synod.wasm` |
| [std](https://github.com/synodlang/std) | The standard library packages |
| [examples](https://github.com/synodlang/examples) | Sample programs and acceptance tests |
| [player](https://github.com/synodlang/player) | Canvas library that plays `scene.json` |
| [mac-app](https://github.com/synodlang/mac-app) | The Mac app |
| [agent-skills](https://github.com/synodlang/agent-skills) | Skills for coding agents |
| **synodlang.github.io** (this repository) | The website and documentation |
| [homebrew-tap](https://github.com/synodlang/homebrew-tap) | Homebrew formula for `synod` |
| [scoop-bucket](https://github.com/synodlang/scoop-bucket) | Scoop manifest for `synod` |
| [.github](https://github.com/synodlang/.github) | Organization profile and shared workflows |

## Contributing

Keep pages small and plain. Use LF line endings, no trailing spaces and a final newline. The text of the tutorial, the reference and the Library lives in `dsl` and `std`; changes to the language itself start as a pull request to [dsl](https://github.com/synodlang/dsl). To update the pins, change `deps.json` (tags, commits and asset hashes) and run `fetch`, which refuses anything that does not match.

## Why Synod

Synod is named after the Synod protocol, the core consensus algorithm in Leslie Lamport's paper "The Part-Time Parliament" (ACM Transactions on Computer Systems, 1998; first submitted in 1990). The paper tells it as the story of the parliament of the ancient Greek island of Paxos, whose part-time legislators could agree on decrees although they came and went; today the algorithm is known as Paxos. Lamport later explained it plainly in "Paxos Made Simple" (2001), and in 2013 he received the ACM A.M. Turing Award for fundamental contributions to the theory and practice of distributed and concurrent systems. His "Time, Clocks, and the Ordering of Events in a Distributed System" (1978) and his TLA+ specification language shaped how we reason about such systems. We are grateful for his work, and we hope Synod honours it by making these ideas easier to watch and understand. The website tells this story in its own words.

Implements Synod 0.3 (specification: https://github.com/synodlang/dsl).

Licensed under the [Apache License 2.0](LICENSE).
