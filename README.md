# ctxpack

**Pack a repository into one LLM-ready document that fits the context window.**

```
ctxpack pack ./myrepo --model gpt-4o --format markdown
```

That is the whole pitch. ctxpack walks a repository, estimates the token cost
of every file, selects the subset that fits your budget, and emits one
well-structured document you can paste into a chat — or hand to an agent.

It is a single statically-linked Go binary with **zero third-party
dependencies**. It runs anywhere Go runs, works fully offline, and doubles as
an [MCP](https://modelcontextprotocol.io) server so AI agents can ingest repos
on demand.

---

## Why this exists

Three things go wrong when you feed source code to a large model:

1. **The context wall.** You paste your repo, hit "send", and something gets
   truncated. Usually the most important half.
2. **Silent money burn.** You do not know whether you paid for 40k tokens or
   400k. Nobody tells you.
3. **The pack is fragile.** Copied code loses its structure. Markdown fences
   break on content that contains backticks. XML breaks on `]]>`.

ctxpack makes all three visible and controllable:

- A **token budget** is a first-class flag. When it is exceeded, files are
  dropped by priority and the dropped set is *listed*, so you see exactly what
  you gave up.
- Every command reports token counts, so "does this fit?" is a number and not
  a guess.
- Renderers wrap file bodies in forms that survive arbitrary source: XML uses
  CDATA, Markdown uses a fence-length scanner, JSON is escaped by the encoder.

---

## Install

**One-line binary install (no Go required):**

```sh
# Linux / macOS / WSL / git-bash
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash
# pin a version:
curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash -s -- v0.1.5
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex
```

The installers detect your OS/architecture, download the matching binary
from the [latest release](https://github.com/la2278647-arch/ctxpack/releases),
and put it on your `PATH`. Set `CTXPACK_INSTALL_DIR` to override the target.

Both download to a scratch directory and move into place only after verifying
the binary against the release's `SHA256SUMS.txt`. A mismatch, a missing
entry, or a failed download aborts without leaving a binary on your `PATH`.
They prefer the GitHub REST API to find the latest release and fall back to
the unrate-limited `/releases/latest` redirect if the API 403s.

**Or via a package manager:**

```sh
# Homebrew (macOS / Linux) — builds from source via the tap
brew tap la2278647-arch/tap https://github.com/la2278647-arch/homebrew-tap
brew install ctxpack
```

```powershell
# Scoop (Windows) — installs the prebuilt binary
scoop bucket add la2278647-arch https://github.com/la2278647-arch/scoop-bucket
scoop install ctxpack
```

**Or via Docker:**

```sh
# build and run from source
docker build -t ctxpack .
docker run --rm -v "$(pwd):/repo:ro" ctxpack map /repo
docker run --rm -v "$(pwd):/repo:ro" ctxpack pack /repo --budget 8000 --format markdown

# build with a version stamp
docker build -t ctxpack --build-arg VERSION=0.1.4 --build-arg COMMIT=$(git rev-parse --short HEAD) .
```

The image is a multi-stage build: a static binary compiled with `CGO_ENABLED=0`
on `golang:1.21-alpine`, copied into `alpine:latest`. The entrypoint is the
binary itself, so any arguments become `ctxpack` arguments. Mount your tree
read-only (`:ro`) — `ctxpack` never writes to the tree it reads.

**Or build from source:**

```sh
# from source
git clone https://github.com/la2278647-arch/ctxpack
cd ctxpack && go build -o ctxpack .

# or as a module
go install github.com/la2278647-arch/ctxpack@latest
```

Requires Go 1.21+. No modules are fetched; `go.sum` is empty by design.

```sh
ctxpack version
# ctxpack 0.1.4 (linux/amd64, go1.26.5, commit abc1234, built 2026-09-22)
```

Pin build identity at release time:

```sh
go build -ldflags "\
  -X github.com/la2278647-arch/ctxpack/internal/version.Version=v0.1.5 \
  -X github.com/la2278647-arch/ctxpack/internal/version.BuildCommit=$(git rev-parse --short HEAD) \
  -X github.com/la2278647-arch/ctxpack/internal/version.BuildDate=$(date -u +%Y-%m-%d)" \
  -o ctxpack .
```

---

## Examples

See [examples/](examples/) for **ctxpack run on itself**: a token-aware tree
map, and a budget-capped self-pack that shows exactly which files are kept and
which are omitted — and why.

For the tool against a codebase that is not ctxpack, see
[ctxpack-demo](https://github.com/la2278647-arch/ctxpack-demo): a small FastAPI
service carrying ctxpack's own output captured on it. Both are described in
[docs/examples.md](docs/examples.md).

---

## Quick start

```sh
# What does this repo cost?
ctxpack tokens ./myrepo

# Where is it going? Token-aware tree outline.
ctxpack map ./myrepo

# Pack it for GPT-4o, as Markdown, to a file.
ctxpack pack ./myrepo --model gpt-4o --format markdown -o repo.md

# Only what you changed since main.
ctxpack diff ./myrepo --ref main

# Squeeze into a small window.
ctxpack pack ./myrepo --budget 20000
```

### `tokens` — what is this repo worth?

```
Path:       /home/me/myrepo
Tokens:     ~31,842
Bytes:      142.3 KB

Per-model fit (est. tokens / context window):
  [FITS]     gpt-4o               31.8k / 123.9k (26%)
  [FITS]     claude-3.5-sonnet    31.8k / 195.9k (16%)
  [OVERFLOW] gpt-3.5-turbo        31.8k / 12.3k (258%)
```

`--model gpt-4o` shows just that model's line:

```
  [FITS] gpt-4o  31.8k / 123.9k (26%)
```

### `map` — where is the budget going?

```
Repository: /home/me/myrepo
Files: ~31842 tokens, 142.3 KB

myrepo/  (48 files, 31842 tokens, 142.3KB)
├── internal/  (30 files, 24110 tokens, 108.1KB)
│   ├── parser/  (6 files, 8804 tokens, 40.2KB)
│   │   └── parser.go  [8120 tokens, 37.4KB]
│   └── ...
├── README.md  [1204 tokens, 5.6KB]
└── go.mod  [12 tokens, 312B]
```

### `map --json`, `tokens --json`, `models --json`

All three take `--json` for scripts. `pack` has `--format json` for its bundle;
these have no format family, so a boolean flag is enough.

```json
{
  "root": "myrepo",
  "total_tokens": 31842,
  "total_bytes": 145728,
  "tree": {
    "name": "myrepo",
    "is_dir": true,
    "tokens": 31842,
    "bytes": 145728,
    "children": [
      { "name": "README.md", "is_dir": false, "tokens": 1204, "bytes": 5632, "children": [] }
    ]
  }
}
```

```json
{
  "path": "myrepo",
  "total_tokens": 31842,
  "total_bytes": 145728,
  "reserve_tokens": 4096,
  "fits": [
    { "model": "gpt-4o", "used": 31842, "limit": 123904, "fits": true, "pct_used": 25.7 }
  ]
}
```

```json
{
  "models": [
    { "name": "gpt-4o", "vendor": "openai", "context_window": 128000, "limit": 123904 }
  ]
}
```

Three invariants worth relying on:

- `tree.children` is always an array, never `null`. A file and an empty
  directory both get `[]`; `is_dir` is what separates them.
- The header totals equal the tree totals, and both agree with the text output.
  Every format reads one shared walk, so the estimates cannot drift. The text
  output rounds the same `pct_used` to the nearest integer (`25.7` above shows
  as `26%`).
- `limit` is always `context_window` minus the same 4096-token reply reserve, so
  `models --json` and `tokens --json` join without recomputing it.

### `pack` — the bundle

```xml
<?xml version="1.0" encoding="UTF-8"?>
<repository root="/home/me/myrepo" files="17" skipped="0" tokens="31842">
  <file path="README.md" lang="markdown" bytes="5632" tokens="1204">
<![CDATA[
# My project
...
]]>
  </file>
</repository>
```

### `diff` — the delta, not the tree

Packs only files changed against a git ref, so a reviewer (human or model)
sees the change set. Deleted files are listed in the header but their content
is omitted — it is gone from the working tree. Use `--list` to show only the
changed file paths without packing.

---

## Commands

| Command  | Purpose |
| -------- | ------- |
| `pack`   | Pack a repository into one context bundle. |
| `map`    | Token-aware tree outline. |
| `diff`   | Pack only the files changed in git. |
| `tokens` | Total token estimate + per-model fit. |
| `models` | List known models and context windows. |
| `mcp`    | Run as an MCP server on stdio. |
| `doctor` | Print environment diagnostics (version, git, models). |

Run `ctxpack <command> --help` for details.

### Shared flags

| Flag | Meaning |
| ---- | ------- |
| `--format F` | `pack`/`diff`: `xml` (default), `markdown`, `json`, `text`. `map`: `text` (default), `json`, `csv`. `tokens`/`doctor`/`models`: `text` (default), `json`. |
| `--include GLOB` | Restrict to matching paths. Repeatable; basename globs like `*.go` work. |
| `--exclude GLOB` | Drop matching paths. Repeatable. |
| `--max-size N` | Read no more than N bytes of a file. Larger files are still listed — without content — so the pack shows the name but not the body. `0` = unlimited. |
| `--no-gitignore` | Ignore `.gitignore` (the built-in denylist still applies). |
| `--hidden` | Include dotfiles. `.git`/`.hg`/`.svn` are always skipped. |
| `--budget N` | Cap output to ~N tokens; selects by priority. |
| `--depth N` | Limit traversal to N levels below root. `0` = unlimited (default). |
| `--top N` | `map`: show a flat list of the N largest files (by tokens or bytes). `tokens`/`models`: show only the N largest models by context window. `doctor`: show only the top N vendors by model count. |
| `--csv` | `map`: output a flat CSV list of all files (path, tokens, bytes). |
| `--model NAME` | `pack`: annotate fit for a named model. `tokens`: show fit for one model instead of all 31. |
| `--sort BY` | `map`: sort children by `name` (default, dirs first), `tokens` (largest first), or `bytes` (largest first). `tokens`: sort fit table by `name` (default), `pct`, or `window`. `models`: sort by `name` (default), `window` (largest first), or `vendor`. |
| `--vendor NAME` | `models`: show only models from this vendor (case-insensitive). |
| `-o, --output FILE` | Write to FILE instead of stdout (all commands). |
| `--dry-run` | `pack`/`diff`: show what would be packed without writing output. |
| `--list` | `diff`: list changed file paths only (no packing, no token counts). |
| `-q, --quiet` | Suppress stderr status messages (`pack`: the "wrote" line; `diff`: the JSON status). |

Environment: `CTXPACK_MODEL`, `CTXPACK_FORMAT`, `CTXPACK_BUDGET`.

`diff` additionally takes `--ref REF` (default: working tree, uncommitted
changes included).

---

## How the budget works

Passing `--model gpt-4o` derives the ceiling as
`context_window − reserve` and packs files until it is spent. Files that do not
fit are **listed as omitted**, never silently dropped — XML carries an
`<omitted count="…" tokens="…">` element, Markdown appends an
"Omitted by budget" section, and JSON adds `omitted` and `omitted_tokens`. The
omitted element is left out entirely when nothing was cut, so an unlimited pack
keeps its shape. Adding the packed total to the omitted total gives back every
token the walk saw.

Files are ranked by *context value*, not by size:

| Score | Tier | Examples |
| ----- | ---- | -------- |
| 1000 | Orientation | `README*`, `LICENSE`, `CONTRIBUTING*`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `PATENTS` |
| 400 | Entry points | `main.go`, `index.ts`, `index.js`, `mod.go`, `server.js`, `manage.py` |
| 350 | Interfaces | `*.proto`, `*.graphql`, `*.thrift` |
| 300 | Documentation | `*.md`, `*.rst`, `*.txt` |
| 200 | Source | `*.go`, `*.rs`, `*.py`, `*.ts`, `*.java`, `*.rb`, `*.cs` |
| 150 | Configuration | `*.yaml`, `*.yml`, `*.toml`, `*.json`, `Dockerfile*`, `Makefile*` |
| 100 | Scripts | `*.sh` |
| 50 | Tests | `*_test.go`, `*.test.js`, `*.spec.ts`, `*.test.py`, `test_*.py` |
| 10 | Everything else | |

Order matters as much as the numbers: a `_test.go` file is also a `.go` file,
so the test tier is matched before source, or every test would score as code
and nothing would be left to rank last. Within one tier, ties break by
ascending token count, so a budget buys the most files it can.

Only whole files are dropped by the budget; nothing is truncated. `diff`
prunes files before the budget is applied, so a file excluded there is not
reported as omitted.

### Token estimation

Estimates use a tiktoken-style pre-tokenisation regex plus a calibrated
per-chunk heuristic. Each pre-token costs at least one token; longer chunks
cost roughly 3.5 bytes per token, blended between prose (≈4 bytes/token) and
symbol-heavy code (≈2.7). The result is floored at bytes/4, although with the
current formula that floor never actually binds — the per-chunk sum is always
at least bytes/3.5.

Use it for sizing, not for accounting. Against `o200k_base` it over-reports:
ASCII prose comes out around bytes/3 where GPT-4o sits nearer bytes/4, and
chunk-dense code can be double the bytes/4 baseline. `EstimateSize`, used when
a file was too large to read or `ReadContent` is off, applies bytes/4
directly, so a repo map built without content reads lower than a pack of the
same files — by up to a factor of two for code. Both numbers are estimates,
and the error does not have a fixed sign. The estimator sits behind an
interface, so a real BPE tokenizer can be dropped in later.

---

## MCP server

The same binary serves as an MCP server over stdio, so agents can ingest
repositories on demand instead of you pasting files.

```json
{
  "mcpServers": {
    "ctxpack": {
      "command": "ctxpack",
      "args": ["mcp"]
    }
  }
}
```

Claude Desktop and Cursor read this from their config; Codex and any other MCP
client work the same way. Exposed tools:

| Tool | Arguments |
| ---- | --------- |
| `pack_repo` | `path`, `format?`, `include?`, `exclude?`, `max_size?`, `no_gitignore?`, `hidden?`, `budget?`, `model?` |
| `repo_map` | `path`, `format?`, `include?`, `exclude?`, `max_size?`, `max_depth?`, `sort?` |
| `count_tokens` | `path`, `format?`, `include?`, `exclude?`, `max_size?`, `max_depth?`, `no_gitignore?`, `hidden?`, `model?`, `top?`, `sort?` |
| `list_models` | none — the whole table, so a client can pick a valid model name before calling `count_tokens` |
| `diff_repo` | `path`, `ref?`, `format?`, `budget?`, `model?`, `include?`, `exclude?`, `max_size?`, `no_gitignore?`, `hidden?`, `max_depth?`, `list?` |

Conversation:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"count_tokens","arguments":{"path":"/home/me/myrepo"}}}
```

The server speaks JSON-RPC 2.0 with newline-delimited messages and protocol
version `2024-11-05`. It logs nothing to stdout — stdout is the protocol
channel — and reports tool failures as `isError: true` rather than throwing.

Every `limit` it reports is `context_window` minus the same 4096-token reply
reserve the CLI uses, held in one constant (`counter.ReplyReserve`). A test
checks that `list_models` and `count_tokens` agree on every model's limit, so
the two cannot drift apart.

---

## Design notes

- **Stdlib only.** No `cobra`, no `viper`, no tokenizer libraries, no JSON
  helpers. The `flag` package is enough for six commands; the JSON encoder is in
  the standard library. One `go build`, zero network access, one file to ship.
- **Frontends share an engine.** The CLI and the MCP server call the same
  `packer`/`repomap`/`gitutil` packages, so the two surfaces cannot drift.
- **Read-only.** ctxpack never writes to a repository. Output goes to stdout or
  to a path you name.
- **Git is optional.** Only `diff` requires git, and it shells out to the `git`
  CLI rather than parsing `.git`. Every other command works on a bare
  directory.
- **Hidden files are skipped by default.** A tool that ingests repos will
  otherwise leak `.env`. Opt in with `--hidden`.

---

## Limitations

- Token counts are estimates. Budgeting is sizing, not accounting.
- Binary files are listed with sizes but their content is never read, so a
  bundle is text-only.
- `--budget` drops whole files; there is no partial-file truncation.
- Git ranges are two revisions only (`--ref main`), not arbitrary `A..B`.
- The MCP server is single-client and synchronous.

## Development

```sh
make check
```

From a source checkout. That runs gofmt cleanliness, `go vet`, the test
suite, and the stdlib-only assertion — `go.sum` must stay empty and
`go list -m all` must report exactly one module. `make smoke` additionally
builds the binary and drives every command against the source tree itself.

Without make, the same four checks are:

```sh
test -z "$(gofmt -l internal/ ./*.go)"
go vet  ./...
go test ./...
test ! -s go.sum
```

The same gate runs as GitHub Actions. The workflow definition is in
[docs/ci.yml](docs/ci.yml) rather than under `.github/workflows/` because the
token that published this repository does not carry the `workflow` scope
GitHub requires to create workflow files. Activate it with:

```sh
git mv docs/ci.yml .github/workflows/ci.yml
git push
```

## License

MIT — see [LICENSE](LICENSE).
