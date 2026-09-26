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
`==== deleted (N files) ====` in text — `1 file` when it is one, as here — and a
`deleted` array in JSON.

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

## `--list` names deletions too

```sh
ctxpack diff <repo> --list --ref HEAD~1..HEAD
# src/app.go
# src/util.go
# src/legacy.go
```

`--list` answers one question — which paths will the bundle name? — and a
deletion is named in the `<deleted>` element, so `--list` says so too. That is
the convention `git diff --name-only` uses.

The practical consequence is that a listed path may not exist on disk. A loop
such as `ctxpack diff . --list | while read -r f; do wc -l "$f"; done` will trip
over `src/legacy.go`, so test `test -f "$f"` first. `--dry-run` is the call that
keeps the two apart, which is what it exists for:

```console
$ ctxpack diff <repo> --dry-run --ref HEAD~1..HEAD
dry run vs "HEAD~1..HEAD": 2 files, ~141 tokens, 328 B
  src/app.go (~72 tokens, 167 B)
  src/util.go (~69 tokens, 161 B)
  ... 1 file deleted
    D src/legacy.go
```

The same filters the pack applies — `--include`, `--exclude`, `--max-size`,
`--depth`, `--no-gitignore`, `--hidden` — narrow `--list` too, so a filter the
caller expects to narrow the bundle is not silently ignored by the one call that
pretends not to build it. The deletion is named by path and is therefore not
filtered: the file is gone, and there is no content left to size or skip.

## Files

- `make.sh` — regenerates `diff.xml`; idempotent and self-cleaning.
- `diff.xml` — the generated output.
