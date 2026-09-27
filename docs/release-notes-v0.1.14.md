# v0.1.14

The parity release. The CLI's `tokens --json` fit envelope and the MCP
`count_tokens` envelope are now identical field for field —
`model`/`vendor`/`window`/`used`/`limit`/`fits`/`pct_used` — so a script
consuming both interfaces needs exactly one parser, and the field naming no
longer betrays whether the bytes came from the CLI or the server. Plus a
smoke-suite guard so the installers' `--help` branch can never rot into a
version install.

## Added

- **`tokens --json` fits now carry `vendor` and `window`.**
  The MCP `count_tokens` envelope has included both fields; the CLI's
  `tokens --json` did not, so a script reading both interfaces had to join
  the model table by name. Each fit now carries `"vendor"` and `"window"`
  (the raw context window, alongside `limit`, which is the window minus the
  reply reserve). The csv output is unchanged — it stays a compact five
  columns.

- **MCP `count_tokens` fits rename `name` to `model`.**
  The CLI's fit envelope used `model`; the MCP envelope used `name` for the
  same field. The MCP fits field is now `model`, and the two envelopes are
  identical field for field.

- **`smoke` checks `install.sh --help` does not touch the network.**
  The installers' `--help` branch prints the usage and exits before anything
  is downloaded; a regression would turn `--help` into a version install.
  The smoke suite now runs it (in-process, no network) so the branch cannot
  rot silently.