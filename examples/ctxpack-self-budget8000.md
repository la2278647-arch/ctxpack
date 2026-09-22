<!-- fit: FITS model=gpt-4o used=7.5k/123.9k (6%) FITS -->
# Repository: D:\垃圾\ctxpack

- Files: 9  | Tokens: ~7529  | Bytes: 17.7 KB  | Skipped: 0

---

## `CODE_OF_CONDUCT.md` (1404 tokens, 3.6 KB)

```markdown
# Code of Conduct

## Our pledge

We commit to making participation in this project a harassment-free experience
for everyone, regardless of age, body size, disability, ethnicity, sex
characteristics, gender identity and expression, level of experience,
education, socio-economic status, nationality, personal appearance, race,
religion, or sexual identity and orientation.

## Our standards

Behaviour that contributes to a positive, respectful environment:

- Using welcoming and inclusive language.
- Being respectful of differing viewpoints and experiences.
- Gracefully accepting constructive criticism.
- Focusing on what is best for the community.
- Showing empathy towards other community members.

Behaviour that will not be tolerated:

- The use of sexualized language or imagery, and unwelcome sexual attention or
  advances of any kind.
- Trolling, insulting or derogatory comments, and personal or political
  attacks.
- Public or private harassment.
- Publishing others' private information without explicit permission — this is
  especially relevant to this project, since its purpose is to handle
  repository contents that are frequently private.
- Other conduct that could reasonably be described as inappropriate in a
  professional setting.

## Scope

This Code of Conduct applies in all project spaces, in GitHub issues, in
discussions, on social media when you are representing the project, and in
person at events. It also applies to a person's official project roles, such
as maintainer, contributor, or reviewer.

## Enforcement responsibilities

Project maintainers are responsible for clarifying the standards of acceptable
behaviour and are expected to take appropriate and fair corrective action in
response to any instances of unacceptable behaviour.

Maintainers have the right and responsibility to remove, edit, or reject
comments, commits, code, wiki edits, issues, and other contributions that are
not aligned to this Code of Conduct, and to ban temporarily or permanently any
contributor for other behaviours that they deem inappropriate, threatening,
offensive, or harmful.

## Enforcement guidelines

Maintainers will respond to reports of misconduct as follows:

**Correction.** If the behaviour is a first or isolated incident, maintainers
may issue a private written warning, clearly outlining the nature of the
violation and explaining why the behaviour is inappropriate. A private warning
does not become public without the reporter's consent.

**Warning.** For a pattern of behaviour or a serious first incident,
maintainers will issue a public warning with an expectation of behavioural
change. The project reserves the right to revoke previously published content
that is determined to violate this code.

**Temporary ban.** For serious or repeated behaviour, maintainers may exclude
a contributor from project spaces for a defined period.

**Permanent ban.** For severe or repeated violations, maintainers may permanently
exclude a contributor.

## Reporting

Report unacceptable behaviour to the project maintainers by opening a
private security advisory titled "Conduct report" — this keeps details
confidential. Include what happened, when, where, and any evidence you have.

Reports will be treated confidentially and reviewed within two business days.

## Enforcement

Every report is reviewed by at least one maintainer. If a report concerns a
maintainer, a second maintainer reviews it.

## Attribution

This Code of Conduct is adapted from the
[Contributor Covenant version 2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/),
with the repository-content confidentiality note added.
```

---

## `CONTRIBUTING.md` (1232 tokens, 2.9 KB)

```markdown
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
```

---

## `LICENSE` (413 tokens, 1.0 KB)

```
MIT License

Copyright (c) 2025 SkillGuard Dev

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

## `SECURITY.md` (1457 tokens, 3.6 KB)

```markdown
# Security Policy

## Supported versions

| Version | Supported |
| ------- | --------- |
| 0.1.x   | ✅        |

Only the latest release receives fixes. This is a single-binary tool with no
server component and no persistent storage, so there is nothing to patch
long-term.

## Reporting a vulnerability

**Do not open a public issue.** Public issues are visible to everyone the
moment they are filed.

