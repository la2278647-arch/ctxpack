# Changelog

All notable changes to ctxpack are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project aims
to follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed

- **`examples/diff-demo/README.md` said `--list` "deliberately hides
  deletions".**
  The section carried a two-path example plus the reasoning that `--list` keeps a
  shell loop receiving "one real, existing path per line". The filter fix below
  made `--list` name deletions, because the pack names them in `<deleted>` and a
  list that disagrees with the document it pretends not to build is the same
  defect as a header that disagrees with `<fileCount>`. So the claim was wrong and
  the example was no longer reproducible: the demo's own range contains a
  deletion, so `--list` prints three paths, not two. The section now shows the
  real three-path output, states the consequence (a listed path may not exist on
  disk, so a loop must `test -f "$f"` first) and points at `--dry-run`, which is
  the call that keeps packed and deleted paths apart.

  The stale sentence had already been copied into
  `examples/ctxpack-self-budget8000.md` — that snapshot packs this repository and
  embeds `examples/diff-demo/README.md` — so the published examples were wrong in
  two places. Both snapshots were regenerated, and the README's `--list` row, the
  `ctxpack help` line and the MCP `diff_repo.list` description now all say the
  same thing. `TestDiffDemoReadmeListOutputMatches` rebuilds the demo's
  two-commit scenario the way `make.sh` does and compares the README's documented
  output with a real run; it fails on the stale two-path example and passes on the
  fixed one.

- **`examples/diff-demo/make.sh` wrote to the wrong file when run as the README
  tells you to.**
  The script set `OUT=examples/diff-demo/diff.xml`, a repo-root-relative path,
  while both READMEs document `cd examples/diff-demo && ./make.sh`. From inside
  the script's own directory that resolves to
  `examples/diff-demo/examples/diff-demo/diff.xml`, so the committed `diff.xml`
  was left untouched and a nested `examples/` directory appeared beside it. The
  regeneration reported success, because `ctxpack diff` exits 0 either way — the
  output just landed somewhere nobody was looking. Found by following the
  documented command verbatim. `OUT` now resolves from the script's own location
  via `HERE=$(cd "$(dirname "$0")" && pwd)`, so all three plausible invocations —
  from the repo root, from `examples/diff-demo`, and by relative path from an
  unrelated directory — write the same file, verified byte-identical (865 B) in
  each case.

- **Thirteen advertised MCP arguments had no description.**
  An MCP client renders `inputSchema` straight into what a user sees: an
  argument without a `description` shows up with an empty hint, so asking
  `pack_repo` what `no_gitignore` does returned nothing at all — even though the
  schema said the argument existed, and `tools/call` honoured it. The affected
  properties were `format`, `no_gitignore` and `hidden` on `pack_repo`, `path`,
  `include`, `exclude` and `max_size` on `repo_map`, `path`, `no_gitignore` and
  `hidden` on `count_tokens`, and `format`, `no_gitignore` and `hidden` on
  `diff_repo`. All thirteen now carry a description in the same voice as the
  properties that already had one.

- **`server.go`'s package doc drifted from the schema it describes.**
  The doc repeated every tool's argument list by hand and had fallen behind the
  published schema: it abbreviated `pack_repo` to eight of its ten arguments
  (dropping `max_depth` and `model`), `count_tokens` to `path` alone while the
  schema advertised eleven, `list_models` to no arguments while it has
  `format`, and `diff_repo` to four of its twelve (dropping `model`, `include`,
  `exclude`, `max_size`, `no_gitignore`, `hidden`, `max_depth` and `list`). A
  hand-typed second copy of a table that is already declared once is a drift
  source, so the doc now names the six tools and their purposes and points at
  `tools()` as the single place arguments are declared.
  `TestToolSchemasDocumentEveryProperty` fails if any property loses its
  description, if a `required` entry is missing from `properties`, if a
  property declares no `type`, or if the tool list stops matching the doc's
  list. `TestEveryAdvertisedPropertyIsAccepted` calls each tool with every
  advertised property set to a schema-derived value and fails if a handler
  rejects its own schema.

- **`diff` reported two different file counts in one document.**
  The header comment was built from the pre-filter change list while
  `<fileCount>` was built from what actually got packed, so with a filter in
  force the two disagreed:
  `ctxpack diff . --include 'hello_service/*'` printed
  `<!-- ctxpack diff vs "WORKTREE": 3 files -->` and then
  `<fileCount>2</fileCount>` eleven lines below it. A reader who trusts one
  number over the other has no way to tell which one describes the document.
  The header (and the JSON stderr note) now report the packed count, which is
  what `<fileCount>`, `totalTokens` and `totalBytes` all describe.

- **`diff --list` ignored `--include`, `--exclude` and every other filter.**
  It printed the raw change list from git and returned before the packer ran, so
  `--include` narrowed the bundle it advertised but not the list. That is exactly
  the wrong place to get silent: `--list` is what a pre-commit hook or a CI check
  reads to decide whether the change fits, and a filter it ignores reports a file
  that the next command in the same pipeline never sees. `--list` now applies the
  same walker options as the pack — include, exclude, max-size, depth,
  gitignore, hidden — so its output is the set of paths the pack would emit. It
  also reads no file bodies: `ReadContent` stays false, so it stays cheap enough
  to run on every commit. Deletions are listed too, since the pack names them in
  a `<deleted>` element and `--list` was the only call that did not.

- **One file printed as "1 files".**
  Every count was `%d files`, so a single changed, omitted or deleted file read
  `1 files`, and the same was true of `pack --dry-run`, the `--output` summary
  and the `map --top 1` heading. All of them go through `format.Plural` now,
  which singularises only the count of exactly one, so `2 files` is untouched
  and `0 files` stays plural the way `0 files` naturally reads.

- **`install.sh` retried detecting the release but not downloading it.**
  v0.1.10 added `--retry 3` to the release-detection `curl`, and the two
  downloads that follow — the binary and `SHA256SUMS.txt` — were left without
  any. Those are exactly the transfers that get reset: `install.ps1`'s
  `DownloadWithRetry` already covered them, so the two installers disagreed about
  what a transient failure means. Found while verifying the v0.1.10 release:
  `install.sh v0.1.10` failed on its first attempt with
  `curl: (28) Failed to connect to github.com:443` and aborted with exit 28,
  while `install.ps1 v0.1.10` ran to completion on the same connection. Both
  downloads now retry 4 times on any error, matching `install.ps1`.

- **`go test ./...` failed in a downloaded source archive.**
  `TestCommandsAdvertiseTheFlagsTheyAccept` runs every command against every
  flag and expects the advertised ones to exit 0. `cmd diff` defaults to the
  process' current directory, which is a checkout in a clone but not in a
  downloaded tarball, so all 15 of `diff`'s flags reported
  `cmd diff: --format accept (exit 1, want 0)`. The published archive is a real
  input — the Homebrew formula builds from it — and so is any directory a
  contributor unzips to inspect the module. The test now creates a repository
  of its own and passes it to `diff` explicitly, so the suite no longer depends
  on the working directory.

## [0.1.10] - 2026-09-26

