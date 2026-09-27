# v0.1.11

The release that turns the command surface into a scripting surface and closes
the gap between the CLI and the MCP server. Every lookup table (`models`,
`tokens`, `version`) gains a machine-readable mode, the two MCP tools that
could never filter or rank (`list_models`, `repo_map`) catch up with their CLI
commands, and three guard scripts now exercise the parts of the project —
documented commands, installers, published releases — that a green test suite
never touched. One long-standing model-table lie (`qwen2.5`, a duplicate of
`qwen-2.5-72b` that had been leaking into prefix lookups since v0.1.0) is
finally gone, and `map --top` stops claiming "by name" while ranking by
tokens.

## Added

- **`version --json`.** Every other command already had a machine-readable
  mode; `version` only printed prose, so a script that wanted the commit or
  build date parsed the human line. `ctxpack version --json` now emits one
  compact JSON object with `name`, `version`, `os`, `arch`, `go`, `commit`
  and `built`. Anything beyond that single flag is rejected with exit code 2
  instead of being silently ignored, and `version` gets a `FLAGS (version)`
  section in `--help` and a row in the README command table.

- **`models --csv` and `tokens --csv`.** `models --csv` prints
  `name,context_window,vendor` — the header doubles as the schema — and
  `tokens --csv` prints `model,used,limit,fits,pct_used`, the same field
  names as `tokens --json`, so a consumer can switch formats without changing
  its schema. Both accept `--format csv` as the alias, and vendor/top/sort
  filters run before rendering, so the CSV rows always match what the text
  and JSON output would show for the same flags.

- **MCP `list_models` gains `vendor`, `top` and `sort`.**
  The CLI's `models` command has filtered and ordered the table for ages, but
  the MCP tool only offered `format:json` — an agent could see the whole
  registry or nothing. The tool now executes the same case-insensitive vendor
  filter, the same top-N-by-window truncation, and the same
  name/window/vendor ordering as the CLI, in both text and JSON output. The
  old "deliberately takes no filtering arguments" note is gone; the arguments
  are real, so a misspelled vendor still shows its effect (an empty table)
  instead of being silently ignored.

- **MCP `repo_map` gains `top`.**
  The CLI's `map --top N` has rendered a flat largest-files table since
  before v0.1.4, but the MCP tool only ever returned the tree. `repo_map
  top:N` now prints the same table — `Top N files`, ranked by tokens, or by
  bytes with `sort:bytes` — while `format:json` keeps the full tree
  regardless of `top`, matching how the CLI's JSON envelope ignores the flag.

- **`scripts/check-commands.sh` (`make commandscheck`).**
  Runs every ctxpack invocation the current docs show a reader — extracted
  from README.md, docs/ci.yml, docs/examples.md and docs/promote.md — against
  a real scratch repository, and fails on the first one the binary rejects.
  Release notes are skipped on purpose: they are history. Also confirms each
  `CTXPACK_*` variable README names is still read, by the CLI or an installer.
  Verified against eleven injected defects.

- **`scripts/check-installers.sh` (`make installerscheck`).**
  Cross-checks the two installers against each other and against what
  `make release` publishes: runs every row of `OSARCHES` through each
  source's own asset-name template, feeds a real Makefile-produced
  `SHA256SUMS.txt` through each installer's own parser, and asserts a name
  that is not listed does not match. Parses `install.sh` with `bash -n` and
  `install.ps1` with the PowerShell parser API.

- **`scripts/check-release.sh` (`make releasecheck`).**
  The only target that opens a network connection, so it is deliberately not
  in `make ci`: fetches a published release's manifest, confirms it names
  exactly the assets the Makefile builds, re-hashes the archive the Homebrew
  formula points at, and compares every scoop bucket hash with the manifest.
  `--install` also runs `install.sh` against a temp dir. The tap and bucket
  are siblings of this repository, read from `../homebrew-tap` and
  `../scoop-bucket`.

## Fixed

