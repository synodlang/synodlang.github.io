# synodlang.github.io

The Synod website and documentation, served at https://synodlang.github.io.

[![ci](https://github.com/synodlang/synodlang.github.io/actions/workflows/ci.yml/badge.svg)](https://github.com/synodlang/synodlang.github.io/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

## What is here today

- `index.html`: a placeholder home page with one sentence and two links.
- `install/index.html`: the install page. It shows the commands for macOS, Linux, Windows and Go, direct download links filled from the latest release, and a note on unsigned binaries and `synod version`.
- `install.sh`: the install script, served at https://synodlang.github.io/install.sh. It downloads the latest `synod` release for macOS or Linux, verifies the checksums and installs to `$HOME/.local/bin` without sudo. `SYNOD_VERSION` pins a version and `SYNOD_INSTALL` sets the directory.
- `.nojekyll`: tells GitHub Pages to serve the files as they are.

The pages are plain HTML with inline CSS. There is no build step yet.

## What is planned

The full site is milestone M11 in the [roadmap](https://github.com/synodlang/dsl/blob/main/implementation/10-roadmap.md). It is built from `dsl` and `std` at deploy time, so it always matches a tagged specification. The plan is in [09-site-and-org.md](https://github.com/synodlang/dsl/blob/main/implementation/09-site-and-org.md) and [11-documentation.md](https://github.com/synodlang/dsl/blob/main/implementation/11-documentation.md).

| Section | What it will be |
| --- | --- |
| Home | The icon, one sentence, a looping Raft election played live by the player, and three buttons: Start the tutorial, Install, See examples |
| Tutorial | Fifteen chapters, bottom up, each a runnable program, ending with Raft leader election |
| Reference | The specification, one page per chapter |
| Library | One page per standard library package, with a runnable example per phrase |
| Examples | One page per example, with editable source and a Run button |
| Guides | Install the CLI, use the Mac app, write a look, publish a package, use Synod with an agent |
| Errors | One page per diagnostic code: the message, a program that triggers it, the fix |
| Design | Why Synod is the way it is |
| Tribute | Leslie Lamport's "The Part-Time Parliament" and the name |

Every program on the site will have a Run button. It calls `synod.wasm` (check, run and scene) in the page and plays the result with the [player](https://github.com/synodlang/player), with diagnostics shown inline. There will be no recordings. Docs will be versioned by `dsl` tag, and search will be static.

Those pieces depend on the WebAssembly build (M6) and the player (M7), which are not released yet.

## The organization

| Repository | What it is | Status |
| --- | --- | --- |
| [dsl](https://github.com/synodlang/dsl) | The language specification and implementation guides | Spec v0.2 |
| [compiler](https://github.com/synodlang/compiler) | The `synod` command: `fmt`, `check`, `run` | v0.2.2 released |
| [std](https://github.com/synodlang/std) | The standard library packages | In use by the compiler |
| [examples](https://github.com/synodlang/examples) | Sample programs and acceptance tests | Available |
| [player](https://github.com/synodlang/player) | Canvas library that plays `scene.json` | Planned (M7) |
| [mac-app](https://github.com/synodlang/mac-app) | The Mac app | Planned (M8, M9, M10) |
| [agent-skills](https://github.com/synodlang/agent-skills) | Skills for coding agents | Planned (M10) |
| **synodlang.github.io** (this repository) | The website and documentation | Install page live; rest planned (M11) |
| [homebrew-tap](https://github.com/synodlang/homebrew-tap) | Homebrew formula for `synod` | Updated by compiler releases |
| [scoop-bucket](https://github.com/synodlang/scoop-bucket) | Scoop manifest for `synod` | Updated by compiler releases |
| [.github](https://github.com/synodlang/.github) | Organization profile and shared workflows | Available |

## Preview locally

The site is static files. From the repository root:

```
python3 -m http.server --directory .
```

Then open http://localhost:8000. Links to `/install/` and `/install.sh` work from the root of that server.

## Deployment

GitHub Pages serves the `main` branch of this repository at https://synodlang.github.io. Merging to `main` publishes. The CI job checks line endings, trailing spaces and the final newline of `*.md`, `*.ts`, `*.js`, `*.json`, `*.css` and `*.html` files.

## Design

The site uses the brand from the language icon: navy ground `#151A2E`, pale `#EDE9E1`, amber accent `#F4A340`, and JetBrains Mono for the wordmark and code.

## Contributing

Keep pages small and plain. Use LF line endings, no trailing spaces and a final newline. Changes to the language itself start as a pull request to [dsl](https://github.com/synodlang/dsl).

## Why Synod

Synod is named after the Synod protocol, the core consensus algorithm in Leslie Lamport's paper "The Part-Time Parliament" (ACM Transactions on Computer Systems, 1998; first submitted in 1990). The paper tells it as the story of the parliament of the ancient Greek island of Paxos, whose part-time legislators could agree on decrees although they came and went; today the algorithm is known as Paxos. Lamport later explained it plainly in "Paxos Made Simple" (2001), and in 2013 he received the ACM A.M. Turing Award for fundamental contributions to the theory and practice of distributed and concurrent systems. His "Time, Clocks, and the Ordering of Events in a Distributed System" (1978) and his TLA+ specification language shaped how we reason about such systems. We are grateful for his work, and we hope Synod honours it by making these ideas easier to watch and understand. The website tells this story in its own words.

Implements Synod 0.2 (specification: https://github.com/synodlang/dsl).

Licensed under the [Apache License 2.0](LICENSE).
