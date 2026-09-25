#!/usr/bin/env bash
# Mirrors the ctxpack Makefile smoke target: exercises the whole CLI surface
# against the source tree itself, including the diff range and deleted-file
# cases that need a throwaway git repository.
set -euo pipefail

# Resolve the binary and repository root up front: this script lives in
# scripts/ and the binary in bin/, so relative paths have to be pinned before
# anything changes directory.
B="${1:-}"
HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE/.."
ROOT="$(pwd)"
[ -n "$B" ] || B="$ROOT/bin/ctxpack${EXE:-}"
# A Windows host needs the .exe name; git-bash and a POSIX shell do not.
[ -x "$B" ] || { B="$ROOT/bin/ctxpack.exe"; }
[ -x "$B" ] || { echo "build first: make build"; exit 1; }

echo "--- commands ---"
"$B" version
"$B" models | grep -qi gpt
"$B" tokens . | grep -qi tokens
"$B" map . | grep -qi repository
"$B" pack . > /dev/null 2>&1

echo "--- formats ---"
OUT="$(mktemp)"
"$B" pack . --format json | grep -q '"files"'
"$B" pack . --format markdown | grep -q '^#'
"$B" pack . --format text | grep -q '^Repository:'
"$B" pack . --format xml | grep -q '<repository>'
"$B" pack . --format markdown --budget 500 > /dev/null
# -o writes the artifact to stdout-less output; the "wrote" status line is
# stderr, so discard both and assert the file itself is non-empty.
"$B" pack . -o "$OUT" >/dev/null 2>&1 && test -s "$OUT"
rm -f "$OUT"

echo "--- doctor ---"
# doctor's report IS the artifact, so it goes to stdout — unlike --dry-run,
# whose status report travels on stderr. Assert the shape, not just that it
# runs, so a missing diagnostic shows up here instead of in the field.
DOC="$("$B" doctor)"
grep -q 'ctxpack diagnostics:' <<< "$DOC"
grep -q 'Version:' <<< "$DOC"
grep -q 'Platform:' <<< "$DOC"
grep -q 'Git:' <<< "$DOC"
grep -q 'Models:' <<< "$DOC"
grep -q 'Vendor breakdown:' <<< "$DOC"
# --top truncates the vendor list but must not touch the totals, or a truncated
# report would understate the registry's size.
TOTALS="$(grep -oE '[0-9]+ models, [0-9]+ vendors' <<< "$DOC")"
[ -n "$TOTALS" ] || { echo "FAIL: doctor reports no totals"; exit 1; }
DOC_N="$(grep -c 'model(s)' <<< "$DOC")"
DOC_N="$((DOC_N + 0))"
TOP="$("$B" doctor --top 1)"
grep -qF "$TOTALS" <<< "$TOP" || { echo "FAIL: --top changed the totals"; exit 1; }
TOP_N="$(grep -c 'model(s)' <<< "$TOP")"
TOP_N="$((TOP_N + 0))"
if [ "$DOC_N" -le 1 ] || [ "$TOP_N" != "1" ]; then
  echo "FAIL: --top 1 did not truncate to one vendor ($DOC_N -> $TOP_N)"
  exit 1
fi
# JSON diagnostics and --output.
"$B" doctor --json | grep -q '"model_count"'
"$B" doctor --format json | grep -q '"version"'
"$B" doctor -o "$OUT" >/dev/null 2>&1 && grep -q 'diagnostics' "$OUT" && rm -f "$OUT"
# Error paths must exit non-zero rather than print a partial report.
"$B" doctor --format yaml >/dev/null 2>&1 && { echo "FAIL: unknown format accepted"; exit 1; }
"$B" doctor extraneous >/dev/null 2>&1 && { echo "FAIL: positional argument accepted"; exit 1; }

echo "--- flag surface ---"
"$B" map . --sort tokens --top 5 > /dev/null
"$B" tokens . --sort window --top 3 > /dev/null
"$B" models --sort vendor --top 5 > /dev/null
"$B" models --format json | grep -q '"models"'
# --dry-run writes its report to stderr by design: this codebase keeps
# artifacts on stdout and status on stderr, so a dry-run report is unreachable
# through a pipe. Assert that explicitly rather than silently discarding it.
DRY="$("$B" pack . --dry-run 2>&1 >/dev/null)"
grep -q 'dry run:' <<< "$DRY" || { echo "FAIL: pack --dry-run reports nothing"; exit 1; }
echo x >> README.md
"$B" diff --list | grep -q README.md
DDRY="$("$B" diff --dry-run 2>&1 >/dev/null)"
grep -q 'dry run' <<< "$DDRY" || { echo "FAIL: diff --dry-run reports nothing"; exit 1; }
"$B" diff --format json | grep -q '"files"'
"$B" diff --format text | grep -q '^Repository:'
git checkout -- README.md

