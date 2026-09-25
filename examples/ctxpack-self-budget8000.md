<!-- fit: FITS model=gpt-4o used=7.9k/123.9k (6%) FITS -->
# Repository: ctxpack

- Files: 10  | Tokens: ~7924  | Bytes: 19.2 KB  | Skipped: 0

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

## `docs/examples.md` (904 tokens, 2.1 KB)

```markdown
# Examples

Two ways to see ctxpack in action.

## [ctxpack-demo](https://github.com/la2278647-arch/ctxpack-demo)

A real project, not ctxpack itself. It is a FastAPI microservice
(hello-service v0.3.1): one table, five endpoints, cursor pagination,
pydantic validation, a test per behaviour, and a 58 KB synthetic catalog that
makes the token budget bite.

The demo's README embeds ctxpack's own output captured on that project, which
is a more useful illustration than any hand-written sample:

- `ctxpack tokens .` reports 39212 estimated tokens and marks two small-window
  models `[OVERFLOW]` — `gpt-3.5-turbo` at 319% of its window, `gpt-4` at 957%.
- `ctxpack map .` shows `data/seed.json` at 26178 tokens, 66.8% of the whole
  tree, so the ranking problem is visible at a glance.
- `ctxpack pack . --budget 5000` fits `gpt-4o` at 4% and keeps 13 files at
  ~4973 tokens, then lists the twelve files it cut with their individual token
  counts.

That last list is the feature. A budget that truncates mid-file hides what it
lost; a budget that names what it cut lets you decide.

Clone it and try your own numbers:

```sh
git clone https://github.com/la2278647-arch/ctxpack-demo
cd ctxpack-demo
ctxpack tokens .
ctxpack map .
ctxpack pack . --model gpt-4o --format markdown --budget 5000
```

Try `--budget 2000` and watch the omitted list grow. Try `--max-size 10000`
and see the 58 KB catalog stop being read.

## [`examples/`](../examples/)

ctxpack run on **itself**: a token-aware tree of this repository, and a pack
of this repository capped at 8000 tokens. Useful as a self-contained artifact
because it has no dependency on a second checkout.

The two approaches are complementary. The self-snapshots are stable and
shippable; the demo project shows the tool against a codebase with the shape
of something a reader would actually point it at.

## Regenerating

```sh
ctxpack map . > examples/ctxpack-self.map.txt
ctxpack pack . --format markdown --budget 8000 --model gpt-4o \
  -o examples/ctxpack-self-budget8000.md
```

Both snapshots reflect the tree at the commit they were generated. The
estimates move as the tree changes, so regenerate before tagging a release.
```

---

## `examples/README.md` (665 tokens, 1.6 KB)

```markdown
# Examples

This directory holds **`ctxpack` run on itself** — a show-don't-tell snapshot
of what the tool produces. Every snapshot here is a generated artifact;
regenerate them with the commands documented under each file.

## `ctxpack-self.map.txt`

A token-aware tree outline of the ctxpack repository. Every directory and file
is annotated with a token estimate and byte size, so you can see at a glance
where the context budget goes.

```sh
ctxpack map . > examples/ctxpack-self.map.txt
```

## `ctxpack-self-budget8000.md`

`ctxpack` packing **itself** into a single Markdown bundle, capped to an 8000
token budget and annotated for `gpt-4o`. This is the interesting demo: it
keeps the 10 highest-priority files (~7.9k tokens) and **lists the 44 files it
omitted and why** — the "what got cut" report that is the whole point of a
budget.

```sh
ctxpack pack . --format markdown --budget 8000 --model gpt-4o \
  -o examples/ctxpack-self-budget8000.md
```

## `diff-demo/`

A generated `ctxpack diff` output over a two-commit range that contains a
deletion — the smallest example that shows range syntax, packed changes and the
`Deleted` section together. See [`diff-demo/README.md`](diff-demo/README.md).

```sh
cd examples/diff-demo && ./make.sh /path/to/ctxpack
```

## Notes

- Token counts are estimates (a tiktoken-style pre-tokenization + calibrated
  heuristic), not real-BPE output. See the main README's "Limitations" section.
- These snapshots reflect the tree at the commit they were generated; the
  numbers move as the repo changes. Regenerate before tagging a release.
```

