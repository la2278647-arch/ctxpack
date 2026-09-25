# v0.1.8

The release that gives the MCP server a way to explain itself. A new `doctor`
tool answers the question an agent cannot otherwise ask, and the whole smoke
suite finally moved into one script that the CLI, the Makefile and CI all share.

## Added

- **`doctor` is an MCP tool.**
  An agent that got `git error: exit status 128` from `diff_repo` had no way to
  tell whether git was missing, on the wrong version, or whether the ref was
  simply wrong — and the one diagnostic that answers that required a `path`,
  which is exactly what the agent may be unable to name. `doctor` takes no path
  at all: it reports the server's version, Go version, platform, git availability
  and version, and the model registry broken down by vendor. `top` truncates
  only the text breakdown; the `N models, M vendors` total and the
  `format: "json"` payload always stay complete, so a truncated answer cannot
  understate the registry. It accepts just `text` and `json` and refuses
  anything else — a client cannot request a format it would silently get back
  as text.

- **`scripts/smoke.sh`: the smoke suite as one script.**
  It drives every command, all four output formats, the
  `--sort`/`--top`/`--format`/`--dry-run`/`--list` flags, `diff` with a
  `--ref A..B` range, a deletion across all four formats, and the MCP server over
  stdio — against the source tree itself. `make smoke` now delegates to it, so
  there is one copy instead of a Makefile recipe and a CI recipe that could drift
  apart. The script finds the repository root from its own path, so it runs from
  any working directory and accepts an explicit binary path as an optional first
  argument.

- **`doctor`, the file-selection flags, and all six MCP tools are in the smoke
  suite.**
  `doctor` was documented and unit-tested but never exercised end to end. Nor
  were `--include`, `--exclude`, `--max-size`, `--depth`, `--hidden`,
  `--no-gitignore`, `map --csv` or `models --vendor`. The suite now asserts each
  one's contract with relative counts rather than pinned numbers, so it keeps
  passing as the repository grows: `--depth` must shrink the file set,
  `--max-size` must leave the count alone while collapsing the token total,
  `--include` and `--exclude` must each shrink it, `--hidden` must add files,
  and `--no-gitignore` can only keep or add. On the MCP side the suite asserts
  that `tools/list` advertises all six tools, that each is callable, and that
  five failure modes are reported correctly.

## Changed

- **`doctor` moved into a shared package.**
  The CLI's `doctor` command and the MCP `doctor` tool now both call
  `internal/doctor` instead of each implementing the checks. That is the same
  "frontends share an engine" rule the packer and repomap packages already follow,
  and it removes about a hundred lines of duplicated git probing and vendor
  sorting from `internal/cli`. The JSON keys the CLI has always emitted are
  unchanged, so a script that parses `ctxpack doctor --json` keeps working.

- **`--dry-run` writes its report to stderr, and the smoke suite asserts it.**
  The suite used to discard dry-run output with `> /dev/null`, which silently
  failed to silence it — this codebase sends artifacts to stdout and status to
  stderr, and a dry run produces status, not an artifact, so the redirect
  captured nothing at all. The assertions now capture stderr and require the
  report to be present, so the stream convention is pinned instead of tolerated.

- **The CI smoke test calls `scripts/smoke.sh` instead of inlining its steps.**
  The inlined copy had two bugs the script run surfaced: it grepped for `"tree"`
  in MCP output, where the tool result arrives escaped as `\"tree\"` inside the
  JSON-RPC envelope string, and it used `> /dev/null` to silence dry-run output
  that travels on stderr.

- **The README MCP table was corrected.**
  It omitted `pack_repo`'s `max_depth`, and the introductory sentence implied
  only `repo_map`, `count_tokens` and `list_models` take a `format` when all six
  tools do. The smoke suite now exercises `max_depth` through the MCP interface
  so this cannot quietly regress.

## Test coverage

| package | v0.1.7 | v0.1.8 |
| ------- | ------ | ------ |
| cli     | 96.6%  | 97.6%  |
| counter | 100%   | 100%   |
| doctor  | —      | 98.0%  |
| format  | 99.3%  | 99.3%  |
| gitutil | 100%   | 100%   |
| ignore  | 98.0%  | 98.0%  |
| mcp     | 97.4%  | 97.1%  |
| packer  | 100%   | 100%   |
| repomap | 100%   | 100%   |
| version | 100%   | 100%   |
| walker  | 94.4%  | 94.4%  |

389 tests, all passing. The suite still asserts `go.sum` stays absent and
`go list -m all` reports exactly one module, so the binary remains dependency-free.

## Installation

```sh
# Homebrew
brew tap la2278647-arch/tap https://github.com/la2278647-arch/homebrew-tap
brew install ctxpack

# Go
go install github.com/la2278647-arch/ctxpack@v0.1.8

# Windows PowerShell
irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex

# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash

# Direct download
# https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.8
```

## What's next

- GitHub Actions CI workflow (blocked on `workflow` scope token)
- Docker daemon testing of the multi-stage Dockerfile
- Model table expansion (blocked: context windows cannot be verified)
- Promotion posts (drafts in `docs/promote.md`)
