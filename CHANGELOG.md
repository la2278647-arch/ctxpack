# Changelog

All notable changes to ctxpack are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project aims
to follow [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2025-01-01

First public release.

### Added

- **`ctxpack pack`** — pack a repository into one LLM-ready bundle with an
  optional token budget. Files that do not fit are ranked by context value
  (READMEs and entry points first, tests and generated scaffolding last) and
  listed as omitted rather than dropped silently.
- **`ctxpack map`** — token-aware tree outline with per-file and per-directory
  estimates, so you can see where a budget will go before spending it.
- **`ctxpack diff`** — pack only the files changed against a git ref, for
  change sets instead of whole trees. Deleted files are listed without content.
- **`ctxpack tokens`** — total token estimate with a per-model fit table.
- **`ctxpack models`** — the model registry with context windows.
- **`ctxpack mcp`** — Model Context Protocol server over stdio exposing
  `pack_repo`, `repo_map` and `count_tokens`.
- **Four renderers** — XML (CDATA), Markdown (fence-safe), JSON, and plain
  text, all escaping file bodies in a way that survives arbitrary source.
- **Token estimator** — tiktoken-style pre-tokenisation plus a calibrated
  per-chunk heuristic, targeting ±15% of real `o200k_base` counts. No network
  access, no embedded model data.
- **Gitignore-aware walker** — layered `.gitignore` matching, a built-in
  denylist for VCS/dependency/build directories, binary detection, and
  dotfiles excluded by default.
- **Model registry** — OpenAI, Anthropic, Google, Meta, Mistral, DeepSeek and
  Qwen with exact and prefix matching.

### Design

- **Stdlib only.** Zero third-party modules; `go.sum` is empty.
- **Single static binary**, offline-capable.
- **Shared engine** — CLI and MCP server call the same
  `packer`/`repomap`/`gitutil` packages.
- **Read-only** — no code path writes into the target tree.

## [Unreleased]

### Fixed

- `--version` printed `gogo1.26.5` because `runtime.Version()` already carries
  a `go` prefix.
- Flags placed after a positional argument were silently ignored. Go's `flag`
  package stops parsing at the first non-flag token, so
  `ctxpack pack ./repo --format markdown` produced XML. Positional arguments
  are now moved to the end of the argument slice before parsing, so the
  documented `ctxpack <command> [path] [flags]` usage works in either order.
- Registered the documented `-o` shorthand for `--output` on `pack` and `diff`.

[0.1.0]: https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.0
