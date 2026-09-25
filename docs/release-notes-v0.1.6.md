# v0.1.6

The release that completes the MCP machine-readable surface and rounds out the
CLI flag set. All five MCP tools now support `format: "json"` output, and the
`diff` command gained a `--list` mode for scripts.

## Added

- **MCP `list_models` `format` parameter.**
  `list_models` now accepts `format` (`text` or `json`, default `text`). With
  `format: "json"` it returns `{"models": [{name, vendor, context_window,
  limit}]}` — the same shape as the CLI's `models --json` output.

- **MCP `repo_map` `format` parameter.**
  `repo_map` now accepts `format` (`text` or `json`, default `text`). With
  `format: "json"` it returns the structured envelope with `root`,
  `total_tokens`, `total_bytes`, and a `tree` of `{name, is_dir, tokens,
  bytes, children}` nodes — the same shape as the CLI's `map --json` output.

- **`--list` flag for `diff`.**
  `ctxpack diff . --list` prints only the changed file paths, one per line,
  without packing them or reading their content. Faster than `--dry-run`
  (which packs to count tokens) and useful for scripting. The MCP `diff_repo`
  tool accepts a `list` parameter for the same behaviour.

- **More parameters for MCP `diff_repo`.**
  `diff_repo` now accepts `no_gitignore`, `hidden`, and `max_depth`, matching
  the CLI's `diff` command.

- **MCP `count_tokens` full parity with the CLI's `tokens` command.**
  Added `model` (filter to one model), `top` (limit to N largest windows),
  `sort` (by name, pct, or window), `no_gitignore`, and `hidden`.

## Test coverage

| package | v0.1.5 | v0.1.6 |
| ------- | ------ | ------ |
| cli     | 96.4%  | 96.6%  |
| counter | 100%   | 100%   |
| format  | 99.1%  | 99.1%  |
| gitutil | 100%   | 100%   |
| ignore  | 98.0%  | 98.0%  |
| mcp     | 96.0%  | 97.4%  |
| packer  | 100%   | 100%   |
| repomap | 100%   | 100%   |
| version | 100%   | 100%   |
| walker  | 94.4%  | 94.4%  |

349 tests, all passing.

## Installation

```sh
# Homebrew
brew tap la2278647-arch/tap https://github.com/la2278647-arch/homebrew-tap
brew install ctxpack

# Go
go install github.com/la2278647-arch/ctxpack@v0.1.6

# Windows PowerShell
irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex

# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash

# Direct download
# https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.6
```

## What's next

- GitHub Actions CI workflow (blocked on `workflow` scope token)
- Docker daemon testing of the multi-stage Dockerfile
- Promotion posts (drafts in `docs/promote.md`)
