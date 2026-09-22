# v0.1.2

Bug-fix and hardening release. Three things that now report what they do,
fourteen that were plainly wrong, and the tests that would have caught each of
them.

Statement coverage went from 82.0% to 97.5%.

## Added

- **The budget reports what it cut.** `format.Bundle` gained `Omitted` (paths)
  and `OmittedTokens`, both `omitempty`, so an unlimited pack keeps its exact
  shape. XML renders an `<omitted>` element, Markdown appends an "Omitted by
  budget" section, text prints an `==== omitted by budget ====` block, and
  `pack --output f` says so on stderr instead of printing a bare count.
  `LICENSE` and the other policy documents, which used to rank dead last, are
  now recognised, along with the missing tiers (`*.proto`, `*.graphql`,
  `*.thrift`, `server.js`, `manage.py`, `*.rst`, `Dockerfile`, `Makefile`,
  `test_*.py`).

## Fixed

- **`--format json` did not produce JSON.** `pack --model` and `diff` always
  prepended an HTML comment, so every JSON bundle failed to parse. The note and
  the header are still printed, to stderr, where a program consumer does not
  read them.
- **`a/**/b` matched `a/xby`.** `**/` was translated by consuming the slash,
  so the separator between the two glob parts was lost.
- **Indented comments became patterns.** Git strips leading whitespace unless
  it is escaped, and ctxpack only trimmed trailing whitespace, so an indented
  `!foo` silently became a negation.
- **`--max-size` was documented as "skip".** It does not skip files: a file over
  the limit is still listed with its real size, just never read. Corrected in
  the CLI help, four flag registrations, the README, and the MCP `pack_repo`
  schema — the last of which is what an LLM reads before calling the tool.
- **`--budget` ranked every test as source code.** The `_test.go` tier was
  placed after the general source case, so tests scored 200 instead of 50.
- **`map` could lose a file that shared its name with a directory.** A folder
  matched children on the name alone, so a file and a directory of the same
  name folded into one node.
- **The XML and Markdown renderers could not be read back exactly.** `<content>`
  was pretty-printed, giving every file a leading and a trailing blank line.
  Both now round-trip byte for byte.
- **`--model ""` matched `gpt-3.5-turbo`.** `strings.HasPrefix(name, "")` is
  true for every registered model, so an unset value resolved to the smallest
  window on the list.
- **`EstimateSize` returned a negative token count for a negative size.**
- **The "±15%" accuracy claim was not earned.** Nothing had ever been measured
  against a real tokenizer, so the heuristic and the measured skew are stated
  instead.
- **`Match("")` reported "ignored"**, because `*` and `**` compile to regexes
  that match the empty string.
- **The walker contained dead code that lied about `--include`.** Both arms of
  the conditional returned `nil`, and the comment above it claimed hidden files
  could be brought in with `--include`. They cannot, and will not: `--include`
  narrows a scan, it is not an escape hatch from the hidden-file default. If it
  did leak `.env`, a secret would reach a prompt without the user ever opting
  in.
- **`ctxpack mcp` could not be tested.** `cmdMCP` hard-coded `os.Stdin` and
  `os.Stdout`, so the dispatch could only be reached by driving a live JSON-RPC
  loop over the process's real streams. It now takes `io.Reader`/`io.Writer`.

## Tests

| package   | v0.1.1 | v0.1.2 |
| --------- | ------ | ------ |
| version   |100.0%  |100.0%  |
| packer    | 80.0%  |100.0%  |
| mcp       | 93.1%  |100.0%  |
| cli       | 57.6%  | 99.1%  |
| format    | 98.9%  | 99.1%  |
| repomap   | 98.2%  | 98.1%  |
| ignore    | 81.1%  | 97.0%  |
| gitutil   | 95.4%  | 95.4%  |
| counter   | 74.4%  | 95.3%  |
| walker    | 91.3%  | 92.7%  |
| **total** |**82.0%**|**97.5%**|

`go test ./...`, `go vet ./...` and `gofmt -l .` are all clean. Two lines
remain uncovered in the whole repository — `cmdDiff`'s `packer.Pack` error
branch and the walker's `filepath.Abs` error branch — and each needs a
filesystem condition no test can produce reliably. Both are commented in the
source for the reason they are unreachable.

## Install

```sh
go install github.com/la2278647-arch/ctxpack@v0.1.2
```

Or take a binary from this release. Binaries are built with `-trimpath` and
`CGO_ENABLED=0`, so they are statically linked and portable.

## Binaries

`ctxpack_0.1.2_<os>_<arch>` — windows (amd64, 386, arm64), linux (amd64, 386,
arm64), darwin (amd64, arm64). A `SHA256SUMS.txt` is attached.
