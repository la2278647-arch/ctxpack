# v0.1.4

The release that makes the installers trustworthy, and closes out the
machine-readable surface. Statement coverage is 98.8% of 965 statements; seven
of the ten non-`main` packages sit at 100%, unchanged from v0.1.3 because this
release adds surface without changing the testable core.

## Added

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
  argument-taking tool that ignored its arguments would hide typos, which is
  the bug that just landed on `ctxpack models`.

- **`ctxpack models --json`.**
  That completes the machine-readable surface: `pack --format json` for the
  bundle, and `--json` for `map`, `tokens` and now `models`. The model table is
  what an LLM reads before deciding whether a repo fits at all, so it belongs
  in the same scriptable shape as the fit it produces.

  `models --json` wraps the table in `{models: [...]}`, one entry per
  registered model carrying `name`, `vendor`, `context_window` and `limit`.
  `limit` is the window minus the 4096-token reply reserve — the same value
  `tokens --json` reports inside each fit entry — so the two outputs join
  without a script recomputing the subtraction.

- **`ctxpack models` no longer swallows unknown arguments.**
  It was the only command using a manual `args[0]` check instead of a
  `flag.FlagSet`, so `ctxpack models --bogus`, `ctxpack models --json` and a
  bare `ctxpack models` were all indistinguishable: a typo'd flag was silently
  ignored and the table printed anyway. It now parses with `reorderArgs` like
  the rest, and a stray argument exits 2 naming the offender.

  `models -h` still exits 0 rather than 2, unlike every other command. That is
  a test-asserted quirk so it was preserved: the help check runs before the
  FlagSet parse. Fixing the inconsistency the other way would break a
  documented exit code for no gain.

## Fixed

- **`install.ps1` failed to find the latest release whenever GitHub rate-limited it.**
  Untagged installs call the REST API, which allows 60 anonymous requests per
  hour; on a busy machine that 403s and the install aborts. It now falls back
  to the `/releases/latest` page, which 302-redirects to `/releases/tag/vX.Y.Z`
  and is not rate-limited. That fallback reads the `Location` header off the
  raw response, because `Invoke-WebRequest -MaximumRedirection 0` throws an
  exception whose `Headers['Location']` comes back empty. `install.sh` gets the
  same fallback.

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
  `cli` declared it as a constant; `mcp` typed `4096` inline. Nothing was
  wrong today, but changing the reserve in one place would have left the other
  surface reporting a different `limit` with no failure and no warning. Both
  now read `counter.ReplyReserve`, defined next to the `FitsModel` that
  consumes it, and `cli.fitReserve` is an alias rather than a restatement.

- **The README's own JSON example had a wrong number.**
  It showed `gpt-4o` with `"limit": 119808`, which is off by exactly 4096.
  `gpt-4o`'s window is 128000, so the limit is 123904; the `pct_used` of
  26.58 was computed against the wrong window and is 25.7. Caught by cross-
  checking the example against `ctxpack models`'s table rather than by eye.

## Coverage

| package   | v0.1.3 | v0.1.4 |
| --------- | ------ | ------ |
| version   |100.0%  |100.0%  |
| packer    |100.0%  |100.0%  |
| mcp       |100.0%  |100.0%  |
| cli       |100.0%  |100.0%  |
| format    | 99.1%  | 99.1%  |
| repomap   |100.0%  |100.0%  |
| ignore    | 98.0%  | 98.0%  |
| gitutil   |100.0%  |100.0%  |
| counter   |100.0%  |100.0%  |
| walker    | 94.4%  | 94.4%  |
| **total** |**98.7%**|**98.8%**|

The four commands that gained `--json` are held by invariants rather than by
sample-output snapshots. `models --json` is asserted to list every one of the
19 registered models and every 7 vendors, and to agree with the text output
line by line; its `limit` for every model is asserted to equal
`context_window - counter.ReplyReserve`, which is the relationship the README
example got wrong. `list_models` is asserted to emit one line per registered
model, and a cross-check parses the limits out of both `list_models` and
`count_tokens` and fails if they disagree on any model.

The eleven tests added since v0.1.3 cover the new tool's registration and
argument shape, the new flag, the two unknown-argument cases, the reserve alias
and the cross-tool limit agreement. `counter`, `mcp` and `cli` remain at 100%.

Twelve statements remain uncovered, at the same twelve places as v0.1.3 — each
is defended by its inputs rather than by a claim of laziness:

- **`main.go` (2)** — `main()` itself. The test harness cannot call it.
- **`walker` (7)** — `filepath.Abs` needs the process cwd deleted mid-call;
  `filepath.Rel` needs two paths on different volumes, which one `WalkDir` root
  cannot produce; `DirEntry.Info()` needs a file to vanish between `ReadDir` and
  `Info`; `WalkDir` itself returns a non-nil error only if its callback returns
  one, and this callback never does; and the two `globRegex` failures need
  `ignore.translateGlob` to emit a regex it cannot compile, which it provably
  cannot because it escapes every regex-special character.
- **`ignore` (2)** — `filepath.Abs` needs the cwd deleted mid-call, and
  `regexp.Compile` needs an unescaped pattern.
- **`format` (1)** — `json.MarshalIndent` cannot fail for `*Bundle`: every field
  is a string, an int, a bool or a slice of those. No floats, maps or cycles.

`go test ./...`, `go vet ./...` and `gofmt -l .` are all clean. `go.sum` does
not exist: the module imports nothing outside the standard library, and `make
check` fails the build if it appears.

## Install

```sh
go install github.com/la2278647-arch/ctxpack@v0.1.4
```

Or take a binary from this release. Both installers now verify against
`SHA256SUMS.txt` before anything lands on `PATH`, install atomically, and fall
back to the unrate-limited `/releases/latest` redirect when the REST API
403s.

## Binaries

`ctxpack_0.1.4_<os>_<arch>` — windows (amd64, 386, arm64), linux (amd64, 386,
arm64), darwin (amd64, arm64). Eight in all. Binaries are built with `-trimpath`
and `CGO_ENABLED=0`, so they are statically linked and portable. A
`SHA256SUMS.txt` is attached.
