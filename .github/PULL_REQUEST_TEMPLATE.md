## What and why

One sentence on the change, one on why it matters. Small PRs land faster.

## Checklist

- [ ] `go vet ./...` and `go test ./... -count=1` pass
- [ ] Behaviour change: a test covers it
- [ ] User-facing flag or command change: `README.md` and the help text in
      `internal/cli/cli.go` updated in the same commit
- [ ] No new module dependencies (`go.sum` must stay empty)
- [ ] If the tree changed: `bash scripts/check-examples.sh` regenerated the
      self-snapshots (see `docs/examples.md`, "Regenerating")

## Testing

What you ran and what you saw. A copy of the command output for the touched
behaviour is worth more than prose.