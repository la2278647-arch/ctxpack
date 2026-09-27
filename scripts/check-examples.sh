#!/usr/bin/env bash
# Check the committed examples against a fresh run.
#
# examples/ctxpack-self.map.txt and examples/ctxpack-self-budget8000.md are
# generated artifacts that are also committed, so they rot silently the moment
# the tree changes: nobody is building them, so nobody notices.
#
# They are regenerated with --exclude for themselves, and that flag is
# load-bearing rather than cosmetic - a pack of this repository that lists
# itself is not idempotent, because pass one reads the size of the committed
# snapshot and writes a different one, so every regeneration shifts the totals.
# With the snapshots excluded from their own walk the regeneration is a fixed
# point: run it twice and the second run is byte-identical to the first.
# The ctxpack-demo README reaches the same conclusion about itself and
# documents --exclude README.md for the same reason.
#
# NOTE: this script lives in the tree the snapshots map, so editing it changes
# the map. Regenerate the snapshots last, after this file is final.
#
# Usage: scripts/check-examples.sh [binary] [path-to-ctxpack-demo]
# Exit status: 0 when the examples match a fresh run, 1 otherwise.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE/.." || exit 1
ROOT="$(pwd)"

fail() { echo "FAIL: $*" >&2; exit 1; }

# The docs write small counts as words ("the eight files it cut"). Convert a
# count to the word the prose uses, so the comparison is between like values
# instead of between "8" and "eight".
num2word() {
  case "$1" in
    0) echo zero ;; 1) echo one ;; 2) echo two ;; 3) echo three ;;
    4) echo four ;; 5) echo five ;; 6) echo six ;; 7) echo seven ;;
    8) echo eight ;; 9) echo nine ;; 10) echo ten ;; 11) echo eleven ;;
    12) echo twelve ;; 13) echo thirteen ;; 14) echo fourteen ;;
    15) echo fifteen ;; 16) echo sixteen ;; 17) echo seventeen ;;
    18) echo eighteen ;; 19) echo nineteen ;; 20) echo twenty ;;
    *) echo "$1" ;;
  esac
}

B="${1:-}"
[ -n "$B" ] || B="$ROOT/bin/ctxpack${EXE:-}"
[ -x "$B" ] || B="$ROOT/bin/ctxpack.exe"
[ -x "$B" ] || { echo "no ctxpack binary; run make build first"; exit 1; }

DOC="$ROOT/docs/examples.md"
# The doc wraps at 76 columns, so a phrase like "at ~4986 tokens" can straddle
# a newline. Flatten it once and extract every number from the flat text, or a
# cosmetic reformatting edit would break the comparison for no reason. Squeezing
# collapses the double space the wrap leaves behind, so the patterns can keep
# matching on a single space.
DOCTXT="$(tr '\n' ' ' < "$DOC" | tr -s '[:space:]' ' ')"

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT

echo "--- the documented regeneration command reproduces the committed snapshots ---"
# Pull the fenced shell block out of the doc and run it verbatim in a scratch
# copy of the tree, then compare its output with what is committed. This proves
# the documented command works as written, instead of asserting that certain
# flags happen to appear somewhere in the prose. The copy is named ctxpack
# because the map records the directory name.
BLOCK="$(awk '/^## Regenerating/{f=1;next} f&&/^```/{n++;next} f&&n==1{print}' "$DOC")"
[ -n "$BLOCK" ] \
  || fail "docs/examples.md has no fenced shell block under ## Regenerating"

cp -a . "$W/ctxpack"
mkdir -p "$W/orig"
cp "$W/ctxpack/examples/ctxpack-self.map.txt" "$W/orig/"
cp "$W/ctxpack/examples/ctxpack-self-budget8000.md" "$W/orig/"
# The block calls bare `ctxpack`, so put the binary it was resolved from on PATH.
( cd "$W/ctxpack" && PATH="$(dirname "$B"):$PATH" bash -c "$BLOCK" ) \
  >/dev/null 2>&1 \
  || fail "the documented regeneration command failed to run"

for f in ctxpack-self.map.txt ctxpack-self-budget8000.md; do
  if diff -q "$W/orig/$f" "$W/ctxpack/examples/$f" >/dev/null; then
    echo "  examples/$f ok"
  else
    echo "--- drift in examples/$f ---" >&2
    diff "$W/orig/$f" "$W/ctxpack/examples/$f" | head -12 >&2
    fail "examples/$f does not match what the documented command produces"
  fi
done

echo "--- the documented regeneration excludes the snapshots from their own walk ---"
# The check above catches a dropped flag only as opaque drift. Assert each flag
# directly so the failure message says what was lost, and so a snapshot that
# packed itself is named as the cause rather than reported as a mismatch.
grep -q -- '--exclude examples/ctxpack-self.map.txt' <<< "$DOCTXT" \
  || fail "docs/examples.md does not exclude the map snapshot from its own walk"
grep -q -- '--exclude examples/ctxpack-self-budget8000.md' <<< "$DOCTXT" \
  || fail "docs/examples.md does not exclude the pack snapshot from its own walk"
echo "  both snapshots are excluded from their own regeneration"

echo "--- the self-pack's budget claim holds ---"
# docs/examples.md calls it "a pack of this repository capped at 8000 tokens".
tok=$(grep -oE 'Tokens: ~[0-9]+' examples/ctxpack-self-budget8000.md | head -1 \
  | grep -oE '[0-9]+')