echo "--- diff range excludes the working tree ---"
R="$(mktemp -d)"
cleanup() { rm -rf "$R"; }
trap cleanup EXIT
(cd "$R" && git init -q \
  && git config user.email t@t.invalid && git config user.name t \
  && git config core.autocrlf false \
  && echo one > a.txt && git add -A && git commit -qm one \
  && echo two > a.txt && git add -A && git commit -qm two \
  && echo three > untracked.txt)
RANGE="$("$B" diff --list --ref 'HEAD~1..HEAD' "$R")"
if grep -q untracked <<< "$RANGE"; then
  echo "FAIL: range ref picked up a working-tree file"
  exit 1
fi
if grep -q 'a.txt' <<< "$RANGE"; then
  echo "PASS: range lists the changed file ($RANGE)"
else
  echo "FAIL: range did not list a.txt ($RANGE)"
  exit 1
fi

echo "--- diff reports a deletion ---"
(cd "$R" && git rm -q a.txt && git commit -qm rm)
"$B" diff --format xml --ref HEAD~1 "$R" | grep -q '<deleted'
"$B" diff --format text --ref HEAD~1 "$R" | grep -q 'deleted'
"$B" diff --format markdown --ref HEAD~1 "$R" | grep -q '^## Deleted'
"$B" diff --format json --ref HEAD~1 "$R" | grep -q '"deleted"'
# --list must never name a deletion: it feeds `while read f; do` loops that
# would choke on a path whose content is gone. The untracked.txt still sitting
# in the worktree IS listed, because a plain ref diffs against the worktree —
# so assert the absence of a.txt specifically, not an empty result.
LISTED="$("$B" diff --list --ref HEAD~1 "$R")"
if grep -q 'a\.txt' <<< "$LISTED"; then
  echo "FAIL: --list must not name deleted files"
  exit 1
fi
echo "PASS: --list names the worktree file, not the deletion"

echo "--- filter flags ---"
# The text header is "Files: N | Tokens: ~T | Bytes: ...". Both helpers read
# it, and the assertions below are relative so they stay valid as the tree
# grows instead of pinning a number that will drift.
file_count() { grep -oE '^Files: [0-9]+' <<< "$1" | head -n 1 | grep -oE '[0-9]+'; }
tok_count()  { grep -oE 'Tokens: ~[0-9]+' <<< "$1" | head -n 1 | grep -oE '[0-9]+'; }
fail() { echo "FAIL: $*" >&2; exit 1; }

ALL="$("$B" pack . --format text)"
N_ALL="$(file_count "$ALL")";  T_ALL="$(tok_count "$ALL")"
[ "$N_ALL" -gt 0 ] || fail "no file count parsed ($N_ALL)"

# map --csv: a flat, headered list.
"$B" map . --csv | grep -q '^path,tokens,bytes'

# models --vendor narrows to one vendor and rejects an unknown one.
"$B" models --vendor anthropic | grep -q claude
"$B" models --vendor nonexistent >/dev/null 2>&1 && fail "models accepts an unknown vendor"

# --depth limits traversal strictly: a shallow walk finds fewer files.
N_DEPTH="$(file_count "$("$B" pack . --depth 1 --format text)")"
[ "$N_DEPTH" -gt 0 ] || fail "--depth 1 produced no files"
[ "$N_DEPTH" -lt "$N_ALL" ] || fail "--depth 1 ($N_DEPTH) did not reduce the count ($N_ALL)"

# --max-size caps content, not the file list: count holds, tokens collapse.
SMALL="$("$B" pack . --max-size 10 --format text)"
N_SMALL="$(file_count "$SMALL")"; T_SMALL="$(tok_count "$SMALL")"
[ "$N_SMALL" -eq "$N_ALL" ] || fail "--max-size changed the file count ($N_ALL -> $N_SMALL)"
[ "$T_SMALL" -lt "$T_ALL" ] || fail "--max-size did not reduce tokens ($T_ALL -> $T_SMALL)"

