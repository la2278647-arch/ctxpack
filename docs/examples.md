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
