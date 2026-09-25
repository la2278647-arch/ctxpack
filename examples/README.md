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
keeps the 10 highest-priority files (~7.9k tokens) and **lists the 48 files it
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