# --include keeps only matches, --exclude drops them; each must shrink the set.
# Only the "==== path (N tokens) ====" entry headers are authoritative here:
# the file bodies of README.md and CHANGELOG.md legitimately contain the text
# "internal/", so grepping the whole bundle would report a false leak.
entry_paths() { sed -nE 's/^==== (.+) \([0-9]+ tokens\) ====/\1/p' <<< "$1"; }
INC="$("$B" pack . --include '*.md' --format text)"
N_INC="$(file_count "$INC")"
grep -qx README.md <<< "$(entry_paths "$INC")" || fail "--include '*.md' dropped README.md"
grep -qE 'internal/' <<< "$(entry_paths "$INC")" && fail "--include '*.md' leaked internal/ paths"
[ "$N_INC" -lt "$N_ALL" ] || fail "--include '*.md' did not reduce ($N_ALL -> $N_INC)"

EXC="$("$B" pack . --exclude 'internal/*' --format text)"
N_EXC="$(file_count "$EXC")"
grep -qE 'internal/' <<< "$(entry_paths "$EXC")" && fail "--exclude leaked internal/ paths"
[ "$N_EXC" -lt "$N_ALL" ] || fail "--exclude did not reduce ($N_ALL -> $N_EXC)"

# --hidden adds dotfiles; --no-gitignore can only keep or add files.
N_HIDDEN="$(file_count "$("$B" pack . --hidden --format text)")"
N_NOGIT="$(file_count "$("$B" pack . --no-gitignore --format text)")"
[ "$N_HIDDEN" -gt "$N_ALL" ] || fail "--hidden found no extra files ($N_ALL -> $N_HIDDEN)"
[ "$N_NOGIT" -ge "$N_ALL" ] || fail "--no-gitignore reduced the count ($N_ALL -> $N_NOGIT)"

echo "--- mcp ---"
# One JSON-RPC conversation per call: initialize, then a single tools/call.
# The last output line is the response to that call.
mcp_call() {
  printf '%s\n%s\n' \
    '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
    "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" \
    | "$B" mcp | tail -n 1
}

# tools/list must advertise every tool, and each one must actually be callable.
LIST="$(printf '%s\n%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | "$B" mcp | tail -n 1)"
for t in pack_repo repo_map count_tokens list_models diff_repo doctor; do
  grep -q "\"name\":\"$t\"" <<< "$LIST" || fail "tools/list does not advertise $t"
done

# The tool result is a JSON *string* inside the envelope, so the inner keys
# arrive escaped as \"tree\" rather than "tree". Match the escaped form.
mcp_call repo_map '{"path":".","format":"json"}' | grep -qF '\"tree\"'
mcp_call pack_repo '{"path":".","format":"json","max_depth":1}' | grep -qF '\"files\"'
mcp_call count_tokens '{"path":".","model":"gpt-4o"}' | grep -q 'gpt-4o'
mcp_call list_models '{"format":"json"}' | grep -qF '\"models\"'
# doctor is the one tool that takes no path, so it must work with an empty
# argument object: that is its whole purpose, for a client that cannot name a
# repository. Text mode must state the registry size; json must be structured.
mcp_call doctor '{}' | grep -q 'ctxpack diagnostics:'
mcp_call doctor '{"format":"json"}' | grep -qF '\"model_count\"'
# A range ref is deterministic: it compares two revisions and never touches the
# working tree, so it cannot pass merely because an untracked file happens to
# be lying around.
mcp_call diff_repo '{"path":".","list":true,"ref":"HEAD~1..HEAD"}' \
  | grep -qE '"result".*"text":"[A-Za-z]'
# Errors come back inside the envelope as isError, not as a JSON-RPC error
# object, so a client must check isError rather than the top-level shape.
mcp_call pack_repo '{"path":".","format":"yaml"}' | grep -q 'isError'
mcp_call diff_repo '{"path":".","format":"yaml"}' | grep -q 'isError'
mcp_call pack_repo '{"format":"json"}' | grep -q 'missing required argument'
mcp_call diff_repo '{"path":".","ref":"not-a-ref"}' | grep -q 'isError'
# doctor offers only text and json, not the xml/markdown the packing tools
# accept, so a format:xml call must fail rather than quietly returning text.
mcp_call doctor '{"format":"xml"}' | grep -q 'isError'

echo "smoke passed"
