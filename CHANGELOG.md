# Changelog

All notable changes to ctxpack are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project aims
to follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **The ignore matcher is pinned down.** `internal/ignore` went from 81.1%
  to 97.0% coverage. New direct tests cover the loader (blank lines, commented
  lines, whitespace-only lines, trailing and leading whitespace, backslash
  escaping of `#` and `!`, an unreadable input surfacing as an error, a missing
  file being silent) and, for the first time, the scoping contract the walker
  only exercised end-to-end: a pattern loaded from `web/.gitignore` must not
  apply to `app.js` at the root, a root pattern still applies inside `web/`, a
  nested negation overrides the root exclusion, and the last matching pattern
  wins. Also covered: `?` not crossing a separator, every `**` form, leading
  `/` anchoring, every regex-special character being emitted literally,
  directory-only patterns not ignoring a file of the same name, exclusion
  propagating through three or more levels, and `Pattern.String()`.

  The two remaining uncovered lines are the `filepath.Abs` error fallback and
  the `os.Open` error return, both reachable only from a filesystem that
  refuses a call the tests cannot produce. The `regexp.Compile` error branch
  is documented as unreachable: `translateGlob` escapes every regex-special
  character, so its output can never fail to compile.

- **The estimator is pinned down.** `internal/counter` went from 74.4% to
  95.3% coverage. New tests pin the pre-tokenization contract (contractions
  split as `don` + `'t`, digits cap at three per chunk, a leading space is
  absorbed by the following class, whitespace and punctuation runs collapse
  into single chunks, non-ASCII letters stay letters, emoji are symbols), the
  `EstimateSize`/`EstimateBytes` tables, the model registry, and the
  `FitsModel` boundary. The two remaining uncovered lines are the `est < 1`
  clamp and the bytes/4 floor, both provably unreachable: no alternative in
  the pattern matches the empty string, so `n` is always at least 1, and the
  per-chunk sum is always at least bytes/3.5. `TestEstimateNeverYieldsEmptyChunks`
  and `TestEstimateFloorDoesNotBind` assert that, so a formula change that
  would make either guard start firing fails loudly. Both stay in place as
  insurance against exactly that.

  The same test surfaced the honest gap in `EstimateSize`: the estimate from
  content runs at roughly one token per pre-token, so chunk-dense code lands
  at 170–210% of the bytes/4 the size heuristic uses (11 tokens for
  `func Foo() int {\n\treturn 0\n}\n` against 7), while ASCII prose sits about
  33% higher. A repo map built with `ReadContent:false` therefore under-reports
  the same files the pack reports with content, by up to a factor of two. It
  stays at "the same magnitude" as documented, and `TestEstimateSizeMirrorsEstimate`
  now records the measured ratios so the skew cannot drift further unnoticed.

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

- **`a/**/b` matched `a/xby`.** `**/` was translated by consuming the
  following slash and emitting `.*`, so the separator between the two glob
  parts was lost: `a/**/b` compiled to `a/.*b`. That matches `a/b`, `a/x/b`
  and `a/x/y/b` as intended, but also `a/xby` and `a/xb`, which git does not
  match. `**/` now becomes `(?:.*/)?` — zero or more intermediate directories
  with the separator preserved. `a/**` still becomes `a/.*`.

- **Indented comments became patterns.** Git strips leading spaces and tabs
  unless they are escaped with a backslash, so `  # comment` is a comment.
  ctxpack only trimmed trailing whitespace, so an indented comment line became
  a real pattern — and an indented `!foo` silently turned into a negation that
  could re-include something no one asked to keep. Leading whitespace is now
  stripped, and a leading backslash still protects it (`\#name` matches a file
  literally named `#name`).

- **`Match("")` reported "ignored".** `*` and `**` compile to regexes that
  match the empty string, so an empty relative path returned ignored for any
  glob. There is no such path in practice — the walker only passes real entries
  — but `Match` is exported and documented to take a root-relative path, so an
  empty string now returns `false` explicitly.

- **`--model ""` matched `gpt-3.5-turbo`.** `LookupModel` fell back to a
  bidirectional prefix match, and `strings.HasPrefix(name, "")` is true for
  every registered model, so an unset or empty value resolved to whatever came
  first in the registry — `gpt-3.5-turbo`'s 16,385-token window, the smallest
  on the list. `ctxpack pack --model ""` printed a fit line against the wrong
  model, and an MCP client that passed an empty string got silently mis-sized.
  An empty query now matches nothing.

- **`EstimateSize` returned a negative token count for a negative size.** With
  `n` negative, `(n+3)/4` rounds toward zero and produced `-1` for `-8` and
  worse below that, which would have driven a bundle's totals backwards. Sizes
  are now clamped at zero.

- **The "±15%" accuracy claim was not earned.** The README and the
  `counter` package doc both promised ±15% of real `o200k_base` tokenization,
  which had never been measured against a real tokenizer — the tool ships
  offline by design, so there was nothing to measure it against. Measured
  ratios against the bytes/4 baseline are 133% for ASCII prose and 170–210%
  for chunk-dense code, with `TestEstimateSizeMirrorsEstimate` recording them
  so they cannot drift unnoticed. Both documents now describe the heuristic
  and the measured skew instead of asserting an unverified precision.
  `SECURITY.md` was updated the same way. The [0.1.0] entry above is left as
  it stood at release time.

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