[ -n "$tok" ] || fail "cannot read the pack header"
[ "$tok" -le 8000 ] || fail "the self-pack is ~$tok tokens, not capped at 8000"
echo "  ~$tok tokens, within the documented 8000 cap"

echo "--- the demo numbers in docs/examples.md ---"
DEMO="${2:-$ROOT/../ctxpack-demo}"
if [ ! -d "$DEMO/.git" ]; then
  echo "  SKIP: no ctxpack-demo checkout at $DEMO"
  echo "  (pass its path as \$2, or clone it beside this repository)"
else
  cd "$DEMO"
  TOKENS_OUT=$("$B" tokens .)
  MAP_OUT=$("$B" map .)
  # Exactly the command docs/examples.md documents - no extra flags, so this
  # compares the documented prose with what the documented command prints.
  PACK_OUT=$("$B" pack . --model gpt-4o --format markdown --budget 5000 2>/dev/null)

  # Every grep below returns non-zero when it finds nothing, which under
  # set -e would abort the script with no message at all - the failure mode
  # scripts/smoke.sh's header warns about. So the body runs with set -e
  # disabled and every empty extraction is reported explicitly by fail.
  set +e
  GOT_TOKENS=$(printf '%s' "$TOKENS_OUT" | grep -oE '^Tokens:.*' | grep -oE '[0-9]+' | head -1)
  GOT_S35=$(printf '%s' "$TOKENS_OUT" | grep 'gpt-3.5-turbo' \
    | grep -oE '\([0-9]+%\)$' | grep -oE '[0-9]+')
  GOT_G4=$(printf '%s' "$TOKENS_OUT" | grep -E 'gpt-4[[:space:]]' \
    | grep -oE '\([0-9]+%\)$' | grep -oE '[0-9]+')
  GOT_SEED=$(printf '%s' "$MAP_OUT" | grep 'seed.json' \
    | grep -oE '\[[0-9]+t' | grep -oE '[0-9]+' | head -1)
  GOT_KEPT=$(printf '%s' "$PACK_OUT" | grep -oE 'Files: [0-9]+' | grep -oE '[0-9]+')
  GOT_KEPTTOK=$(printf '%s' "$PACK_OUT" | grep -oE 'Tokens: ~[0-9]+' | grep -oE '[0-9]+')
  GOT_OMIT=$(printf '%s' "$PACK_OUT" | grep -oE 'Omitted by budget \([0-9]+' | grep -oE '[0-9]+')

  for n in GOT_TOKENS GOT_S35 GOT_G4 GOT_SEED GOT_KEPT GOT_KEPTTOK GOT_OMIT; do
    [ -n "${!n}" ] || { set -e; fail "could not read $n from the demo run"; }
  done

  want=$(grep -oE 'reports [0-9]+ estimated tokens' <<< "$DOCTXT" | grep -oE '[0-9]+')
  [ "$GOT_TOKENS" = "$want" ] \
    || fail "docs/examples.md says $want tokens, the demo reports $GOT_TOKENS"
  want=$(grep -oE 'gpt-3.5-turbo` at [0-9]+%' <<< "$DOCTXT" | grep -oE '[0-9]+%' | grep -oE '[0-9]+')
  [ "$GOT_S35" = "$want" ] \
    || fail "docs/examples.md says gpt-3.5-turbo at $want%, the demo reports ${GOT_S35}%"
  want=$(grep -oE 'gpt-4` at [0-9]+%' <<< "$DOCTXT" | grep -oE '[0-9]+%' | grep -oE '[0-9]+')
  [ "$GOT_G4" = "$want" ] \
    || fail "docs/examples.md says gpt-4 at $want%, the demo reports ${GOT_G4}%"
  want=$(grep -oE 'keeps [0-9]+ files' <<< "$DOCTXT" | grep -oE '[0-9]+')
  [ "$GOT_KEPT" = "$want" ] \
    || fail "docs/examples.md says $want files kept, the demo keeps $GOT_KEPT"
  want=$(grep -oE 'at ~[0-9]+ tokens' <<< "$DOCTXT" | grep -oE '[0-9]+')
  [ "$GOT_KEPTTOK" = "$want" ] \
    || fail "docs/examples.md says ~$want tokens, the demo packs ~$GOT_KEPTTOK"
  want=$(grep -oE 'lists the [a-z]+ files it cut' <<< "$DOCTXT" \
    | sed -E 's/lists the ([a-z]+) files it cut/\1/')
  [ "$want" = "$(num2word "$GOT_OMIT")" ] \
    || fail "docs/examples.md says $want files cut, the demo cuts $GOT_OMIT"
  want=$(grep -oE 'at [0-9]+ tokens, [0-9.]+% of the whole' <<< "$DOCTXT" | grep -oE '[0-9.]+%')
  gotpct=$(awk -v s="$GOT_SEED" -v t="$GOT_TOKENS" 'BEGIN {printf "%.1f", s*100/t}')
  [ "${gotpct}%" = "$want" ] \
    || fail "docs/examples.md says ${want} of the tree, seed.json is ${gotpct}%"

  echo "  tokens $GOT_TOKENS, gpt-3.5-turbo ${GOT_S35}%, gpt-4 ${GOT_G4}%"
  echo "  keeps $GOT_KEPT files at ~$GOT_KEPTTOK tokens, cuts $GOT_OMIT"
  echo "  seed.json $GOT_SEED tokens = ${gotpct}% of the tree"
  echo "  all demo numbers match"
  set -e
fi

echo "check-examples passed"
