# Changelog

All notable changes to ctxpack are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project aims
to follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
