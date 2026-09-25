# v0.1.5

The release that completes the MCP surface and rounds out the CLI flag set.
Statement coverage across all non-`main` packages is 96.4% of ~980 statements;
MCP went from 90.6% to 96.0% with the error-path and edge-case tests added in
this round.

## Added

- **MCP `diff_repo` tool.**
  The fifth MCP tool packs only the files changed against a git ref. Takes
  `path`, optional `ref` (default `WORKTREE`), `format` (default `xml`),
  `budget`, `model`, `include`, `exclude`, `max_size`, `no_gitignore`,
  `hidden`, and `max_depth`. Deleted files are listed but their content is
  omitted, matching the CLI's `diff` command.

- **MCP `count_tokens` format parameter.**
  `count_tokens` now accepts `format: "json"` and returns the structured
  envelope with `total_tokens`, `total_bytes`, `reserve_tokens`, and a `fits`
  array — the same shape as the CLI's `tokens --json` output.

- **MCP `pack_repo` model parameter.**
  When `model` is set, the output is prefixed with an HTML comment annotating
  whether the packed bundle fits within that model's context window after the
  reply reserve — the same annotation the CLI's `--model` flag produces.

- **MCP `diff_repo` walker options.**
  `diff_repo` now accepts `include`, `exclude`, `max_size`, `no_gitignore`,
  `hidden`, and `max_depth`, matching the CLI's `diff` command.

- **MCP `count_tokens` full parity.**
  `count_tokens` now accepts `include`, `exclude`, `max_size`, `max_depth`,
  `no_gitignore`, `hidden`, `model`, `top`, and `sort` — matching the CLI's
  `tokens` command. `sort` accepts `name` (default), `pct`, or `window`.

## Changed

- **The model table grew from 23 to 31 entries.**
  Added `gpt-5`, `o4-mini`, `claude-3.7-sonnet`, `claude-4-opus`,
  `gemini-2.5-flash`, `llama-3.3-70b`, `mistral-large-2`, and `qwen-2.5-72b`.
  Seven vendors: OpenAI, Anthropic, Google, Meta, Mistral, DeepSeek, Alibaba.

- **`--sort` for `models`.**
  Sort the model table by `name` (default), `window` (largest context first),
  or `vendor` (group by vendor, then by window). Only applied when explicitly
  provided; without it the registration order is preserved.

- **`--top` for `doctor`.**
  Limit the vendor breakdown to the N largest vendors. The totals line always
  reflects the full count.

## Test coverage

| package     | v0.1.4 | v0.1.5 |
| ----------- | ------ | ------ |
| cli         | 96.4%  | 96.4%  |
| counter     | 100%   | 100%   |
| format      | 99.1%  | 99.1%  |
| gitutil     | 100%   | 100%   |
| ignore      | 98.0%  | 98.0%  |
| mcp         | 93.1%  | 96.0%  |
| packer      | 100%   | 100%   |
| repomap     | 100%   | 100%   |
| version     | 100%   | 100%   |
| walker      | 94.4%  | 94.4%  |

340 tests, all passing.

## Installation

```sh
# Homebrew
brew tap la2278647-arch/tap https://github.com/la2278647-arch/homebrew-tap
brew install ctxpack

# Go
go install github.com/la2278647-arch/ctxpack@v0.1.5

# Windows PowerShell
irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex

# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash

# Docker
docker run --rm -v "$(pwd):/repo:ro" ghcr.io/la2278647-arch/ctxpack pack /repo

# Direct download
# https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.5
```

## What's next

- GitHub Actions CI workflow (blocked on `workflow` scope token)
- Docker daemon testing of the multi-stage Dockerfile
- Promotion posts (drafts in `docs/promote.md`)
- v0.1.2 was never published; deliberately not backfilled