---

## `examples/diff-demo/README.md` (1002 tokens, 2.4 KB)

```markdown
# diff demo — a range with a deletion

`diff.xml` shows `ctxpack diff` run over a range of two commits, where that
range contains **one modification, one addition and one deletion**. It is the
smallest output that exercises three features at once:

| Feature | Where it appears |
| ------- | ---------------- |
| Range syntax | the header comment, `diff vs "HEAD~1..HEAD"` |
| Modified + added files | `<files>`, `src/app.go` and `src/util.go` |
| Deleted files | the `<deleted>` element, `src/legacy.go` |

A deletion has no content to pack — the file is gone. But a diff that silently
omitted it would make the removal look like it never happened, so the bundle
names the path in a `Deleted` section and leaves it out of `<files>`. The same
section appears in every format: `<deleted>` in XML, `## Deleted` in Markdown,
`==== deleted (N files) ====` in text, and a `deleted` array in JSON.

## Reproduce

```sh
cd examples/diff-demo
./make.sh /path/to/ctxpack      # rewrites diff.xml
```

`make.sh` builds a throwaway git repository in a temp directory, commits two
revisions apart, and runs `ctxpack diff` over the range. The scratch repo is
deleted when the script exits, so nothing it creates is checked in and the
output is byte-identical on every machine.

The demo repository's own directory name is `diffdemo`, which is why the bundle
reads `<root>diffdemo</root>` — a bundle is labelled with the walked directory's
base name, never the absolute path of the machine that produced it.

## The range is history, not the working tree

`HEAD~1..HEAD` compares two revisions. Because neither side of a range is the
working tree, the diff never picks up uncommitted or untracked files — they
exist in no revision. Pass a plain ref instead (`--ref HEAD~1`, or nothing for
the default `WORKTREE`) to include them.

## `--list` deliberately hides deletions

```sh
ctxpack diff <repo> --list --ref HEAD~1..HEAD
# src/app.go
# src/util.go
```

`--list` prints the packable paths and nothing else, so a shell loop such as
`ctxpack diff . --list | while read f; do ...; do` keeps receiving one real,
existing path per line. Deletions are reported by the packed bundle and by
`--dry-run` (which prints `... N files deleted` followed by each path prefixed
with `D `), not by `--list`.

## Files

- `make.sh` — regenerates `diff.xml`; idempotent and self-cleaning.
- `diff.xml` — the generated output.
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
var Version = "0.1.8"

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

## Omitted by budget (48 files, ~246180 tokens)

- `CHANGELOG.md`
- `Dockerfile`
- `Makefile`
- `README.md`
- `docs/ci.yml`
- `docs/promote.md`
- `docs/release-notes-v0.1.0.md`
- `docs/release-notes-v0.1.1.md`
- `docs/release-notes-v0.1.2.md`
- `docs/release-notes-v0.1.3.md`
- `docs/release-notes-v0.1.4.md`
- `docs/release-notes-v0.1.5.md`
- `docs/release-notes-v0.1.6.md`
- `docs/release-notes-v0.1.7.md`
- `docs/release-notes-v0.1.8.md`
- `examples/ctxpack-self-budget8000.md`
- `examples/ctxpack-self.map.txt`
- `examples/diff-demo/diff.xml`
- `examples/diff-demo/make.sh`
- `install.ps1`
- `install.sh`
- `internal/cli/cli.go`
- `internal/cli/cli_more_test.go`
- `internal/cli/cli_test.go`
- `internal/counter/counter.go`
- `internal/counter/counter_test.go`
- `internal/doctor/doctor.go`
- `internal/doctor/doctor_test.go`
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
- `scripts/smoke.sh`

