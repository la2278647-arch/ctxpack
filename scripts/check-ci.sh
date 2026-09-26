#!/usr/bin/env bash
# Audit docs/ci.yml.
#
# That workflow lives in docs/ on purpose: the token that publishes this
# repository does not carry the `workflow` scope, so it can never be pushed
# under .github/workflows/ and therefore can never run. A workflow that is never
# executed rots silently - a shell block that no longer parses, a script it
# calls that was moved, a gate that quietly drifts from the local one. This
# checks what can be checked on any host that has bash and awk.
#
# Usage: scripts/check-ci.sh
# Exit status: 0 when docs/ci.yml is sound, 1 otherwise.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE/.." || exit 1

CI="docs/ci.yml"
MF="Makefile"

fail() { echo "FAIL: $*" >&2; exit 1; }
[ -f "$CI" ] || fail "missing $CI"
[ -f "$MF" ] || fail "missing $MF"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "--- run: blocks parse as bash ---"
# Pull every run: value out of the workflow - block scalar or inline string -
# into blockN.sh so bash -n can reject it.
#
# Indentation is computed with substr, not match()/RLENGTH: in a multi-line awk
# program gawk reports "unterminated regexp" on a match() line when a RLENGTH
# assignment follows, aborting before it gets to the offending regex, so this
# script would die with a message about the wrong line. Indentation decides
# whether a line continues the preceding block scalar.
awk -v tmp="$tmp" '
  function ind(s,   i, n) {
    n = length(s)
    for (i = 1; i <= n; i++) {
      if (substr(s, i, 1) != " " && substr(s, i, 1) != "\t") break
    }
    return i - 1
  }
  {
    line = $0
    sub(/\r$/, "", line)
    i = ind(line)
    if (inblock) {
      if (i > runindent && line ~ /[^ \t]/) { print line >> (tmp "/block" n ".sh"); next }
      inblock = 0
      next
    }
    if (i > 0 && substr(line, i + 1, 4) == "run:") {
      val = substr(line, i + 5)
      while (val ~ /^[ \t]/) val = substr(val, 2)
      if (substr(val, 1, 1) == "|") {
        inblock = 1
        runindent = i
        n++
        printf "\n" > (tmp "/block" n ".sh")
        next
      }
      if (val ~ /[^ \t]/) { n++; print val > (tmp "/block" n ".sh") }
    }
  }
' "$CI"
blocks=$(find "$tmp" -name 'block*.sh' | sort -t. -k2 -n)
[ -n "$blocks" ] || fail "no run: blocks found in $CI"
count=0
for f in $blocks; do
  count=$((count + 1))
  bash -n "$f" || { echo "--- block $count ---" >&2; sed -n '1,8p' "$f" >&2
    fail "run: block $count does not parse"
  }
done
# A malformed run: line is silently skipped by the extraction, so the block
# count must equal the number of run: lines the workflow declares. Without this
# the audit can pass on a workflow it only half-read.
declared=$(grep -c '^[[:space:]]*run:' "$CI")
[ "$count" -eq "$declared" ] || fail "extracted $count run: blocks but $CI declares $declared"
echo "  all $count run: blocks parse ($declared run: steps)"

echo "--- scripts it calls exist ---"
for script in $(grep -o 'bash scripts/[A-Za-z0-9._-]*' "$CI" | sed 's/^bash //' | sort -u); do
  [ -f "$script" ] || fail "$CI calls $script, which does not exist"
  echo "  $script ok"
done

echo "--- Make targets it names exist ---"
# The header comment tells a future reader which local targets are equivalent.
# Each name it uses must be a real target, or the comment points at nothing.
for target in $(grep -o '`make [a-z]*`' "$CI" | sed 's/`make //; s/`$//' | sort -u); do
  grep -qE "^[[:space:]]*$target:" "$MF" || fail "$CI names make $target, not a target in $MF"
  echo "  make $target ok"
done

echo "--- the gate matches make check ---"
# These five primitives make up the gate. Both files must assert all five, so
# "the local equivalent of what CI runs" stays literally true rather than being
# a claim that has drifted. The go.sum gate is matched on the operator plus the
# filename, because CI negates it (test ! -s go.sum) while the Makefile inverts
# it (if [ -s go.sum ]; then exit 1) - matching the whole phrasing would be a
# false failure, and matching the bare filename would match the header prose.
for pat in 'gofmt -l' 'vet \./\.\.\.' 'test \./\.\.\. -count=1' '-s go\.sum' 'list -m all'; do
  grep -Eq -- "$pat" "$CI" || fail "$CI is missing gate primitive: $pat"
  grep -Eq -- "$pat" "$MF" || fail "$MF is missing gate primitive: $pat"
  echo "  $pat ok in both"
done

echo "--- smoke has one source of truth ---"
grep -q 'scripts/smoke.sh' "$CI" || fail "$CI does not call scripts/smoke.sh"
grep -q 'scripts/smoke.sh' "$MF" || fail "$MF does not call scripts/smoke.sh"
echo "  both call scripts/smoke.sh"

echo "check-ci passed"
