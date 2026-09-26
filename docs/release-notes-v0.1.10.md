# v0.1.10

The release that fixes how the packer decides what a file *is*. `packer.priority`
scores every file before a budget is applied, and three of the fixes below are
matching defects in that one function: each one classified a file as something it
is not, so under a tight budget the wrong files survived. The other seven are
release-plumbing defects — the parts of the pipeline that turn a commit into a
downloadable binary — most of which had never been exercised on any machine other
than the one that wrote them.

## Fixed

- **`readme_helper.py` was scored as a project document.**
  The README check was `strings.HasPrefix(base, "readme")` — any basename
  *beginning* with readme. `readme_helper.py` and `readmes.py` therefore scored
  1000, the policy-document tier, instead of 200 (source), outranking config
  files, scripts and every test. The same function matched `policyDocs` by exact
  basename, where `securityscanner.go` correctly scored 200 against
  `SECURITY.md` at 1000, so the two halves of one rule disagreed: one exact, one
  greedy. `isPolicyDoc` now accepts only the basename `readme` or `readme.<ext>`,
  so `README.md` and `docs/README.rst` still score 1000 while `readme_helper.py`
  and `readmes.py` score 200 and `docs/READMEING.md` scores 300 — a markdown file
  that is not a README.

- **`notmain.go` was scored as an application entry point.**
  Entry-point detection ran `strings.HasSuffix` on the *full* path, and
  `HasSuffix("notmain.go", "main.go")` is true — as are `myindex.js` against
  `index.js`, `mymanage.py` against `manage.py` and `customserver.js` against
  `server.js`. Four ordinary source files therefore scored 400, outranking the
  real source they sat among at 200, so a tight budget spent tokens on
  `notmain.go` before they reached anything the repository actually runs. Entry
  points are now an exact-basename set: `main.go`, `mod.go`, `index.ts`,
  `index.js`, `index.jsx`, `index.tsx`, `server.js`, `manage.py`.

- **React and TSX files scored 10, the lowest tier of all.**
  The source list was `.go .ts .js .py .rs .java .rb .cs` and contained no
  `.tsx` or `.jsx`, so React and TypeScript-JSX components fell through to the
  default 10 — below config files (150), shell scripts (100) and even test files
  (50). Under a budget a component was dropped before a YAML file. Both
  extensions now score 200 as source, and `index.jsx` / `index.tsx` are
  registered as entry points at 400.

- **`ctxpack models --sort vendor` could sort wrong.**
  The comparator compared vendor strings raw for equality but folded them to
  lowercase for ordering, so `OpenAI` and `openai` were treated as *different*
  vendors while ordering as *equal*. That is an inconsistent comparator, and
  `sort.Slice` has no defined result for one — the grouping came out wrong. The
  default (name) sort had the identical defect for case-variant names. Both now
  fold before comparing. Found by writing tests for the two tiebreak branches
  that had never been exercised; the real 31-entry table has distinct names, so
  neither branch had ever run.

- **Both installers could not verify any checksum file.**
  `install.sh` and `install.ps1` matched an asset with a two-space separator,
  which is what `sha256sum -t` emits in text mode. But GNU coreutils defaults to
  **binary mode** when given file arguments, which emits `hash *name` instead.
  `sha256sum --help` states it plainly: `-b, --binary  read in binary mode
  (default unless reading tty stdin)`. So the checksum file that `make release`
  produces on this project's own release host made both installers fail with
  `ctxpack_0.1.10_windows_amd64.exe is not listed in SHA256SUMS.txt` — the
  installers rejected every release, on every platform. Neither had ever been
  executed, so the defect shipped for four releases. Both parsers now accept
  either marker and compare the name after stripping it, and the Makefile pins
  `-t` so the published file is deterministic. The parsers are covered by 8
  offline cases each: binary and text mode, the asset not being first, and two
  prefix traps — `..._amd64.exe` must not match `..._amd64.exe.debug`, and
  `..._386.exe` must not match `..._amd64.exe`.

- **`install.sh` without a version produced a "version" made of release notes.**
  The REST call was piped through `grep -m1 '"tag_name"'`, which returns the
  whole matching line. When the API answers on a single line that line is the
  entire response including the release body, and the greedy `sed` then matched
  inside it, yielding a URL fragment before curl refused the malformed URL.
  `grep -o` extracts just the `"tag_name": "v0.1.10"` fragment, so the `sed` only
  ever sees that.

- **A transient GitHub connection reset aborted an install.**
  `Invoke-WebRequest` has no retry of its own, and the script runs under
  `$ErrorActionPreference = 'Stop'`, so one reset connection killed the install
  with no attempt to recover. `install.ps1` now retries with backoff before
  giving up, and `install.sh` passes `--retry 3` to curl. Under a live reset the
  retry path was observed printing `retry 1/4 ... 3/4` and then rethrowing the
  real error, so a failure still reports clearly.

- **The CI release-binary job could not upload its artifacts.**
  The workflow declares `permissions: contents: read` at the top level, and that
  applies to every job unless a job overrides it. `actions/upload-artifact@v4`
  requires `actions: write`, so the `packages` job would have failed at the very
  last step of a release build. The job now declares both permissions itself
  instead of widening the whole workflow. Found while validating the workflow
  structurally — every step, matrix and permission is checked by a script that
  parses the YAML, and all seven `run:` blocks pass `bash -n`.

