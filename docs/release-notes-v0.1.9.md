# v0.1.9

The release that closes the gap between what the tool advertises and what the
tool does. Six of the seven fixes below were not found by reading the code but by
pointing a test or a smoke assertion at the boundary between a documented
contract and the implementation behind it.

## Fixed

- **`pack_repo` with `format: "json"` and a `model` returned unparseable JSON.**
  The MCP tool prepended the HTML comment the text formats carry — `<!-- fit:
  OVERFLOW model=gpt-4 used=267.6k/4.1k (6533%) OVERFLOW -->` — in front of the
  JSON document, so any client that `JSON.parse`s the tool result threw on every
  annotated call. The CLI sidesteps this by routing the note to stderr, but an
  MCP client only ever sees the envelope. The annotation now travels inside the
  envelope as a `fit` object using the same keys `count_tokens --json` already
  uses for its per-model entries (`name`, `vendor`, `window`, `limit`, `used`,
  `pct_used`, `fits`, `reserve`), so the information is still there and the
  document stays valid. `diff_repo` shares the path and gets the same treatment.
  Text formats keep the comment, and an unknown model yields
  `fit: {"model": "...", "unknown": true}` instead of destroying the envelope.
  `fitNote` now derives both renderings from one place, and the smoke suite
  asserts the `fit` key survives.

- **`doctor -o FILE` did not work.**
  The help text advertises `-o, --output FILE` on all commands, but `doctor` only
  registered the long form. The shorthand failed with `flag provided but not
  defined: -o` and exited 2, so anyone scripting `ctxpack doctor -o report.txt`
  got a usage screen instead of a report. Both spellings now write to the file,
  matching `pack`, `map`, `diff`, `tokens` and `models`.

- **The help text described flags the commands do not take.**
  `printHelp` called the walk flags "map / tokens only" when `pack` and `diff`
  accept them too, filed `--depth` under a three-command group while four commands
  take it, and told `diff` it "also accepts" only eight of the sixteen flags it
  registers, omitting `--max-size`, `--depth`, `--no-gitignore`, `--hidden`,
  `--output`, `--dry-run` and `--list`. There is now one WALK FLAGS group naming
  the four traversing commands plus a complete section per command, with no scope
  note the code contradicts.

- **The README's MCP tool table was missing `doctor`.**
  `doctor` shipped in v0.1.8 but the table never gained its row, so a client
  reading the README concluded that a failing pack could not be diagnosed from
  inside the server — the exact case `doctor` exists for. The prose beside it
  called the analytic tools "three" when there are four. `doctor` is now listed
  with its `format?` / `top?` arguments, the `list_models` footnote moved out of
  the cell so the argument lists stay machine-readable, and the `fit` object is
  described where `format: "json"` and `model` meet. The README's install
  snippets also still pinned `v0.1.7` and the Docker examples `0.1.4`.

- **`make smoke` could destroy uncommitted `README.md` edits.**
  The diff test dirties `README.md` to exercise `diff`, then restored it with
  `git checkout -- README.md`. That restores from the index, so anyone who ran
  the smoke suite with uncommitted README work lost it without warning. It now
  copies the file aside and copies it back, preserving the exact bytes that were
  there before the run. The finding was self-inflicted: the MCP table edit was
  applied, the suite was run, and the edits came back missing from the commit.

- **Twenty-three smoke assertions failed silently.**
  `scripts/smoke.sh` ran under `set -euo pipefail`, but 23 assertions were bare
  `grep` calls: when one failed the script aborted with no message at all — a wall
  of passing sections followed by silence, with no way to tell which check broke.
  All of them now go through `fail`, which prints a specific reason. The bug had
  stayed hidden for a structural reason: `fail` was defined near the bottom of
  the script, after the assertions it was meant to serve, so those sections could
  not have used it. It now sits at the top. Wiring the `doctor --output`
  assertion up is what surfaced the `-o` bug above, which had been a silent
  no-op.

## Added

- **Regression tests pin the documentation to the code.**
  `internal/cli/help_test.go` walks the union of every flag against all six
  commands and asserts that each advertised flag parses (exit 0) and each
  non-advertised one is rejected by the parser (exit 2). Two further tests read
  the real `printHelp` output and check both directions: every documented flag
  must be registered, and no section may document a flag the command rejects.
  `internal/mcp/readme_test.go` parses the README's tool table and compares it
  both ways against the schemas `tools()` returns, so a tool added to or removed
  from the server fails it, as does a required/optional mismatch. Both are
  mutation-checked: deleting `doctor`'s `-o` registration fails the help test
  with exactly `cmd doctor: -o accept (exit 2, want 0)`, and deleting the
  `doctor` row from the table fails the README test with `the table does not
  document doctor; the server advertises it`.

- **The `Run` dispatcher and three CLI error branches now have tests.**
  `internal/cli/cli_gaps_test.go` drives `cli.Run` — the function `main.go`
  actually calls — through its `doctor` case, which was the one command never
  dispatched by a test, so a typo in the case label would have shipped as
  "unknown command" for a command the help text advertises. The same file adds
  the near-miss check (`Run(["docter"])` must hit the unknown-command branch),
  the `tokens --model` OVERFLOW verdict with the reverse case proving the mark is
  a property of the repository/model pair rather than of the fixture, and
  `diff --dry-run`'s "omitted by the budget" line with the converse case proving
  it follows the cap rather than printing unconditionally.
  `TestDoctorRejectsExtraArgs` previously rejected only an unknown *flag*, which
  made its name overclaim — flag parsing and positional-argument rejection are
  two different branches with two different messages, and it now covers both and
  asserts both.

- **The fit-report selection and sorting contracts are pinned.**
  Tests now fix the order `--sort tokens` / `--sort bytes` produce, which file
  wins a tie, and that `--top` truncates the text breakdown while the JSON
  payload stays complete.

## Test coverage

| package | v0.1.8 | v0.1.9 |
| ------- | ------ | ------ |
| cli     | 97.6%  | 99.4%  |
| counter | 100%   | 100%   |
| doctor  | 98.0%  | 98.0%  |
| format  | 99.3%  | 99.3%  |
| gitutil | 100%   | 100%   |
| ignore  | 98.0%  | 98.0%  |
| mcp     | 97.1%  | 97.3%  |
| packer  | 100%   | 100%   |
| repomap | 100%   | 100%   |
| version | 100%   | 100%   |
| walker  | 94.4%  | 94.4%  |

423 tests, all passing, up from 389. Every remaining uncovered branch in the
suite is one that cannot be reached: a `filepath.Abs` failure that needs the
working directory deleted mid-call, a `json.Marshal` failure over values that
cannot fail to marshal, and `outputWriter`'s `os.Create` failure, which calls
`os.Exit(1)`. They are marked in comments rather than tested around. The suite
still asserts `go.sum` stays absent and `go list -m all` reports exactly one
module, so the binary remains dependency-free.

## Installation

```sh
# Homebrew
brew tap la2278647-arch/tap https://github.com/la2278647-arch/homebrew-tap
brew install ctxpack

# Go
go install github.com/la2278647-arch/ctxpack@v0.1.9

# Windows PowerShell
irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex

# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash

# Direct download
# https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.9
```

## What's next

- GitHub Actions CI workflow (blocked on `workflow` scope token)
- Docker daemon testing of the multi-stage Dockerfile
- Model table expansion (blocked: context windows cannot be verified)
- Promotion posts (drafts in `docs/promote.md`)
