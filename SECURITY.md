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
