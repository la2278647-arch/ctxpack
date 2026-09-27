# v0.1.12

The release that finishes the shell story and closes the last MCP gap.
Bash, zsh and fish completions arrive for every command and flag — and a
smoke-suite check now proves the completion tables against the help text
instead of trusting a hand-maintained copy. `version --short` gives scripts a
bare version number, the MCP server gains a `version` tool so an agent can
ask about the server without wading through a full environment report, and
`-h` finally means the same thing on every subcommand. The (still inactive)
GitHub Actions workflow document also caught up: it now runs the remaining
local guards, produces the checksum manifest the installers parse, and tests
Go up to 1.26.

## Added

- **Bash, zsh and fish shell completion.**
  `scripts/completion.bash`, `scripts/completion.zsh` and
  `scripts/completion.fish` complete command names and each command's own
  flags — `ctxpack tokens <Tab>` offers `--format`, `--csv`, `--sort`,
  `--top`, `--model` and the walk flags; `ctxpack models <Tab>` offers
  `--vendor` and friends. Source the bash or zsh script from your shell rc,
  or drop the fish script into `~/.config/fish/completions/`. All three are
  exercised in the smoke suite.

- **`smoke` now verifies the completion tables against the help text.**
  The completion flag tables are maintained by hand, and nothing checked them
  against `--help`. The smoke suite extracts every command's `FLAGS (…)`
  section and asserts, in both directions for all three scripts, that the
  offered flags are exactly the documented ones. The check caught a real
  drift in its first run: `pack`/`diff` accept `-q, --quiet` but the scripts
  only offered `-q`, so the long form is now included everywhere; the fish
  parser was pinned to the quoted `-n` list so a description containing a
  command name ("…the pack will name") cannot leak a flag onto the wrong
  command.

- **`version --short` prints just the semantic version.**
  `version` always printed the full banner, so a script that wanted "0.1.12"
  alone had to parse prose or read a JSON field. `version --short` — under
  all three aliases (`version`, `--version`, `-v`) and identical across them —
  prints one line containing only the version. Combining it with `--json` or
  anything else is rejected with exit code 2, matching the existing `--json`
  discipline.

- **MCP `version` tool.**
  The server exposed six tools for the six file-oriented CLI commands and
  left `version` out, so an agent that only wanted to check whether the
  server was current had to call `doctor` and read the whole environment
  report. The new tool answers with the build identity alone — text or, with
  `format:"json"`, the exact envelope the CLI's `version --json` prints. Its
  schema accepts only `text`/`json`, matching `doctor`.

- **`-h`/`--help` now returns 0 on every subcommand, and `help <command>`
  validates its argument.**
  `ctxpack pack -h` used to exit 2 — the flag parser treats an undefined
  `-h` as a parse error and writes help to stderr — while `ctxpack models -h`
  returned 0. Every subcommand now handles `-h` itself: prints the shared
  help to stdout and returns 0. `ctxpack help pack` does the same, but
  `ctxpack help bogus` names the unknown command and returns 2 instead of
  silently ignoring the argument.

- **`docs/ci.yml` test job gains a `local audits` step, and the release job
  ends with a checksum step.**
  The (deliberately inactive, audit-only) workflow's test job ran gofmt, vet,
  the test suite and smoke, but not the remaining `make ci` guards —
  `commandscheck`, `installerscheck`, `examplescheck`, `cicheck` — so it
  would have been less strict than a local checkout. The new step runs those
  four scripts. The release job built the eight platform binaries but never
  produced `SHA256SUMS.txt`; a `checksum` job now writes the manifest with
  the same command `make release` uses, in the exact text-mode format both
  installers parse.

## Fixed

- **`commandscheck` reported real environment variables as unread.**
  The variable check searched `internal/` (Go `"CTXPACK_X"` literals) and the
  two installers, but `scripts/check-release.sh` reads `CTXPACK_TAP` and
  `CTXPACK_BUCKET` through `${CTXPACK_TAP:-…}` expansion — a genuine read the
  guard's narrow scope could not see, so documenting those two variables in
  the README immediately turned the guard red. The check now also greps
  `scripts/` with the same plain-name pattern the installers use.

- **`docs/ci.yml` Go matrix now covers 1.21 through 1.26.**
  The matrix stopped at Go 1.23 while current releases had moved on. The
  upper end now tracks the latest releases; 1.21 stays as the declared
  minimum in go.mod.