### Fixed

- **`readme_helper.py` was scored as a project document.**
  The README check was `strings.HasPrefix(base, "readme")` — any basename
  *beginning* with readme. `readme_helper.py` and `readmes.py` therefore scored
  1000, the policy-document tier, instead of 200 (source), outranking config
  files, scripts and every test. The same function matched `policyDocs` by
  exact basename, where `securityscanner.go` correctly scored 200 against
  `SECURITY.md` at 1000, so the two halves of one rule disagreed: one exact,
  one greedy. `isPolicyDoc` now accepts only the basename `readme` or
  `readme.<ext>`, so `README.md` and `docs/README.rst` still score 1000 while
  `readme_helper.py` and `readmes.py` score 200 and `docs/READMEING.md` scores
  300 — a markdown file that is not a README.

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
  default 10 — below config files (150), shell scripts (100) and even test
  files (50). Under a budget a component was dropped before a YAML file. Both
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

- **The CI release-binary job could not upload its artifacts.**
  The workflow declares `permissions: contents: read` at the top level, and that
  applies to every job unless a job overrides it. `actions/upload-artifact@v4`
  requires `actions: write`, so the `packages` job would have failed at the very
  last step of a release build. The job now declares both permissions itself
  instead of widening the whole workflow. Found while validating the workflow
  structurally — every step, matrix and permission is now checked by a script
  that parses the YAML, and all seven `run:` blocks pass `bash -n`.

- **CI release binaries were named differently from the real release assets.**
  The workflow built `ctxpack-0.1.9-linux-amd64` (hyphens) while `make release`,
  the published assets, and both installers all use `ctxpack_0.1.9_linux_amd64`
  (underscores). A CI artifact therefore could not stand in for a release
  download, and a `sha256sum -c` against the published sums file would fail.
  The naming now matches.

- **A mistyped `-ldflags -X` path is now detected instead of silently ignored.**
  Go accepts a wrong `-X` path with exit 0 and quietly leaves the source default
  in place: building with `-X ...version.Vresion=9.9.9-typo` (one letter wrong)
  and with `-X ...internal/versionz.Version=...` (wrong package) both succeed,
  and both binaries report `ctxpack 0.1.9`. That makes a typo in a version stamp
  invisible until a user inspects the output. The test job now builds with a
  sentinel version and fails if it does not appear in `ctxpack version`.

- **Both installers could not verify any checksum file.**
  `install.sh` and `install.ps1` matched an asset with a two-space separator,
  which is what `sha256sum -t` emits in text mode. But GNU coreutils defaults to
  **binary mode** when given file arguments, which emits `hash *name` instead.
  `sha256sum --help` states it plainly: `-b, --binary  read in binary mode
  (default unless reading tty stdin)`. So the checksum file that `make release`
  produces on this project's own release host made both installers fail with
  `ctxpack_0.1.9_windows_amd64.exe is not listed in SHA256SUMS.txt` — the
  installers rejected every release, on every platform. Neither had ever been
  executed, so the defect shipped for four releases. Both parsers now accept
  either marker and compare the name after stripping it, and the Makefile pins
  `-t` so the published file is deterministic.
  The parsers are covered by 8 offline cases each: binary and text mode, the
  asset not being first, and two prefix traps — `..._amd64.exe` must not match
  `..._amd64.exe.debug`, and `..._386.exe` must not match `..._amd64.exe`.

- **`install.sh` without a version produced a "version" made of release notes.**
  The REST call was piped through `grep -m1 '"tag_name"'`, which returns the
  whole matching line. When the API answers on a single line that line is the
  entire response including the release body, and the greedy `sed` then matched
  inside it, yielding a URL like `.../download/v line with the converse case
  proving` before curl refused the malformed URL. `grep -o` extracts just the
  `"tag_name": "v0.1.9"` fragment, so the `sed` only ever sees that.

- **A transient GitHub connection reset aborted an install.**
  `Invoke-WebRequest` has no retry of its own, and the script runs under
  `$ErrorActionPreference = 'Stop'`, so one reset connection killed the install
  with no attempt to recover. `install.ps1` now retries with backoff before
  giving up, and `install.sh` passes `--retry 3` to curl. Under a live reset the
  retry path was observed printing `retry 1/4 ... 3/4` and then rethrowing the
  real error, so a failure still reports clearly.

### Added

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

- **The `go install` path is now verified end to end.**
  It is the first install method the README and release notes show, and it had
  never been executed. From a clean GOPATH, `go install
  github.com/la2278647-arch/ctxpack@v0.1.9` downloads, builds and installs
  `ctxpack.exe`, which reports `ctxpack 0.1.9`. `@latest` and `@main` resolve to
  v0.1.9, `@v0.1.8` correctly resolves the older release and reports
  `ctxpack 0.1.8`, and `@v9.9.9` fails with a clear error. This also confirms the
  module path in `go.mod` matches the repository and that the root package is a
  main package, both of which `go install` silently requires.

- **Twelve tests cover the `sortModels` contract, including the two branches that
  had never run.**
  `internal/cli/sortmodels_test.go` pins the ordering for `name`, `window`, and
  `vendor` — the primary key, each tiebreak, case-folding, case-insensitive sort
  keys, the fallback for an unknown key, idempotence, and empty and single-element
  slices. Coverage of `internal/cli` rose from 99.4% to 99.6%; the one remaining
  uncovered statement is `outputWriter`'s `os.Create` failure, which calls
  `os.Exit(1)` and so cannot be reached from a test.

## [0.1.9] - 2026-09-26

### Added

- **A regression test pins the help text to the flag registrations.**
  `internal/cli/help_test.go` walks the union of every flag against all six
  commands and asserts that each advertised flag parses (exit 0) and each
  non-advertised one is rejected by the parser (exit 2). Two further tests read
  the real `printHelp` output and check both directions: every documented flag
  must be registered, and no section may document a flag the command rejects.
  Mutation-checked against the bug below — deleting `doctor`'s `-o` registration
  fails it with exactly `cmd doctor: -o accept (exit 2, want 0)`.

- **The `Run` dispatcher and three CLI error branches now have tests.**
  `internal/cli/cli_gaps_test.go` drives `cli.Run` — the function `main.go`
  actually calls — through its `doctor` case, which was the one command never
  dispatched by a test, so a typo in the case label would have shipped as
  "unknown command" for a command the help text advertises. The same file adds
  the near-miss check (`Run(["docter"])` must hit the unknown-command branch),
  the `tokens --model` OVERFLOW verdict with the reverse case proving the mark
  is a property of the repository/model pair rather than of the fixture, and
  `diff --dry-run`'s "omitted by the budget" line with the converse case proving
  it follows the cap rather than printing unconditionally.
  `TestDoctorRejectsExtraArgs` previously only rejected an unknown *flag*, which
  made its name overclaim; it now covers the positional-argument branch too and
  asserts both distinct messages. CLI statement coverage went from 98.4% to
  99.4%; the two remaining gaps are a name tiebreaker in `sortModels` (all
  registered model names are unique) and `outputWriter`'s `os.Create` failure,
  which calls `os.Exit(1)` and cannot be reached from a test.

