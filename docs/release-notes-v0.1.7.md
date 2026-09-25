# v0.1.7

The release that teaches `diff` about history. `diff` now understands git
ranges and reports files that were deleted, and `pack` stops printing your
machine's absolute path into the bundle.

## Added

- **`diff` ranges (`--ref A..B`).**
  `ctxpack diff --ref HEAD~5..HEAD` compares two revisions as history. A range
  never includes the working tree's untracked files, because those belong to
  neither side of the range. A plain ref (`--ref main`) still diffs against the
  working tree, so uncommitted changes remain included. The MCP `diff_repo`
  tool accepts the same range syntax for its `ref` argument.

- **`diff` reports deleted files.**
  A deletion has no content to pack, so before this release the bundle simply
  never mentioned it — a removed file looked like it had never existed. `diff`
  now lists removed paths in a `Deleted` section: XML adds a `<deleted>`
  element, Markdown a `## Deleted` heading, text a `==== deleted ====` block,
  and JSON a `deleted` array. `--dry-run` prints the count and each path. The
  MCP `diff_repo` tool reports them the same way. Paths are reported by name
  only, never content.

- **`examples/diff-demo`: a reproducible diff snapshot.**
  A generated `ctxpack diff` output over a two-commit range containing a
  modification, an addition and a deletion — the smallest example that shows
  range syntax and the new `Deleted` section together. `make.sh` builds the
  scratch repository in a temp directory and deletes it on exit, so the
  committed `diff.xml` is byte-identical on any machine.

## Changed

- **Bundles name the directory, not the machine.**
  `pack` and `diff` now label the bundle with the walked directory's base name
  (`<root>ctxpack</root>`) instead of its absolute path
  (`<root>D:\Users\alice\ctxpack</root>`). The machine the bundle was built on
  is not information a model needs, and an absolute path leaks the username of
  whoever packed the repository. This matches what `map` already did for its
  root node; `pack` and `diff` had diverged from it. The JSON `root` field
  carries the same shortened value.

  If you parsed the JSON `root` field and expected an absolute path, read it
  from the file system instead — the bundle now names the directory.

- **`gitutil.DiffFiles` splits deletions from changes.**
  `ChangedFiles` keeps its signature and behaviour, now returning only the
  packable set; callers that need to report removals use the new `DiffFiles`,
  which returns both lists.

## Test coverage

| package | v0.1.6 | v0.1.7 |
| ------- | ------ | ------ |
| cli     | 96.6%  | 96.6%  |
| counter | 100%   | 100%   |
| format  | 99.1%  | 99.3%  |
| gitutil | 100%   | 100%   |
| ignore  | 98.0%  | 98.0%  |
| mcp     | 97.4%  | 97.4%  |
| packer  | 100%   | 100%   |
| repomap | 100%   | 100%   |
| version | 100%   | 100%   |
| walker  | 94.4%  | 94.4%  |

369 tests, all passing.

## Installation

```sh
# Homebrew
brew tap la2278647-arch/tap https://github.com/la2278647-arch/homebrew-tap
brew install ctxpack

# Go
go install github.com/la2278647-arch/ctxpack@v0.1.7

# Windows PowerShell
irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex

# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash

# Direct download
# https://github.com/la2278647-arch/ctxpack/releases/tag/v0.1.7
```

## What's next

- GitHub Actions CI workflow (blocked on `workflow` scope token)
- Docker daemon testing of the multi-stage Dockerfile
- Model table expansion (blocked: context windows cannot be verified)
- Promotion posts (drafts in `docs/promote.md`)