- **`map --top` header no longer claims "by name" while ranking by tokens.**
  `--sort` defaults to `name`, which is not a ranking key in the flat top-N
  mode, so `map --top 5` printed "Top 5 files by name" while the list was
  ordered by token estimate. It now echoes the effective key: "by tokens" by
  default, "by bytes" with `--sort bytes`.

- **`models` no longer advertises the legacy `qwen2.5` alias.**
  `qwen2.5` was a leftover from v0.1.0 with no hyphen; v0.1.5 added the
  canonical `qwen-2.5-72b`. Keeping both meant prefix lookups like
  `--model qwen2` silently resolved to the legacy spelling and the table
  listed the same model twice under different names. The alias is removed;
  the table is back to 30 models across 7 vendors, and
  `LookupModel("qwen2.5")` now resolves to nothing.

- **`install.ps1` now runs on Windows PowerShell 5.1.**
  The file contained two non-ASCII characters — an em dash in the header
  comment and an ellipsis in a `Write-Host` — and no UTF-8 BOM. PowerShell
  5.1 read the BOM-less script in a single-byte code page, so the closing
  quote was not recognised and the whole file failed to parse before doing
  anything, while PowerShell 7 accepted the same bytes. The file is now pure
  ASCII, verified by `make installerscheck`.

- **`diff` reported two different file counts in one document.**
  The header comment was built from the pre-filter change list while
  `<fileCount>` was built from what actually got packed, so with a filter in
  force the two disagreed. Both now come from the same packed list.

- **`diff --list` ignored `--include`, `--exclude` and every other filter.**
  `--list` now runs the same walk filters as the pack it describes, and the
  smoke suite asserts `--list` names deletions too — a path whose content is
  gone still appears, exactly as it does in the pack's `Deleted` section.

- **One file printed as "1 files".** The plural helper now returns "file" for
  a count of one everywhere the packer and formatter report counts.

- **`install.sh` retried detecting the release but not downloading it.**
  `curl --retry` now covers the download too, so a transient connection
  reset no longer aborts the install.

- **`go test ./...` failed in a downloaded source archive.**
  The CLI test that builds a scratch repository used the process' current
  directory, which is not a checkout in a source tarball — the exact input
  Homebrew builds from. It now creates its own repository.

- **`docs/examples.md` quoted numbers that no longer matched the demo.**
  Regenerated from a fresh `ctxpack-demo` checkout; `scripts/check-examples.sh`
  now fails if the committed snapshots drift from a fresh run.

- **README's flag table made two claims the binary contradicts** (the
  `-o`/`--output` scope and the models `--json` description). The table now
  says what the commands actually accept, and the help text is pinned to the
  flag registrations by a regression test.

- **Thirteen advertised MCP arguments had no description.** Every argument in
  every tool schema now declares a type and a description, and a test
  enforces it, so a client asking what `no_gitignore` does gets an answer.

- **`server.go`'s package doc drifted from the schema it describes.**
  The doc promised the argument lists are declared once in `tools()` and
  documented nowhere else — they had been spelled out by hand and had
  drifted. The doc now matches the single source of truth.

- **A workflow that can never run had nothing to stop it rotting.**
  `docs/ci.yml` lives in docs/ because the publishing token lacks the
  `workflow` scope, so nothing ever executes it. `make cicheck` now
  syntax-checks every run: block, verifies the scripts and targets it names
  exist, and fails if its gate drifts from `make check`.

- **`examples/diff-demo` documentation and generator bugs.**
  Its README said `--list` "deliberately hides deletions" — obsolete since
  `--list` was fixed to name them — and `make.sh` wrote to the wrong file
  when run as documented.

## Testing

- **The smoke suite is a single script** (`scripts/smoke.sh`) that CI calls
  identically, instead of a second copy in the Makefile. Every assertion
  reports what failed. `doctor`, the file-selection flags, all six MCP tools
  and `--dry-run` are now part of the suite.
- **Two packages hit 100%** coverage during this cycle (`counter`, `repomap`),
  and the CLI suite sits at 99.2% with the MCP server at 97.0% — every one of
  the "unreachable" branches that remained had a real trigger someone wrote a
  test for.