### Changed

- **The help text now says what the commands actually accept.**
  `printHelp` described the walk flags as "map / tokens only" when `pack` and
  `diff` accept them too, filed `--depth` under a three-command group while four
  commands take it, and told `diff` it "also accepts" only eight of the sixteen
  flags it registers, omitting `--max-size`, `--depth`, `--no-gitignore`,
  `--hidden`, `--output`, `--dry-run` and `--list`. The FLAGS block is now one
  WALK FLAGS group naming the four traversing commands, plus a complete section
  per command, with no scope note the code contradicts.

- **Every smoke assertion now reports what failed.**
  `scripts/smoke.sh` ran under `set -euo pipefail`, but 23 assertions were bare
  `grep` calls: when one failed the script aborted with no message at all, which
  is the worst possible outcome for a gate — a wall of passing sections followed
  by silence, with no way to tell which check broke. All of them now go through
  `fail`, which prints a specific reason. The reason the bug stayed hidden was
  structural: `fail` was defined near the bottom of the script, after the
  assertions it was meant to serve, so those sections could not have used it. It
  now sits at the top. Wiring the `doctor --output` assertion up is what
  surfaced the `-o` bug below, which had been a silent no-op.

### Fixed

- **`doctor -o FILE` did not work.**
  The help text advertises `-o, --output FILE` as available on all commands, but
  `doctor` only registered the long form. The shorthand failed with
  `flag provided but not defined: -o` and exited 2, so anyone scripting
  `ctxpack doctor -o report.txt` got a usage screen instead of a report. Both
  spellings now write to the file, matching `pack`, `map`, `diff`, `tokens` and
  `models`.

- **`pack_repo` with `format: "json"` and a `model` returned unparseable JSON.**
  The MCP tool prepended the HTML comment that the text formats carry —
  `<!-- fit: OVERFLOW model=gpt-4 used=267.6k/4.1k (6533%) OVERFLOW -->` — in
  front of the JSON document, so any client that `JSON.parse`s the tool result
  threw on every annotated call. The CLI sidesteps this by routing the note to
  stderr, but an MCP client only ever sees the envelope. The annotation now
  travels inside the envelope as a `fit` object using the same keys
  `count_tokens --json` already uses for its per-model fit entries (`name`,
  `vendor`, `window`, `limit`, `used`, `pct_used`, `fits`, `reserve`), so the
  fit information is still there and the JSON stays valid. `diff_repo` shares the
  same path and gets the same treatment. Text formats keep the comment, and an
  unknown model yields `fit: {"model": "...", "unknown": true}` instead of
  destroying the envelope. `fitNote` now derives both renderings from one place,
  and `scripts/smoke.sh` asserts the `fit` key survives.

- **The README's MCP tool table was missing `doctor`.**
  `doctor` shipped in v0.1.8 but the table never gained its row, so a client
  reading the README concluded that a failing pack could not be diagnosed from
  inside the server - the exact case `doctor` exists for. The prose beside it
  called the analytic tools "three" when there are four. `doctor` is now listed
  with its `format?` / `top?` arguments, the `list_models` footnote moved out of
  the cell so the argument lists stay machine-readable, and the `fit` object
  from the fix above is described.
  `internal/mcp/readme_test.go` parses the table and compares it both ways
  against the schemas `tools()` returns: a tool added to or removed from the
  server fails it, as does a required/optional mismatch. Mutation-checked by
  deleting the `doctor` row, which fails with `the table does not document
  doctor; the server advertises it`.

- **`make smoke` could destroy uncommitted `README.md` edits.**
  The diff test dirties `README.md` to exercise `diff`, then restored it with
  `git checkout -- README.md`. That restores from the index, so anyone who ran
  the smoke suite with uncommitted README work lost it without warning. It now
  copies the file aside and copies it back, preserving the exact bytes that were
  there before the run. The finding was self-inflicted: the MCP table edit above
  was applied, the suite was run, and the edits came back missing from the
  commit.

## [0.1.8] - 2026-09-26

### Added

- **`scripts/smoke.sh`: the smoke suite as a single script.**
  Builds into `bin/`, then drives every command, all four output formats, the
  `--sort`/`--top`/`--format`/`--dry-run`/`--list` flags, `diff` with a
  `--ref A..B` range, a deletion across all four formats, and the MCP server
  over stdio — against the source tree itself. `make smoke` now delegates to it
  so there is one copy instead of a Makefile recipe and a CI recipe that could
  drift apart. The script is self-locating: it finds the repository root from
  its own path, so it runs from any working directory and accepts an explicit
  binary path as an optional first argument.

- **`doctor` is now part of the smoke suite.**
  The diagnostics command was documented and well covered by unit tests, but
  had never been exercised end to end — it was absent from both the smoke
  suite and the CI recipe. The suite now asserts its text shape (version,
  platform, git, model counts, vendor breakdown), both JSON entry points
  (`--json` and `--format json`), `--output`, and both error paths. It also
  pins the `--top` contract that matters most: the vendor list may be
  truncated, but the `N models, M vendors` total must not change, or a
  truncated report would understate the registry's size.

- **The file-selection flags are now part of the smoke suite.**
  `--include`, `--exclude`, `--max-size`, `--depth`, `--hidden`,
  `--no-gitignore`, `map --csv` and `models --vendor` were all documented and
  unit-tested, but none of them had been exercised end to end. The suite now
  asserts each one's contract with relative counts rather than pinned
  numbers, so it keeps passing as the repository grows: `--depth` must shrink
  the file set, `--max-size` must leave the count alone while collapsing the
  token total, `--include` and `--exclude` must each shrink it, `--hidden`
  must add files, and `--no-gitignore` can only keep or add.

- **`doctor` is now an MCP tool.**
  An agent that received `git error: exit status 128` from `diff_repo` had no
  way to tell whether git was missing, on the wrong version, or whether the ref
  was simply wrong — and the one diagnostic that answers that question required
  a `path`, which is exactly the thing the agent may be unable to name. The
  `doctor` tool takes no path at all: it reports the server's version, Go
  version, platform, git availability and version, and the model registry broken
  down by vendor. `top` truncates only the text breakdown; the totals and the
  `format: "json"` payload always stay complete. It offers just `text` and
  `json` and refuses anything else, so a client cannot request a format it would
  silently get back as text.

- **All six MCP tools are now part of the smoke suite.**
  The MCP server is the only other frontend to ctxpack, but the smoke suite
  previously called just one tool and one method. It now asserts that
  `tools/list` advertises all six tools, that each of them is callable with a
  real argument, and that five failure modes are reported correctly: an unknown
  `format` on either packing tool, an unknown `format` on `doctor`, a missing
  required `path`, and an unresolvable `ref`. Two conventions worth pinning: a
  tool's result is a JSON *string* inside the envelope, so inner keys arrive
  escaped (`\"tree\"`, not `"tree"`); and errors come back inside the envelope
  as `isError` rather than as a JSON-RPC error object, so a client must check
  `isError`.

### Changed

