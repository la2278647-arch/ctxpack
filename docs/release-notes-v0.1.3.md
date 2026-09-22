# v0.1.3

The first release that was actually published. Statement coverage is 98.7% of
942 statements, and seven of the ten packages sit at 100%.

## Added

- **`ctxpack map --json` and `ctxpack tokens --json`.**
  `pack` had `--format json` for its bundle, but `map` and `tokens` spoke only
  human text, so nothing outside a human could read a tree or a per-model fit
  table. These two have no format family, so they take a `--json` boolean.

  ```
  map --json    -> {root, total_tokens, total_bytes, tree}
  tokens --json -> {path, total_tokens, total_bytes, reserve_tokens, fits}
  ```

  Two invariants are tested rather than merely intended. `tree.children` is
  always an array and never `null`, so a file and an empty directory both read
  `[]` and `is_dir` is what separates them — a consumer never guesses from a
  missing key. And the header totals equal the tree totals and agree with the
  text output, because every format reads one shared walk and cannot drift.

  Each fit entry carries `used`, `limit` (the window minus the reply reserve)
  and `pct_used` rounded to two decimals, so the JSON stops echoing float64
  noise such as `1258.287899747742`. The reply reserve is now `fitReserve`, a
  named constant instead of a literal repeated at two call sites, so the text
  table, the JSON and `pack`'s fit annotation all read one value.

- **A project to try ctxpack on: `ctxpack-demo`.**
  [la2278647-arch/ctxpack-demo](https://github.com/la2278647-arch/ctxpack-demo)
  is a 27-file FastAPI microservice with cursor pagination, pydantic validation,
  a 58 KB synthetic catalog and a test per behaviour. It carries ctxpack's own
  output captured on it, so the tool is shown rather than described: `tokens .`
  reports 35545 estimated tokens with `gpt-3.5-turbo` at 289% and `gpt-4` at
  868% marked `[OVERFLOW]`; `map .` shows `data/seed.json` alone at 26178
  tokens, 73.6% of the tree; `pack . --budget 5000` keeps 16 files at ~4938
  tokens and prints the six it cut with their individual counts.

- **`.gitattributes` pinning `eol=lf`.**
  A fresh clone on a machine with `core.autocrlf=true` checks out as CRLF. Git
  reverses the conversion when comparing, so it reports no diff at all — but
  `gofmt -l .` disagrees and then flags every Go file in the repository as
  unformatted. The trap is invisible in CI and only appears on a first clone on
  Windows, which is exactly where a contributor will hit it.

## Coverage

| package   | v0.1.2 | v0.1.3 |
| --------- | ------ | ------ |
| version   |100.0%  |100.0%  |
| packer    |100.0%  |100.0%  |
| mcp       |100.0%  |100.0%  |
| cli       | 99.1%  |100.0%  |
| format    | 99.1%  | 99.1%  |
| repomap   | 98.1%  |100.0%  |
| ignore    | 97.0%  | 98.0%  |
| gitutil   | 95.4%  |100.0%  |
| counter   | 95.3%  |100.0%  |
| walker    | 92.7%  | 94.4%  |
| **total** |**97.5%**|**98.7%**|

The eleven tests added since v0.1.2 cover both JSON shapes, both invariants,
the empty-tree case, `toJSONNode` directly, `round2`'s rounding, and
`writeEnvelope`'s error branch — the last by handing the encoder a writer that
refuses every byte, so no test hook is needed.

Twelve statements remain uncovered, and each one is now justified by its inputs
rather than by a claim of laziness:

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

The branch that looked most tempting to skip — `writeEnvelope`'s marshal error
— was made reachable instead, using the failing-writer trick, which is what took
`cli` back to 100.0%.

`go test ./...`, `go vet ./...` and `gofmt -l .` are all clean. `go.sum` does
not exist: the module imports nothing outside the standard library, and `make
check` fails the build if it appears.

## Install

```sh
go install github.com/la2278647-arch/ctxpack@v0.1.3
```

Or take a binary from this release.

## Binaries

`ctxpack_0.1.3_<os>_<arch>` — windows (amd64, 386, arm64), linux (amd64, 386,
arm64), darwin (amd64, arm64). Eight in all. Binaries are built with `-trimpath`
and `CGO_ENABLED=0`, so they are statically linked and portable. A
`SHA256SUMS.txt` is attached.
