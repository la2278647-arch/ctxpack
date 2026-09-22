> **Superseded by [v0.1.1](https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.1).**
> The binaries on this release were built at commit `5a2017d`, which is five
> commits after this release's tag, and they predate the fixes for the 1024x
> byte counts, the unparseable XML output, the token under-count in `mcp`'s
> `repo_map`, and the flag-parsing drops. Use v0.1.1.
>
# v0.1.0

First release.

## What it is

`ctxpack` packs a repository into one LLM-ready document that fits a context
window. Single Go binary, standard library only, zero dependencies, works
offline, read-only against the trees it reads.

## Commands

| Command          | What it does                                                    |
| ---------------- | --------------------------------------------------------------- |
| `ctxpack pack`   | One bundled document: XML / Markdown / JSON / text              |
| `ctxpack map`    | Token-aware tree map, cheapest orientation                      |
| `ctxpack diff`   | Pack only the files changed against a git ref                   |
| `ctxpack tokens` | Estimate the token count of files, a directory, or stdin        |
| `ctxpack models` | Registry of model context windows                               |
| `ctxpack mcp`    | Run as a Model Context Protocol server on stdio                 |

## Budget model

Files are ranked by context value — READMEs, licenses and contributing guides
first, then entry points, then interfaces, docs, source, config, tests — and
packed greedily until the budget is spent. The rendered bundle lists what did
not fit and why, so the caller always sees the cut.

## In this release

- `version` printed `gogo1.26.5`; `runtime.Version` already carries the `go`
  prefix.
- Flags after a positional were silently ignored — Go's `flag` package stops
  parsing at the first non-flag token, so the documented
  `ctxpack pack ./repo --format markdown` returned XML. Positionals now move to
  the end of the argument list before parsing.
- Registered the documented `-o` shorthand for `--output`.
- MCP parse errors were swallowed; JSON-RPC requires a
  `{"id": null, "error": ...}` response even when the message could not be
  decoded.
- Tests for argument reordering, format parsing, the MCP protocol round trip,
  tool calls, and parse-error framing.

## Install

```sh
go install github.com/la2278647-arch/ctxpack@v0.1.0
```

Or take a binary from this release. Binaries are built with `-trimpath` and
`CGO_ENABLED=0`, so they are statically linked and portable.

## Binaries

`ctxpack_0.1.0_<os>_<arch>` — windows (amd64, 386, arm64), linux
(amd64, 386, arm64), darwin (amd64, arm64).
