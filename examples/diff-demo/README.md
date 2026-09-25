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