- **`doctor` moved into a shared package.**
  The CLI's `doctor` command and the new MCP `doctor` tool now both call
  `internal/doctor` instead of each implementing the checks themselves. That is
  the same "frontends share an engine" rule the packer and repomap packages
  already follow, and it removes about a hundred lines of duplicated git
  probing and vendor sorting from `internal/cli`. The JSON keys the CLI has
  always emitted are unchanged, so a script that parses
  `ctxpack doctor --json` keeps working.

- **The README MCP table was missing `pack_repo`'s `max_depth`.**
  `pack_repo` accepts ten arguments but the table listed nine, and the
  introductory sentence implied only `repo_map`, `count_tokens` and
  `list_models` take a `format` when all six tools do. Both are corrected, and
  the smoke suite now exercises `max_depth` through the MCP interface so this
  cannot quietly regress again.

- **`--dry-run` writes its report to stderr, and the smoke suite asserts it.**
  The suite previously discarded dry-run output with `> /dev/null`, which
  silently failed to silence it — this codebase sends artifacts to stdout and
  status to stderr, and a dry run produces status, not an artifact. A `> /dev/null`
  redirect therefore captured nothing at all. The assertions now capture stderr
  and require the report to be present, so the stream convention is pinned
  instead of merely tolerated.
- **A brittle smoke assertion was fixed, and the rest of its section now
  reports its failures.**
  The MCP range assertion required the first changed path to begin with an ASCII
  letter. That held by accident: paths list alphabetically, and `.gitignore`
  sorts ahead of every source file, so the assertion failed on a real commit
  range. It now asserts the envelope shape instead — `"text":` present and
  `isError` absent — which is the contract rather than the ordering. Every
  assertion in that section also gained `|| fail`: the script runs under
  `set -e`, so a bare grep that fails aborts with no message at all, which is
  the worst possible outcome for a gate — a wall of passing sections followed by
  silence. The rest of the script still has bare greps that behave this way;
  they pass today, and hardening them is deferred.

- **The CI smoke test calls `scripts/smoke.sh` instead of inlining its steps.**
  The inlined copy contained two bugs the script run surfaced: it grepped for
  `"tree"` in MCP output, where the tool result arrives escaped as `\"tree\"`
  inside the JSON-RPC envelope string, and it used `> /dev/null` to silence
  dry-run output that travels on stderr.

## [0.1.7] - 2026-09-26

### Added

- **`examples/diff-demo`: a reproducible diff snapshot.**
  A generated `ctxpack diff` output over a two-commit range containing a
  modification, an addition and a deletion — the smallest example that shows
  range syntax and the new `Deleted` section together. `make.sh` builds the
  scratch repository in a temp directory and deletes it on exit, so the
  committed `diff.xml` is byte-identical on any machine.

- **`diff` reports deleted files.**
  A deletion has no content to pack, so before this release the bundle simply
  never mentioned it — a removed file looked like it had never existed. `diff`
  now lists removed paths in a `Deleted` section: XML adds a `<deleted>`
  element, Markdown a `## Deleted` heading, text a `==== deleted ====` block,
  and JSON a `deleted` array. `--dry-run` prints the count and each path. The
  MCP `diff_repo` tool reports them the same way. Paths are reported by name
  only, never content.

- **`diff` ranges (`--ref A..B`).**
  `ctxpack diff --ref HEAD~5..HEAD` compares two revisions as history. A range
  never includes the working tree's untracked files, because those belong to
  neither side of the range. A plain ref (`--ref main`) still diffs against
  the working tree, so uncommitted changes remain included. The MCP
  `diff_repo` tool accepts the same range syntax for its `ref` argument.

### Changed

- **Bundles name the directory, not the machine.**
  `pack` and `diff` now label the bundle with the walked directory's base name
  (`<root>ctxpack</root>`) instead of its absolute path
  (`<root>D:\Users\alice\ctxpack</root>`). The machine the bundle was built on
  is not information a model needs, and an absolute path leaks the username of
  whoever packed the repository. This matches what `map` already did for its
  root node; `pack` and `diff` had diverged from it. The JSON `root` field
  carries the same shortened value.

- **`gitutil.DiffFiles` splits deletions from changes.**
  `ChangedFiles` keeps its signature and behaviour, now returning only the
  packable set; callers that need to report removals use the new
  `DiffFiles`, which returns both lists.

## [0.1.6] - 2026-09-25

### Added

- **`format` parameter for MCP `list_models`.**
  The `list_models` tool now accepts `format` (`text` or `json`, default
  `text`). With `format: "json"` it returns `{"models": [{name, vendor,
  context_window, limit}]}` — the same shape as the CLI's `models --json`
  output, so an agent can pick a valid model name before calling
  `count_tokens`.

- **`format` parameter for MCP `repo_map`.**
  The `repo_map` tool now accepts `format` (`text` or `json`, default
  `text`). With `format: "json"` it returns the structured envelope with
  `root`, `total_tokens`, `total_bytes`, and a `tree` of `{name, is_dir,
  tokens, bytes, children}` nodes — the same shape as the CLI's `map --json`
  output.

- **`--list` flag for `diff`.**
  `ctxpack diff . --list` prints only the changed file paths, one per line,
  without packing them or reading their content. Faster than `--dry-run`
  (which packs to count tokens) and useful for scripting or piping into other
  tools. The MCP `diff_repo` tool now accepts a `list` parameter for the same
  behaviour.

- **More parameters for MCP `diff_repo`.**
  The `diff_repo` tool now accepts `no_gitignore`, `hidden`, and `max_depth`
  arguments, matching the CLI's `diff` command. With these, an MCP client can
  fully control traversal: ignore `.gitignore`, include dotfiles, and limit
  depth.

### Changed

- **MCP `count_tokens` now matches the CLI's `tokens` command.**
  Added `model` (filter to one model), `top` (limit to N largest windows),
  `sort` (by name, pct, or window), `no_gitignore`, and `hidden`. The `sort`
  parameter is applied before `top`, so `top: 3, sort: "window"` returns the
  three largest models.

### Tests

- **MCP coverage improved from 90.6% to 96.0%.**
  Added error-path tests for `diff_repo` (non-git directory, no changes, model
  annotation, list mode) and edge-case tests for `count_tokens` (sort by pct,
  unknown model, JSON with filters). `filterAndSortModels` reached 100%.

- **MCP coverage further improved to 97.4%.**
  Added `list_models` JSON tests, `pack_repo` unknown-model annotation tests,
  and direct unit tests for `humanTokens` (all three branches). `humanTokens`
  reached 100%; `annotateFit` went from 75% to 87.5%.

- **CLI coverage improved to 96.6%.**
  Added direct unit tests for `countFiles` (nil node, single file, empty
  directory, nested recursion). `countFiles` reached 100%.

## [0.1.5] - 2026-09-25

### Added

- **A Dockerfile.**
  Multi-stage build: `golang:1.21-alpine` compiles a static binary with
  `CGO_ENABLED=0` and `-trimpath`, `alpine:latest` ships it. Three build args
  (`VERSION`, `COMMIT`, `DATE`) stamp the version identity the same way the
  Makefile does. The entrypoint is the binary itself, so any arguments become
  `ctxpack` arguments. The README gains a Docker install section.

