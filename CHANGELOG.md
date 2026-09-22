# Changelog

All notable changes to ctxpack are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project aims
to follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **The budget reports what it cut.** `format.Bundle` gained `Omitted` (paths)
  and `OmittedTokens`, both `omitempty`, so an unlimited pack keeps its exact
  shape. XML renders an `<omitted count="…" tokens="…">` element after
  `<files>`, Markdown appends an "Omitted by budget" section, and the text
  renderer prints an `==== omitted by budget ====` block. `ctxpack pack
  --output f` now says `…; 12 files, ~8210 tokens omitted by the budget` on
  stderr instead of a bare file count.

  `LICENSE` and the rest of the policy documents used to rank dead last, so a
  tight budget happily filled up on source code and never saw the license. The
  documented tiers (`*.proto`/`*.graphql`/`*.thrift`, `server.js`, `manage.py`,
  `*.rst`, `Dockerfile`, `Makefile`, `test_*.py`) were also missing and scored
  10. They are implemented now, and the README's tier table gained the scores.

### Fixed

- **`--budget` ranked every test as source code.** After reordering the
  priority ladder, the `_test.go`/`.test.js`/`.spec.ts`/`.test.ts` case was
  placed after the general source-code case, so `_test.go` was matched by
  `.go` and scored 200 instead of 50. Nothing was left to rank last, and the
  "tests last" guarantee was false. The test tier is now matched before
  source, with a comment recording why the order is load-bearing.

- **`map` could lose a file that shared its name with a directory.** A file and
  a directory may legally share a name on Linux and macOS, and the tree folder
  matched children on the name alone, so it folded the two into one node: the
  file stopped appearing as an entry and its size was stacked on top of the
  directory's subtree total. The lookup key is now the name plus the kind.

  ```text
  old (matched on the name alone)      new (matched on name plus kind)
  repo/  [33 B]                        repo/  [33 B]
    a  [33 B]                            a  [12 B]          <- the file
      b.go  [21 B]                      a/  [21 B]          <- the directory, subtree only
                                            b.go  [21 B]
  ```

  `fold` was split out of `Build` so the case can be exercised on every
  platform; on Windows the filesystem forbids the collision, so the
  integration test there still skips.

- **The XML and Markdown renderers could not be read back exactly.** `<content>`
  was pretty-printed with a newline before the CDATA and a newline plus
  indentation after it, so parsing the bundle back out gave every file a
  leading and a trailing blank line instead of its own bytes; the Markdown
  fence gained a newline for every file, terminated or not. Both now round-trip
  byte for byte. (XML still normalizes carriage returns to line feeds, but that
  is required by the XML specification and applies to every parser — markdown,
  JSON and text do preserve CRLF.)

## [0.1.1] - 2026-09-22

Bug-fix release. Nothing new to write — eleven things that were plainly
wrong, plus the tests that would have caught each of them.

### Fixed

- **`pack` and `tokens` over-reported every byte count by 1024x.** `a.txt` at
  10 bytes reported as 10 KB. The threshold ladder divided by 1024 but the
  label formatter had already multiplied.
- **`pack --format xml` could emit XML that does not parse.** File paths and
  the `tokens`/`bytes` attributes were not escaped, and a `]]>` inside a file
  body terminated its CDATA section. Both now round-trip through a real
  XML parser, including a file literally named `a&b/x < y.go`.
- **`pack --format foo` silently produced XML.** An unknown format is now
  rejected with a non-zero exit; `CTXPACK_FORMAT` is validated the same way.
- **`map` rendered the tree root as `./`.** The tree is read relative to the
  walked directory, so the root is now labelled with the directory's own
  name.
- **`diff` could not see renames or quoted paths.** `git status --name-status`
  output is split on tabs now (a rename's whole line was previously treated as
  one path) and git's C-style path quoting is unquoted.
- **`mcp`'s `repo_map` under-counted by orders of magnitude.** When content is
  not read, tokens are estimated from file size rather than from the file's
  own name — a 40 KiB file read as roughly one token.
- **A boolean flag made every flag after it disappear.** `--format json --budget 500`
  dropped `--budget`; the flag reordering now respects which flags take a value.
- **`mcp` returned a generic error when `tools/call` had no tool name.** It now
  reports `tools/call is missing arguments.name`.
- **Flags after the path were silently ignored.** Go's `flag` package stops
  parsing at the first non-flag token, so `ctxpack pack ./repo --format
  markdown` produced XML. Positional arguments are moved to the end of the
  argument slice before parsing, so `ctxpack <command> [path] [flags]` works
  in either order.
- **`--version` printed `gogo1.26.5`.** `runtime.Version()` already carries a
  `go` prefix.
- **The documented `-o` shorthand did not exist.** It is now registered on
  `pack` and `diff`.

### Tests

Coverage went from thin to honest on the packages that were untested:

| package   | before | after |
| --------- | ------ | ----- |
| version   |  0.0%  |100.0% |
| format    |  0.0%  | 98.8% |
| repomap   |  0.0%  | 98.2% |
| gitutil   |  0.0%  | 95.4% |
| mcp       |  0.0%  | 92.9% |
| walker    | 73.8%  | 91.3% |
| cli       |  0.0%  | 56.7% |

The full suite is green under `go test ./...`, `go vet ./...` and `gofmt -l .`.

## [0.1.0] - 2025-01-01

First public release.

### Added

- **`ctxpack pack`** — pack a repository into one LLM-ready bundle with an
  optional token budget. Files that do not fit are ranked by context value
  (READMEs and entry points first, tests and generated scaffolding last) and
  listed as omitted rather than dropped silently.
- **`ctxpack map`** — token-aware tree outline with per-file and per-directory
  estimates, so you can see where a budget will go before spending it.
- **`ctxpack diff`** — pack only the files changed against a git ref, for
  change sets instead of whole trees. Deleted files are listed without content.
- **`ctxpack tokens`** — total token estimate with a per-model fit table.
- **`ctxpack models`** — the model registry with context windows.
- **`ctxpack mcp`** — Model Context Protocol server over stdio exposing
  `pack_repo`, `repo_map` and `count_tokens`.
- **Four renderers** — XML (CDATA), Markdown (fence-safe), JSON, and plain
  text, all escaping file bodies in a way that survives arbitrary source.
- **Token estimator** — tiktoken-style pre-tokenisation plus a calibrated
  per-chunk heuristic, targeting ±15% of real `o200k_base` counts. No network
  access, no embedded model data.
- **Gitignore-aware walker** — layered `.gitignore` matching, a built-in
  denylist for VCS/dependency/build directories, binary detection, and
  dotfiles excluded by default.
- **Model registry** — OpenAI, Anthropic, Google, Meta, Mistral, DeepSeek and
  Qwen with exact and prefix matching.

### Design

- **Stdlib only.** Zero third-party modules; `go.sum` is empty.
- **Single static binary**, offline-capable.
- **Shared engine** — CLI and MCP server call the same
  `packer`/`repomap`/`gitutil` packages.
- **Read-only** — no code path writes into the target tree.

[0.1.1]: https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.1
[0.1.0]: https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.0