Please report vulnerabilities by opening a
[private security advisory](https://docs.github.com/en/code-security/security-advisories/working-with-repository-security-advisories/creating-a-security-advisory)
on this repository, which notifies maintainers without exposing details.

If you prefer email, write to the maintainer contact listed in `go.mod`'s
module owner profile.

Please include:

- A description of the issue and its impact.
- Steps to reproduce, ideally a minimal command.
- Affected versions.
- A suggested fix, if you have one.

## Response targets

| Severity | Acknowledged within | Fixed within |
| -------- | ------------------- | ------------ |
| Critical | 3 business days     | 14 days      |
| High     | 5 business days     | 30 days      |
| Medium   | 10 business days    | 60 days      |
| Low      | best effort         | best effort  |

## What counts as a vulnerability here

ctxpack reads files from a path you give it and writes output to a path you
give it. Reasonable security concerns include:

- **Information disclosure** — leaking files the user did not intend to expose,
  e.g. through the `.gitignore` matcher or the `--hidden` path. This matters:
  the whole point of the tool is sending repository contents to a third-party
  model, so what gets included is a security decision, not a cosmetic one.
- **Injection in rendered output** — crafted file content breaking out of an
  XML CDATA section, a Markdown code fence, or a JSON string, which could
  inject instructions into the model's prompt. Renderer correctness is
  security-relevant.
- **Path traversal / symlink escape** — reading outside the target directory
  through symlinks or `..` components.
- **Excessive resource use** — a crafted tree causing unbounded memory or CPU.
- **Git command injection** — passing unsanitized input to the `git` CLI.

## Not vulnerabilities

- Reading files inside the path you passed to ctxpack. That is the tool.
- Sending repository contents to the model you choose to call. That is your
  architecture decision; ctxpack does not contact any model itself.
- Running ctxpack with `--hidden` on a directory containing `.env` and then
  forwarding the output somewhere. The flag is documented to do exactly that.
- Estimate inaccuracy. Token counts are heuristic estimates, over-reporting
  real tokenization by roughly 1.5x to 2x depending on the text. A caller
  that budgets strictly from these numbers may pack less than it could.
- Anything in a fork.

## Design notes relevant to security

- **Stdlib only, no network code.** ctxpack opens no sockets. It cannot phone
  home and cannot be a vector for a compromised dependency supply chain.
- **Read-only by construction.** No code path writes into the target tree.
- **Hidden files excluded by default.** `.env` and friends are skipped unless
  `--hidden` is passed.
- **VCS metadata always skipped.** `.git`, `.hg` and `.svn` are excluded
  unconditionally, including `.git/config` which commonly holds credentials.
- **Git is invoked with `-C dir` and separate argv elements**, never through a
  shell, so user-supplied refs cannot inject commands.

## Acknowledgements

Thanks to anyone who reports issues privately and responsibly. Credits appear
in the release notes alongside the fix.
```

---

## `docs/release-notes-v0.1.0.md` (1107 tokens, 2.6 KB)

```markdown
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
```

---

## `examples/ctxpack-self.map.txt` (1069 tokens, 1.9 KB)

```
Repository: ctxpack
Files: ~141067 tokens, 317.0 KB

ctxpack/  [141067t, 317.0KB]
  docs/  [10367t, 24.3KB]
    ci.yml  [2390t, 5.6KB]
    promote.md  [3741t, 9.0KB]
    release-notes-v0.1.0.md  [1107t, 2.6KB]
    release-notes-v0.1.1.md  [1203t, 2.7KB]
    release-notes-v0.1.2.md  [1926t, 4.5KB]
  examples/  [0t, 0B]
    ctxpack-self.map.txt  [0t, 0B]
  internal/  [106740t, 235.7KB]
    cli/  [24390t, 53.5KB]
      cli.go  [7355t, 16.8KB]
      cli_more_test.go  [12284t, 26.4KB]
      cli_test.go  [4751t, 10.3KB]
    counter/  [8930t, 20.1KB]
      counter.go  [2737t, 6.3KB]
      counter_test.go  [6193t, 13.8KB]
    format/  [10966t, 24.3KB]
      format.go  [3855t, 8.8KB]
      format_test.go  [7111t, 15.6KB]
    gitutil/  [4847t, 10.5KB]
      gitutil.go  [1578t, 3.5KB]
      gitutil_test.go  [3269t, 7.0KB]
    ignore/  [12181t, 27.2KB]
      ignore.go  [3297t, 7.8KB]
      ignore_more_test.go  [7114t, 15.5KB]
      ignore_test.go  [1770t, 3.9KB]
    mcp/  [14058t, 31.0KB]
      protocol_test.go  [5427t, 11.8KB]
      server.go  [4388t, 9.8KB]
      server_more_test.go  [2307t, 5.1KB]
      server_test.go  [1936t, 4.3KB]
    packer/  [7750t, 17.1KB]
      packer.go  [2907t, 6.6KB]
      packer_test.go  [4843t, 10.5KB]
    repomap/  [7186t, 15.6KB]
      repomap.go  [1830t, 4.1KB]
      repomap_test.go  [5356t, 11.5KB]
    version/  [1761t, 4.0KB]
      version.go  [444t, 1.0KB]
      version_test.go  [1317t, 2.9KB]
    walker/  [14671t, 32.3KB]
      walker.go  [3955t, 9.2KB]
      walker_more_test.go  [9282t, 20.1KB]
      walker_test.go  [1434t, 3.0KB]
  CHANGELOG.md  [11037t, 26.7KB]
  CODE_OF_CONDUCT.md  [1404t, 3.6KB]
  CONTRIBUTING.md  [1232t, 2.9KB]
  LICENSE  [413t, 1.0KB]
  Makefile  [1234t, 2.8KB]
  README.md  [5012t, 11.6KB]
  SECURITY.md  [1457t, 3.6KB]
  go.mod  [23t, 50B]
  install.ps1  [744t, 1.7KB]
  install.sh  [1024t, 2.3KB]
  main.go  [380t, 904B]
```

---

## `go.mod` (23 tokens, 50 B)

```
module github.com/la2278647-arch/ctxpack

go 1.21
```

---

## `internal/version/version.go` (444 tokens, 1.0 KB)

```go
// Package version holds the build-time identity shared by the CLI and the MCP
// server. It lives in its own package so the two do not have to import each
// other just to print the same version string.
package version

import "runtime"

// Module is the import path of the ctxpack module.
const Module = "github.com/la2278647-arch/ctxpack"

// Version is the semantic version of the running binary. Override it at build
// time with:
//
//	go build -ldflags "-X github.com/la2278647-arch/ctxpack/internal/version.Version=v1.2.3"
var Version = "0.1.2"

// BuildCommit is populated by CI when a tag is cut.
var BuildCommit = "dev"

// BuildDate is populated by CI when a tag is cut.
var BuildDate = "unknown"

// Info is a human-readable summary of how the binary was built.
func Info() string {
	return "ctxpack " + Version + " (" + runtime.GOOS + "/" + runtime.GOARCH +
		", " + runtime.Version() + ", commit " + BuildCommit + ", built " + BuildDate + ")"
}

// UserAgent is a short product token for HTTP clients.
func UserAgent() string {
	return "ctxpack/" + Version
}
```

---

## `main.go` (380 tokens, 904 B)

```go
// Package main is the entry point for ctxpack.
//
// ctxpack is a fast, single-binary tool that packs a repository (or any
// directory tree) into an LLM-optimized context bundle. The same binary can
// also run as a Model Context Protocol (MCP) server over stdio, so AI agents
// (Claude Desktop, Cursor, Codex, ...) can call it to ingest a repo on demand.
//
// Build:
//
//	go build -o ctxpack .
//
// Usage:
//
//	ctxpack pack ./myrepo            # pack a repo to stdout (XML by default)
//	ctxpack map ./myrepo             # show a token-aware tree outline
//	ctxpack diff ./myrepo            # pack only files changed vs HEAD
//	ctxpack tokens ./myrepo          # total token estimate
//	ctxpack mcp                      # run as an MCP server on stdio
package main

import (
	"os"

	"github.com/la2278647-arch/ctxpack/internal/cli"
)

func main() {
	code := cli.Run(os.Args[1:])
	os.Exit(code)
}
```

---

## Omitted by budget (33 files, ~134607 tokens)

- `CHANGELOG.md`
- `Makefile`
- `README.md`
- `docs/ci.yml`
- `docs/promote.md`
- `docs/release-notes-v0.1.1.md`
- `docs/release-notes-v0.1.2.md`
- `install.ps1`
- `install.sh`
- `internal/cli/cli.go`
- `internal/cli/cli_more_test.go`
- `internal/cli/cli_test.go`
- `internal/counter/counter.go`
- `internal/counter/counter_test.go`
- `internal/format/format.go`
- `internal/format/format_test.go`
- `internal/gitutil/gitutil.go`
- `internal/gitutil/gitutil_test.go`
- `internal/ignore/ignore.go`
- `internal/ignore/ignore_more_test.go`
- `internal/ignore/ignore_test.go`
- `internal/mcp/protocol_test.go`
- `internal/mcp/server.go`
- `internal/mcp/server_more_test.go`
- `internal/mcp/server_test.go`
- `internal/packer/packer.go`
- `internal/packer/packer_test.go`
- `internal/repomap/repomap.go`
- `internal/repomap/repomap_test.go`
- `internal/version/version_test.go`
- `internal/walker/walker.go`
- `internal/walker/walker_more_test.go`
- `internal/walker/walker_test.go`