- **`--quiet`/`-q` for `pack` and `diff`.**
  `pack`: suppresses the "wrote X — N files, ~M tokens" stderr message when
  `--output` is set, and the JSON fit annotation that would otherwise go to
  stderr. `diff`: suppresses the JSON-mode stderr status lines. Scripts that
  capture stderr for diagnostics no longer need to grep around the status line.

- **`--model NAME` for `tokens`.**
  Show the fit for one model instead of all 19. An unknown model name exits 2
  with a stderr message before any output is written. JSON output filters the
  `fits` array to the single entry.

- **`--vendor NAME` for `models`.**
  Filter the model table to one vendor (case-insensitive): `openai`,
  `anthropic`, `google`, `meta`, `mistral`, `deepseek`, `alibaba`. An unknown
  vendor exits 2 with a stderr message and writes nothing to stdout. JSON
  output applies the same filter to the `models` array.

- **`--sort BY` for `map`.**
  Sort children within each directory by `tokens` (largest first) or `bytes`
  (largest first) instead of the default alphabetical order. `repomap.Build`
  now takes an `Options` struct carrying `Walker` and `SortBy`; the MCP
  `repo_map` tool gains a `sort` parameter.

- **`--depth N` for `pack`, `map`, `tokens`, and `diff`.**
  Limit traversal to N levels below root. `0` = unlimited (default).
  Directories at or below the limit are skipped entirely. `MaxDepth` is a new
  `walker.Options` field; the MCP `pack_repo` and `repo_map` tools gain a
  `max_depth` parameter.

- **`--top N` for `map`.**
  Show a flat list of the N largest files across the entire tree, sorted by
  tokens (default) or bytes (`--sort bytes`). Ignores the tree structure and
  the `--sort` child-ordering. Useful for finding the biggest files at a
  glance. Output is a table with path, tokens, and bytes columns.

- **`--csv` for `map`.**
  Output a flat CSV list of all files (path, tokens, bytes), sorted by tokens
  (default) or bytes (`--sort bytes`). Header row included. Ready for
  spreadsheet import or `csvkit` pipelines.

- **4 new models in the model table.**
  `gpt-4.1` (OpenAI), `claude-4-sonnet` (Anthropic), `gemini-2.5-pro` (Google),
  `deepseek-r1` (DeepSeek). The table now has 23 models across 7 vendors.
  Prefix matching works for all new models.

- **`ctxpack doctor` command.**
  Prints environment diagnostics: version, Go version, platform, git path
  and version (or "not found"), and model/vendor count. Supports `--json`
  for machine-readable output. Useful for troubleshooting and verifying
  installation health.

- **`--sort BY` for `tokens`.**
  Sort the per-model fit table by `pct` (most constrained first), `window`
  (largest context window first), or `name` (default, alphabetical). Also
  sorts the JSON `fits` array. Useful for finding the most or least
  constrained model at a glance.

- **`--output`/`-o` for `map` and `tokens`.**
  Write output to a file instead of stdout, matching the existing `--output`
  flag on `pack` and `diff`. All four commands now support writing to a file.

- **`--format F` for `map`.**
  Select output format: `text` (default), `json`, `csv`. The existing
  `--json` and `--csv` flags still work as aliases. Unknown formats exit
  with code 2 and a stderr message.

- **`--format F` for `tokens`.**
  Select output format: `text` (default), `json`. The existing `--json`
  flag still works as an alias. Unknown formats exit with code 2 and a
  stderr message.

- **`--format F` for `doctor`.**
  Select output format: `text` (default), `json`. The existing `--json`
  flag still works as an alias. Unknown formats exit with code 2 and a
  stderr message.

- **`--format F` for `models`.**
  Select output format: `text` (default), `json`. The existing `--json`
  flag still works as an alias. Unknown formats exit with code 2 and a
  stderr message.

- **8 new models in the model table.**
  Added: `gpt-5` (openai, 200k), `o4-mini` (openai, 200k),
  `claude-3.7-sonnet` (anthropic, 200k), `claude-4-opus` (anthropic, 200k),
  `gemini-2.5-flash` (google, 1M), `llama-3.3-70b` (meta, 128k),
  `mistral-large-2` (mistral, 128k), `qwen-2.5-72b` (alibaba, 128k).
  Total: 31 models across 7 vendors.

- **`--output`/`-o` for `doctor` and `models`.**
  Write output to a file instead of stdout. All six commands (pack, diff,
  map, tokens, doctor, models) now support writing to a file.

- **`--dry-run` for `pack`.**
  Show what would be packed (files, tokens, bytes) without writing any
  output. Useful for checking the budget before generating a bundle.

- **`--dry-run` for `diff`.**
  Show the changed files that would be packed without writing any output.
  Useful for checking which files would be included in a diff bundle.

- **`--top N` for `tokens`.**
  Show only the N largest models by context window size. Useful for
  finding the models with the biggest context windows that can fit
  your repository.

- **`--top N` for `models`.**
  Show only the N largest models by context window size. Useful for
  quickly finding the most capable models in the registry.

- **`--top N` for `doctor`.**
  Show only the top N vendors by model count in the vendor summary.
  The "Total: X models, Y vendors" line still reflects the full count,
  so the truncation is display-only and cannot mislead about the
  registry's total size.

- **`--sort BY` for `models`.**
  Sort the model table by `name` (default, alphabetical), `window`
  (largest context window first), or `vendor` (group by vendor, then
  by window within each group). Only applied when `--sort` is explicitly
  provided; without it the registration order is preserved.

- **`diff_repo` MCP tool.**
  The MCP server now exposes a fifth tool, `diff_repo`, which packs only
  the files changed against a git ref. Takes `path`, optional `ref`
  (default `WORKTREE`), `format` (default `xml`), and `budget`. Deleted
  files are listed but their content is omitted, matching the CLI's
  `diff` command.

- **`format` parameter for MCP `count_tokens`.**
  The `count_tokens` tool now accepts a `format` argument (`text` or
  `json`, default `text`). With `format: "json"` it returns the structured
  envelope with `total_tokens`, `total_bytes`, `reserve_tokens`, and a
  `fits` array — the same shape as the CLI's `tokens --json` output.

- **`model` parameter for MCP `pack_repo`.**
  The `pack_repo` tool now accepts a `model` argument. When set, the
  output is prefixed with an HTML comment annotating whether the packed
  bundle fits within that model's context window after the reply reserve —
  the same annotation the CLI's `--model` flag produces.

- **More parameters for MCP `diff_repo`.**
  The `diff_repo` tool now accepts `include`, `exclude`, `max_size`, and
  `model` arguments, matching the CLI's `diff` command. This lets an MCP
  client filter which changed files are packed and annotate fit for a
  specific model.

## [0.1.4] - 2026-09-22

The release that makes the installers trustworthy, and closes out the
machine-readable surface: four commands now emit JSON, the MCP server exposes
all four operations, and a script can read the whole model table without
parsing text.

