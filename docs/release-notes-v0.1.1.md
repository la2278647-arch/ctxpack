# v0.1.1

Bug-fix release. No new features.

## Fixed

- **Byte counts were 1024x too large.** `pack` and `tokens` reported a 10-byte
  file as 10 KB.
- **`pack --format xml` could emit XML that does not parse.** Paths and the
  `tokens`/`bytes` attributes were unescaped, and a `]]>` inside a file body
  closed its CDATA section early. Output now round-trips through a real XML
  parser, including a file literally named `a&b/x < y.go`.
- **An unknown `--format` silently produced XML.** It is rejected now, and
  `CTXPACK_FORMAT` is validated the same way.
- **`map` rendered the tree root as `./`.** The root is now labelled with the
  walked directory's own name.
- **`diff` could not see renames or quoted paths.** `git status --name-status`
  is split on tabs (a rename's whole line was previously one path) and git's
  C-style quoting is unquoted.
- **`mcp`'s `repo_map` under-counted by orders of magnitude.** With
  `ReadContent: false` the estimate came from the file's own name — a 40 KiB
  file read as about one token. It now comes from file size.
- **A boolean flag made every flag after it disappear.** `--format json
  --budget 500` dropped `--budget`.
- **Flags after the path were silently ignored.** Go's `flag` package stops at
  the first non-flag token, so `ctxpack pack ./repo --format markdown` returned
  XML. Positionals now move to the end of the argument list before parsing.
- **`--version` printed `gogo1.26.5`.** `runtime.Version` already carries the
  `go` prefix.
- **The documented `-o` shorthand did not exist.** Registered on `pack` and
  `diff`.
- **`mcp` returned a generic error for a `tools/call` with no tool name.** It
  now reports `tools/call is missing arguments.name`.

## Tests

The previously untested packages now carry suites, including table tests for
every parser and one end-to-end pack per format:

| package   | before | after |
| --------- | ------ | ----- |
| version   |  0.0%  |100.0% |
| format    |  0.0%  | 98.8% |
| repomap   |  0.0%  | 98.2% |
| gitutil   |  0.0%  | 95.4% |
| mcp       |  0.0%  | 92.9% |
| walker    | 73.8%  | 91.3% |
| cli       |  0.0%  | 56.7% |

`go test ./...`, `go vet ./...` and `gofmt -l .` are all clean.

## Note on v0.1.0's assets

The binaries attached to `v0.1.0` were built at commit `5a2017d`, which is five
commits after that release's tag. Take this release instead.

## Install

```sh
go install github.com/la2278647-arch/ctxpack@v0.1.1
```

Or take a binary from this release. Binaries are built with `-trimpath` and
`CGO_ENABLED=0`, so they are statically linked and portable.

## Binaries

`ctxpack_0.1.1_<os>_<arch>` — windows (amd64, 386, arm64), linux (amd64, 386,
arm64), darwin (amd64, arm64). A `SHA256SUMS.txt` is attached.
