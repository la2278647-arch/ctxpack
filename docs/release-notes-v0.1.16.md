# v0.1.16

The MCP parity release: the server's analytic tools now walk the tree the
same way the CLI does, and the arguments they advertise actually take effect.

## Fixed

- **MCP `repo_map` honours `no_gitignore` and `hidden`, and reads contents
  like the CLI's `map`.**
  The tool declared the two flags in its schema but hard-coded
  `RespectGitignore: true` and omitted `IncludeHidden`, so neither argument
  had any effect — a caller asking for `no_gitignore: true` still got the
  .gitignore-filtered tree. It also ran with `ReadContent: false`, reporting
  a different total than the CLI for the same tree. The walker now honours
  both flags and reads contents (with `max_size` still capping the read), so
  `repo_map` agrees with `map` and with the other packing tools.

- **MCP `count_tokens` reads contents, matching the CLI's `tokens`.**
  The tool's walker ran with `ReadContent: false`, so it reported the
  bytes/4 estimate while the CLI's `tokens` read the files — the same tree
  produced two different totals depending on which interface you asked.
  `count_tokens` now reads like the CLI (and `max_size` still caps the
  reading, so the estimate path remains reachable), and the two interfaces
  agree on the total.

Also in this release: the walk-flag integration series landed — every
`--include`/`--exclude`/`--max-size`/`--depth`/`--hidden`/gitignore behavior
now has a test at both the CLI and MCP layers, plus the mcp argument
behaviors (budget, max_size, max_depth, include, exclude) that pin those
contracts.