### Added

- **The installers now verify the binary they install.**
  Both downloaded the release asset straight onto `PATH` and trusted it. They
  now also fetch the release's `SHA256SUMS.txt`, compare, and abort on a
  mismatch, a missing entry, or a failed download — with nothing left on
  `PATH`. `install.sh` already downloaded to a scratch directory and moved the
  result into place; `install.ps1` wrote directly to the destination, so a
  truncated download could have left a broken `ctxpack.exe` where the real one
  belonged. Both are atomic now.

  Verification is exercised three ways against a local stand-in release rather
  than only by inspection: a correct checksum installs, a single flipped hex
  digit is refused, and an asset absent from the list is refused. In every case
  the scratch files are removed and nothing is installed.

- **A `list_models` MCP tool.**
  The three MCP tools all required a path, and none could tell an MCP client
  which model names existed. An MCP client *is* an LLM picking a model name by
  hand, so it had to guess. `list_models` takes no arguments and returns the
  whole table — 19 models, each with its window and effective limit — so a
  client can read it before it asks `count_tokens` or `pack_repo` about a
  repository.

  It deliberately takes no arguments: there is nothing to filter on, and an
  argument-taking tool that ignored its arguments would hide typos, which is the
  bug that just landed on `ctxpack models`.

- **`ctxpack models --json`.**
  That completes the machine-readable surface: `pack --format json` for the
  bundle, and `--json` for `map`, `tokens` and now `models`. The model table is
  what an LLM reads before deciding whether a repo fits at all, so it belongs
  in the same scriptable shape as the fit it produces.

  `models --json` wraps the table in `{models: [...]}`, one entry per
  registered model carrying `name`, `vendor`, `context_window` and `limit`.
  `limit` is the window minus the 4096-token reply reserve — the same value
  `tokens --json` reports inside each fit entry — so the two outputs join
  without a script recomputing the subtraction. All 19 models and 7 vendors are
  present, and every `limit` is asserted to be `context_window - fitReserve`.

- **`ctxpack models` no longer swallows unknown arguments.**
  It was the only command using a manual `args[0]` check instead of a
  `flag.FlagSet`, so `ctxpack models --bogus`, `ctxpack models --json` and a
  bare `ctxpack models` were all indistinguishable from each other: a typo'd
  flag was silently ignored and the table printed anyway. It now parses with
  `reorderArgs` like the rest, and a stray argument exits 2 naming the offender.

  `models -h` still exits 0 rather than 2, unlike every other command. That is
  a test-asserted quirk (`TestRunDispatchExitCodes` pins it) so it was
  preserved: the help check runs before the FlagSet parse. Fixing the
  inconsistency the other way would break a documented exit code for no gain.

### Fixed

- **`install.ps1` failed to find the latest release whenever GitHub rate-limited it.**
  Untagged installs call the REST API, which allows 60 anonymous requests per
  hour; on a busy machine that 403s and the install aborts. It now falls back
  to the `/releases/latest` page, which 302-redirects to `/releases/tag/vX.Y.Z`
  and is not rate-limited. That fallback reads the `Location` header off the raw
  response, because `Invoke-WebRequest -MaximumRedirection 0` throws an exception
  whose `Headers['Location']` comes back empty. `install.sh` gets the same
  fallback.

- **Two `set -e` bugs in `install.sh` swallowed its own error messages.**
  A `grep` that matches nothing exits 1, and under `set -euo pipefail` that
  aborts the script before the friendly "not listed in SHA256SUMS.txt" line is
  reached. The same applied to the curl that fetches the redirect. Both are now
  guarded, so the intended message is what the user sees.

- **`install.ps1` silently installed the amd64 binary on an unrecognised CPU.**
  The `switch` on `$env:PROCESSOR_ARCHITECTURE` had no `default`, and `$Arch`
  defaulted to `amd64` above it — so an unrecognised value produced a wrong-
  architecture binary rather than an error.

- **`install.sh` contained a dead line.**
  `if [ -n "${BASH_EXE:-}" ]; then :; fi` tested a variable that bash always
  sets and did nothing with it. Deleted.

- **The reply reserve now has one definition.**
  `cli` declared it as a constant; `mcp` typed `4096` inline. Nothing was wrong
  today, but changing the reserve in one place would have left the other surface
  reporting a different `limit` with no failure and no warning. Both now read
  `counter.ReplyReserve`, defined next to the `FitsModel` that consumes it, and
  `cli.fitReserve` is an alias rather than a restatement. A new test parses the
  limit out of both MCP tools and fails if they disagree on any of the 19
  models; another pins `fitReserve == counter.ReplyReserve == 4096` so a change
  to the policy is deliberate rather than accidental.

- **The README's own JSON example had a wrong number.**
  It showed `gpt-4o` with `"limit": 119808`, which is off by exactly 4096.
  `gpt-4o`'s window is 128000, so the limit is 123904; the `pct_used` of
  26.58 was computed against the wrong window and is 25.7. Caught by cross-
  checking the example against `ctxpack models`'s table rather than by eye.
  The new `limit` invariant now pins the relationship that would have caught it.

## [0.1.3] - 2026-09-22

The first release that was actually published. Statement coverage is 98.7% of
942 statements, with seven of ten packages at 100%.

### Added

- **`ctxpack map --json` and `ctxpack tokens --json`.**
  Both commands now emit machine-readable JSON, so a script can read the tree or
  the per-model fit instead of parsing a text outline. `pack` already had
  `--format json` for its bundle; these two have no format family, so they take
  a `--json` boolean instead.

  `map --json` wraps the tree in `{root, total_tokens, total_bytes, tree}` and
  `tokens --json` wraps the summary in `{path, total_tokens, total_bytes,
  reserve_tokens, fits}`. Two invariants are tested rather than merely intended:
  `tree.children` is always an array and never `null`, so a file and an empty
  directory both read `[]` and `is_dir` is what separates them; and the header
  totals equal the tree totals and agree with the text output, because every
  format reads one shared walk and cannot drift.

  Each fit entry carries `used`, `limit` (the window minus the reply reserve)
  and `pct_used` rounded to two decimals, so the JSON stops echoing float64
  noise such as `1258.287899747742`. The reply reserve is now `fitReserve`, a
  named constant instead of a literal repeated in two call sites, so the text
  table, the JSON output and `pack`'s fit annotation all read the same value.

  Eight new tests cover the two JSON shapes, both invariants, the empty-tree
  case, and `writeEnvelope`'s error branch — the last by handing the encoder a
  writer that refuses every byte, which needs no test hook. `internal/cli`
  stays at 100.0%.

