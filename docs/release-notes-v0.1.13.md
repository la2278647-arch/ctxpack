# v0.1.13

The release that hardens the edges: the installers learn `--help`, the
documentation-command guard starts covering the diff-demo, two flag-flag
conflicts are pinned down (help flags on `help`, `top` vs `sort`), a bad
`--output` path stops killing the process, and the model-ranking bugs that
made `tokens --top` (and the MCP `count_tokens top`) list the wrong models
are fixed in both interfaces.

## Added

- **`install.sh --help` (and `-h`) prints the usage and exits 0.**
  The installer's options lived only in the header comment — useless to
  someone who was just told to run `curl | bash` and wonders what `v0.1.13`
  or `CTXPACK_INSTALL_DIR` mean. `--help`/`-h` now print the usage block and
  exit before anything is downloaded. `install.ps1` gets the same branch
  (reachable as `install.ps1 -Version --help`; PowerShell's own `Get-Help`
  also works), and `make installerscheck` still passes with the new parse.

- **`commandscheck` now also runs the invocations in
  `examples/diff-demo/README.md`.**
  The doc-command guard covered README.md, docs/ci.yml, docs/examples.md and
  docs/promote.md, but the diff-demo's README shows `ctxpack diff` over a
  range and its `<repo>` placeholder was skipped. The extractor now rewrites
  `<repo>` to the scratch repository before its line splitting (the `>` in
  the placeholder is also a command separator), and the emitted diff commands
  run for real. Two awk/shell pitfalls surfaced while wiring this in: an
  apostrophe in a comment line inside the single-quoted awk program closed
  the quote and broke `bash -n`, and placeholder replacement had to move
  before splitting.

## Fixed

- **`help --help` and `help -h` show the help instead of an unknown-command
  error.**
  `ctxpack help` validated its one argument as a command name, so the help
  flags themselves were rejected with `unknown command "--help"` and exit
  code 2. They now return the shared help with exit code 0, like `ctxpack
  help` itself, while `help bogus` still names the typo and exits 2.

- **`tokens --top` now means "the N largest context windows".**
  The help text said exactly that, and the `models` command honoured it, but
  `tokens` cut the first N entries of the registration order — which is why
  `--top 3` listed `gpt-3.5-turbo`/`gpt-4` instead of the 2M-window Gemini
  models — and then re-sorted, losing even that. All three output formats
  (text/json/csv) now rank by context window descending, cut to N, and skip
  the format's own sort when `--top` is in effect, matching `models --top`.

- **MCP `count_tokens` `top` now means the same thing, matching the CLI.**
  The MCP server's `filterAndSortModels` still truncated by window and then
  re-sorted — defaulting to a name sort when no `sort` was given — so
  `count_tokens top:3` listed the first rows of the registration order, the
  same documented-lie bug the CLI just shed. `top` now returns the window
  order directly, so the two interfaces cannot disagree about what a top
  slice means.

- **A bad `--output` path no longer calls `os.Exit(1)`.**
  `outputWriter` created the target file and, on failure, printed to stderr
  and called `os.Exit(1)` — bypassing every deferred close in the caller and
  making the failure path impossible to test (a test invoking it would kill
  the test process). It now returns the error, and each of the twelve call
  sites maps it to exit code 1 through a shared helper, so `ctxpack pack -o
  /no/such/dir/x` fails cleanly like any other runtime error and is covered
  by a test.

- **`docs/ci.yml` release job now builds with Go 1.26.**
  The test matrix was extended to 1.26 in v0.1.12, but the (inactive)
  workflow's `packages` job still pinned `go-version: "1.23"`. The release
  job now uses the newest toolchain the matrix covers.