- **CI release binaries were named differently from the real release assets.**
  The workflow built `ctxpack-0.1.10-linux-amd64` (hyphens) while `make release`,
  the published assets, and both installers all use
  `ctxpack_0.1.10_linux_amd64` (underscores). A CI artifact therefore could not
  stand in for a release download, and a `sha256sum -c` against the published
  sums file would fail. The naming now matches.

- **A mistyped `-ldflags -X` path is now detected instead of silently ignored.**
  Go accepts a wrong `-X` path with exit 0 and quietly leaves the source default
  in place: building with `-X ...version.Vresion=9.9.9-typo` (one letter wrong)
  and with `-X ...internal/versionz.Version=...` (wrong package) both succeed, and
  both binaries report the source default. That makes a typo in a version stamp
  invisible until a user inspects the output. The test job now builds with a
  sentinel version and fails if it does not appear in `ctxpack version`.

## Added

- **Fourteen regression cases pin the priority ladder's matching rules.**
  `TestPriorityLadder` now covers the two basename boundaries the defects above
  crossed — `readme.md`, `readme.rst` and `docs/README.adoc` at 1000 against
  `readme_helper.py` and `readmes.py` at 200 and `docs/READMEING.md` at 300 —
  every entry-point near miss (`notmain.go`, `myindex.js`, `mymanage.py`,
  `customserver.js`, all 200) against the real entry points at 400, and the new
  `.tsx` / `.jsx` tiers. Each case varies the extension and the basename
  independently, so the two matching rules cannot drift back into each other
  again. The self-packing snapshots in `examples/` are regenerated in the same
  commit: they now cover 55 files instead of 54 and about 280922 tokens instead
  of 273107, because `internal/cli/sortmodels_test.go` is new since they were
  taken and enters the omitted list. The kept-file selection is unchanged, so
  they still demonstrate the same budget behaviour.

- **Twelve tests cover the `sortModels` contract, including the two branches
  that had never run.**
  `internal/cli/sortmodels_test.go` pins the ordering for `name`, `window`, and
  `vendor` — the primary key, each tiebreak, case-folding, case-insensitive sort
  keys, the fallback for an unknown key, idempotence, and empty and
  single-element slices. Coverage of `internal/cli` rose from 99.4% to 99.6%;
  the one remaining uncovered statement is `outputWriter`'s `os.Create` failure,
  which calls `os.Exit(1)` and so cannot be reached from a test.

- **The `go install` path is now verified end to end.**
  It is the first install method the README and release notes show, and it had
  never been executed. From a clean GOPATH, `go install
  github.com/la2278647-arch/ctxpack@v0.1.9` downloads, builds and installs
  `ctxpack.exe`, which reports `ctxpack 0.1.9`. `@latest` and `@main` resolve to
  the release, `@v0.1.8` correctly resolves the older release, and `@v9.9.9`
  fails with a clear error. This also confirms the module path in `go.mod`
  matches the repository and that the root package is a main package, both of
  which `go install` silently requires.

## Test coverage

| package | v0.1.9 | v0.1.10 |
| ------- | ------ | ------- |
| cli     | 99.4%  | 99.6%   |
| counter | 100%   | 100%    |
| doctor  | 98.0%  | 98.0%   |
| format  | 99.3%  | 99.3%   |
| gitutil | 100%   | 100%    |
| ignore  | 98.0%  | 98.0%   |
| mcp     | 97.3%  | 97.3%   |
| packer  | 100%   | 100%    |
| repomap | 100%   | 100%    |
| version | 100%   | 100%    |
| walker  | 94.4%  | 94.4%   |
| total   | 98.5%  | 98.5%   |

425 test functions and 12 subtests, all passing. `packer` stays at 100%: the
three fixes above are covered by the new `TestPriorityLadder` rows, and the
function they changed already had no uncovered statement. Every remaining
uncovered branch in the suite is one that cannot be reached — a `filepath.Abs`
failure that needs the working directory deleted mid-call, a `json.Marshal`
failure over values that cannot fail to marshal, a `regexp.Compile` of a pattern
the translator always escapes, and `outputWriter`'s `os.Create` failure, which
calls `os.Exit(1)`. They are marked in comments rather than tested around. The
suite still asserts `go.sum` stays absent and `go list -m all` reports exactly
one module, so the binary remains dependency-free.

## Installation

```sh
# Homebrew
brew tap la2278647-arch/tap https://github.com/la2278647-arch/homebrew-tap
brew install ctxpack

# Go
go install github.com/la2278647-arch/ctxpack@v0.1.10

# Windows PowerShell
irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex

# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash

# Direct download
# https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.10
```

## What's next

- GitHub Actions CI workflow (staged in `docs/ci.yml`; activation blocked on
  `workflow` token scope)
- Dockerfile testing (not run: container use is not available in this
  environment)
- Model table expansion (blocked: context windows cannot be verified)
- Promotion posts (drafts in `docs/promote.md`)