- **A real project to try ctxpack on: `ctxpack-demo`.**
  [la2278647-arch/ctxpack-demo](https://github.com/la2278647-arch/ctxpack-demo)
  is a 27-file FastAPI microservice (hello-service v0.3.1) with cursor
  pagination, pydantic validation, a 58 KB synthetic catalog and a test per
  behaviour. It carries ctxpack's own output captured on it, so the tool is
  shown rather than described: `ctxpack tokens .` reports 35545 estimated
  tokens with two small-window models (`gpt-3.5-turbo` at 289%, `gpt-4` at
  868%) marked `[OVERFLOW]`; `ctxpack map .` shows the 58 KB `data/seed.json`
  alone accounting for 26178 tokens, 73.6% of the tree; and
  `ctxpack pack . --budget 5000` keeps 16 files at ~4938 tokens and prints the
  six files it cut with their individual token counts.

  The snapshot is embedded in the demo's README and linked from
  [docs/examples.md](docs/examples.md). The single generated data file is the
  point of the fixture: it is what makes a context budget necessary at all, and
  the omitted-files report is what makes the trade-off legible.

### Fixed

- **The last two uncovered branches of the two "unreachable" packages were
  unreachable because they were unreachable.** `cmdDiff`'s `packer.Pack` error
  branch and `counter.Estimate`'s two insurance clamps were all documented as
  "needs a filesystem condition no test can produce reliably". That was a
  description of a test's laziness, not a fact about the code.

  `cmdDiff` now calls `packer.Pack` through a `runPack` variable, mirroring the
  `runMCP` hook `cmdMCP` got in v0.1.2, and `TestDiffReportsAPackError`
  substitutes it to return an error and asserts the branch reports
  `ctxpack: walk denied` and exits 1. The substitution is justified in the
  variable's own comment: `Pack` silently ignores any path in `Options.Files`
  the walk did not reach, and a root that fails `os.Stat` would already have
  failed `gitutil.ChangedFiles`, so the branch has no repository-level trigger.

  The two clamps in `Estimate` were pulled into named helpers, `chunkTokens`
  and `floorTokens`, so each is pinned directly instead of only through the
  regex: `TestChunkTokens` covers the zero/negative clamp, and `TestFloorTokens`
  covers the at-floor, below-floor and above-floor cases. A new
  `TestEstimateFloorNeverBindsOneChunk` sweeps every one-chunk length from 1 to
  500 — one chunk is the worst case, earning the fewest of the +34 byte bonuses
  in the chunk formula — and confirms the floor never binds, which is what the
  clamp's comment claims. The clamps stay: they are insurance, and the tests
  now prove the insurance is not load-bearing today.

  `internal/cli` and `internal/counter` both reach 100.0%; the repository goes
  from 97.5% to 97.9%. Five packages are now at 100%: `cli`, `counter`, `mcp`,
  `packer`, `version`.

- **Three more branches claimed as unreachable turned out to have real triggers.**
  `gitutil` was reported at 95.4% with its two `git` error branches listed as
  untestable. Both are reachable from the working tree:

  - A **corrupt index** makes `git status --porcelain` fail with exit 128 while
    `git rev-parse --git-dir` still succeeds — which is precisely the split that
    makes `ChangedFiles` distinguish "not a repository" from "repository in a
    bad state". Verified before writing the test: the error is `fatal: index
    file corrupt`, and `rev-parse` returns `.git` normally. `TestChangedFilesReportsAStatusError`
    writes garbage over `.git/index` and asserts the error is not
    `ErrNotARepo`, then re-runs `rev-parse` inside the test to prove the premise
    that makes the branch reachable.
  - An **unresolvable ref** fails `git diff --name-status`, covered by
    `TestChangedFilesReportsADiffError`.

  `TestUnquoteReturnsMalformedPathsUnchanged` covers the third gap: a path that
  starts with `"` but is not a valid Go literal must come back unchanged rather
  than truncated.

  Walker's `os.ReadFile` failure branch had the same story. The existing test
  reached the unreadable-*subtree* case by denying read on a directory, which
  makes `WalkDir` hand back an error. A per-file deny with no `(OI)(CI)` leaves
  enumeration untouched: `os.Stat` on the file still succeeds, so `d.Info()`
  passes, and `os.ReadFile` is the call that fails. `TestWalkSkipsAFileItCannotRead`
  uses `icacls file /deny Everyone:(R)`, then verifies the premise inside the
  test — stat succeeds, read fails — before walking, so it skips loudly rather
  than silently covering nothing on a box where the deny is not enforced.

  `internal/gitutil` reaches 100.0% and `internal/walker` goes from 92.7% to
  94.4%. Six packages are now at 100% and the repository total is 98.5%.

  Walker's remaining six blocks are left alone this time. Two need a file to
  vanish between `ReadDir` and `Info` or between `Info` and `ReadFile` — a race,
  not a condition. `filepath.Abs` needs the process cwd deleted mid-call.
  `filepath.Rel` needs two paths on different volumes, which one `WalkDir` root
  cannot produce. `WalkDir` itself returns a non-nil error only if the callback
  returns one, and this callback returns `nil` or `SkipDir`. The two `globRegex`
  failures need `ignore.translateGlob` to emit a regex it cannot compile, which
  it provably cannot because it escapes every regex-special character.

- **Two more packages hit 100%, and the last reachable `ignore` branch was
  folded in.** The same question again: is this "unreachable" comment
  describing the code or the test?

  `repomap` (98.1% → 100.0%): `fold` skips an empty path component, guarded by
  `if p == "" { continue }`. The walker always yields a relative path, so the
  guard never fires in production. But `fold` was already split out of `Build`
  specifically so it can be tested against a fabricated walk result — which is
  exactly the tool needed here. `TestFoldSkipsEmptyPathComponents` feeds it
  `""`, `"/"`, `"/abs/path.go"` and `"//double"`, and `assertNoEmptyName` walks
  the whole tree asserting no node has an empty name, while the totals still
  count the file.

  `ignore` (97.0% → 98.0%): `Load` reports an error when a `.gitignore` cannot
  be opened, except when it is missing, which is a legitimate "nothing here".
  That distinction had no test. A file whose ACL denies read opens successfully
  and fails on read, which is precisely the reported case, so
  `TestLoadReportsAnUnreadableGitignore` denies read on a real `.gitignore`
  and asserts `Load` returns an error while `Load` of a missing path returns
  nil.

  Two things had to hold before the assertion would mean anything, so the test
  checks them first: the deny must actually be applied (`os.Open` must fail)
  and a missing file must genuinely be a no-op. Without that, the test would
  fail the assertion for the wrong reason on a box where the ACL is not
  enforced.

  `format` stays at 99.1% and `ignore` at 98.0%. Their remaining blocks were
  given accurate comments in place of hand-waving: `renderJSON`'s
  `MarshalIndent` error cannot occur for `*Bundle`, because every field is a
  string, an int, a bool or a slice of those — no floats, maps or cycles — and
  `NewMatcher`'s `Abs` fallback needs the process cwd deleted mid-call.
  `ignore`'s `compilePattern` failure was already explained in place.

  Seven packages are now at 100% and the repository total is 98.7% of 942
  statements. Twelve statements remain uncovered: seven in `walker`, two in
  `ignore`, one in `format`, and two in `main.go` — a `main()` the test harness
  cannot call. The "eight statements" figure in the earlier draft was wrong;
  these counts are taken from the coverage profile itself.

## [0.1.2] - 2026-09-22

Bug-fix and hardening release. Three things that now report what they do,
fourteen that were plainly wrong, and the tests that would have caught each of
them. Statement coverage across the repository went from 82.0% to 97.5%, with
`internal/mcp`, `internal/packer` and `internal/version` at 100%.

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

- **`--format json` did not produce JSON.** `pack --model` and `diff` always
  prepended an HTML comment, so every JSON bundle started with `<!-- fit: ... -->`
  or `<!-- ctxpack diff vs ... -->` and failed to parse. The fit note and the
  diff header are still printed — to stderr, where a program consumer does not
  read them — and the file now parses as JSON. XML, Markdown and text keep the
  leading comment, which is the natural carrier for those formats.

- **`--max-size` was documented as "skip".** The help text, the four flag
  registration strings and the README all said "Skip files larger than N
  bytes". The walker does not skip them: a file over the limit is still listed
  with its real size and a token estimate from its name, it is simply never
  read. A user capping a scan at 100 KB therefore still saw every large file
  counted in the total. All six texts now say "read no more than N bytes of a
  file (larger files stay listed, without content)", matching the walker's
  documented contract.

- **`ctxpack mcp` could not be tested.** `cmdMCP` hard-coded `os.Stdin` and
  `os.Stdout`, so the only way to reach the `mcp` dispatch was to drive a live
  JSON-RPC loop over the process's real standard streams. `cmdMCP` now takes
  `io.Reader`/`io.Writer`, and `Run` reaches it through a `runMCP` hook, so the
  stdio handshake is exercised with pipes and no global state.

- **The walker contained dead code that lied about `--include`.** Inside the
  hidden-entry branch (only reachable when `IncludeHidden` is false) there was
  `if !matchesAny(rel, opts.Include) && len(opts.Include) > 0 { return nil }`
  followed immediately by an unconditional `return nil`. The conditional could
  never change the outcome — a hidden file was skipped either way — so the
  include check was dead, and the comment above it claimed hidden files could
  be brought in with `--include`. That was never true and never will be,
  because `--include` is a way to narrow a scan, not an escape hatch from the
  hidden-file default: if it did leak `.env`, a secret would land in a prompt
  without the user ever having opted in. The branch is now the single comment
  that says so, and `Options.Include`/`Options.IncludeHidden` document the
  interaction. `TestWalkHiddenStillExcludedWhenIncluded` now pins it across
  `.env`, `*.env`, `**/.env`, `**/*.env` and `**/.config`.

- **The `pack_repo` tool schema still lied about `max_size`.** The CLI help,
  the four flag registration strings and the README were corrected above, but
  the MCP `inputSchema` still read "Skip files larger than N bytes." — which is
  the text a model reads when deciding how to call the tool. An LLM choosing
  `max_size` on that basis would expect large files to disappear from the
  bundle and be surprised to find them listed without content. It now says
  "Read no more than N bytes of a file; larger files are still listed, without
  content.", matching the walker. `TestPackRepoSchemaDescribesMaxSizeHonestly`
  fails if the word "Skip" comes back.

### Testing

- **The CLI test suite went from 57.5% to 99.1%.** `internal/cli/cli_test.go`
  exercised the pure helpers (`reorderArgs`, `parseFormat`, `humanTokens`,
  `humanBytes`, `annotateFit`, the env lookups) but never once ran `map`,
  `tokens` or the `mcp` server — all three were at 0% — and `diff` at 44.7%,
  because a test git repository had never been created in the suite.
  `internal/cli/cli_more_test.go` adds 34 tests: stdout/stderr are captured
  through `os.Pipe` with a restoring `t.Cleanup`, `cmdMap` and `cmdTokens`
  are asserted on their rendered output (tree contents, the `Per-model fit`
  block, exactly 19 model rows, both `fits` and `OVERFLOW` marks in one run),
  a real git repository is created with `git init`/`add`/`commit` for the
  `diff` paths (happy path, `--ref HEAD~1`, no changes, outside a repo, an
  unresolvable ref, a bad format, a bad output path, and a model annotation),
  and the MCP server is driven over a pipe with a real `initialize` request
  plus an immediate EOF and a read failure. Two new `json.Valid` assertions
  guard the regression above on both `pack` and `diff`.

  The two remaining uncovered lines are `cmdDiff`'s `packer.Pack` error
  branch, which needs a directory that `git rev-parse` accepts and
  `os.ReadDir` refuses — not something a test can produce reliably.

- **The walker went from 91.3% to 92.7%.** The unreadable-subtree path in
  `Walk` was never exercised: it needs `WalkDir` to hand the callback a
  non-nil error, which needs a subdirectory that refuses to be read.
  `TestWalkToleratesAnUnreadableSubtree` creates one with `icacls /deny
  Everyone:(OI)(CI)(R)`, restores the ACL in a `t.Cleanup` that runs before
  the TempDir is deleted, and asserts the walk continues and drops the
  subtree rather than failing the whole call. A dangling-symlink test is
  included for the `d.Info()` path; it skips where `os.Symlink` needs
  SeCreateSymbolicLinkPrivilege, which is every non-admin Windows box.

  The remaining seven uncovered blocks are all defensible now that they have
  been examined rather than merely unexplained: `filepath.Abs` can only fail
  on a base path the subsequent `os.Stat` would already reject; `filepath.Rel`
  can only fail when the two paths are on different volumes, and both come
  from the same `WalkDir` root; `d.Info` and `os.ReadFile` need a file to
  vanish between two calls, which would make the test flaky by construction;
  `WalkDir` returning an error needs a callback that returns a non-nil error,
  which this callback never does; and both `globRegex` compile failures need
  `ignore.translateGlob` to emit an invalid regex, which it provably cannot,
  since it escapes every regex-special character. Each is now commented in
  the source rather than left as a bare uncovered line.

- **The MCP server went from 93.1% to 100%.** Seven blocks were left
  uncovered, and six of them were reachable — the suite had only ever sent
  well-formed tool calls. `internal/mcp/server_more_test.go` adds:
  `pack_repo`/`repo_map`/`count_tokens` each called with an empty
  `arguments` object, so every tool's required-`path` guard is asserted;
  `repo_map` and `count_tokens` pointed at a repository whose root `.gitignore`
  is one byte past the matcher's 1 MiB line cap, which makes `Build` fail and
  exercises both `map error:` branches; `count_tokens` against 60 documents,
  large enough to show `OVERFLOW` on the smallest window and still `fits` on
  the largest in one run; `{"id":null}` as a request, which is a notification
  and must not be answered; a direct `write(map{"n": math.NaN()})`, since JSON
  cannot represent NaN, asserting that the message is dropped whole and the
  writer is still usable; `toInt` fed a Go `int`, which JSON never produces but
  an in-process caller does; and the `max_size` schema wording above.

  That takes the whole repository from 96.6% to 97.5%. The remaining uncovered
  lines are `cli` at 99.1%, `counter` at 95.3%, `gitutil` at 95.4%, `ignore`
  at 97.0%, `repomap` at 98.1% and `walker` at 92.7%; each was examined and
  left in place, with the `walker` reasons recorded above.

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
