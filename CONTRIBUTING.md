# Contributing

Thank you for taking a look. ctxpack is small and deliberate about staying
small, so the bar for adding code is high — but the bar for fixing bugs and
improving docs is low.

## Before you start

1. **Check for an existing issue.** If one exists, comment there.
2. **If it would add a dependency, open an issue first.** This project is
   stdlib-only by design (see "Why stdlib only" below). Do not open a PR that
   adds a module to `go.mod` without agreement.
3. Small PRs land faster than big ones.

## Setup

```sh
git clone https://github.com/la2278647-arch/ctxpack
cd ctxpack
go build -o bin/ctxpack .
go test ./...
```

Requires Go 1.21+. Nothing is downloaded: the module has no dependencies.

## Working

```sh
git checkout -b my-change
# ...edit...
gofmt -w $(git diff --name-only HEAD -- '*.go')
go vet ./...
go test ./...
git push -u origin my-change
```

The CI checks must be green before a PR merges. If you changed behaviour, add a
test. If you changed a user-facing flag, update `README.md` and the help text
in `internal/cli/cli.go` in the same commit.

## Project conventions

**Stdlib only.** No `cobra`, `urfave/cli`, `viper`, `jsoniter`, `tiktoken-go`,
or anything else. If you find yourself reaching for a library, first ask
whether ~50 lines of Go and one function will do the job. The whole binary
currently imports nothing outside the standard library, and that is a feature,
not an accident: it is what makes the single-file, offline, air-gapped story
true.

**Read the packages before editing them.** The layering is:

```
main.go  ->  internal/cli  ->  internal/{packer,repomap,gitutil,format,counter,walker,ignore}
                        \->  internal/mcp  (same packages, different frontend)
```

- `walker` walks directories. It knows about gitignore and binary detection and
  nothing else.
- `counter` estimates tokens. It never reads the filesystem.
- `packer` selects files under a budget. It never renders.
- `repomap` builds tree outlines. It never packs.
- `format` renders bundles. It never walks or counts.
- `gitutil` shells out to git. It never parses `.git`.

Keep these boundaries. The MCP server exists because two frontends call the
same engine — do not let a command grow its own private copy of the logic.

**Estimates are labeled.** Anything approximate should say so in its name,
comment, or output string. Never print a number as exact when it is a
heuristic.

## Tests

`go test ./...`. Each package that has behaviour worth pinning has a test file
beside it. Tests use `t.TempDir()` for fixtures and never touch the network.

Add tests when you:

- add a code path, or
- fix a bug (the test should fail before the fix).

## Reporting security issues

Please do not open a public issue for a vulnerability. See
[SECURITY.md](SECURITY.md).

## Conduct

Be excellent to each other. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License

By contributing you agree that your contributions are licensed under the MIT
License.
