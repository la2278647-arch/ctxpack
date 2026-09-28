# v0.1.15

The empty-diff release. Both frontends now treat "nothing changed" as a
normal result instead of an edge that broke a JSON consumer or looked like a
tool error.

## Fixed

- **`diff --format json` now emits an envelope even when nothing changed.**
  An empty diff short-circuited before the format branch, so json mode wrote
  nothing to stdout — only a stderr note — and a script that json-parses
  `ctxpack diff` output would fail on the one repo where there was nothing to
  say. The empty diff now renders the empty envelope (`files: null`, zero
  tokens) exactly like a bundle whose files were all filtered out; the stderr
  note stays and respects `--quiet` like the packed json path.

- **MCP `diff_repo` reports an empty diff as a success, not an error.**
  The MCP tool still returned `isError: true` with a "no changed files" note,
  so an agent treating a tool error as a failure would see one on the most
  routine outcome. An empty diff is now a successful result: `format: json`
  yields the parseable empty envelope identical to the CLI's, and the other
  formats get the note as content. json consumers always get valid JSON.

Also in this release: a long tail of integration tests and fixes landed
since v0.1.14 — empty-directory envelopes for both frontends, the `--top 0`
semantics across all four top-taking commands, `--include`/`--exclude` path
globs and their pack/tokens integration, `--max-size`'s estimate behavior on
`map`, the window/limit/reserve invariants on both json envelopes, and a
gofmt sweep that left the whole tree clean.