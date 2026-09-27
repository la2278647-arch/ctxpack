#!/usr/bin/env bash
# Run every ctxpack command that the current docs show a reader.
#
# docs/examples.md already had a guard that executes the doc's own command.
# This is the same idea for the rest of the surface: README.md, docs/ci.yml,
# docs/examples.md, docs/promote.md and examples/diff-demo/README.md together
# show dozens of invocations,
# and nothing was building or running any of them. A reader copies them by
# hand, so a wrong flag there is a documented lie - the `--list`-on-`pack`
# mistake, or a value the binary rejects, ships as instructions.
#
# Extractor rules, each one there because a naive version got it wrong:
#   - release-notes-*.md is skipped. They are history: v0.1.1 shipped a
#     --version flag that is now a `version` command, and one notes quotes
#     `ctxpack models --bogus` as an example of the error message. Both are
#     correct prose and both would fail a run.
#   - a line ending in \ continues, so only statement lines are candidates.
#   - ;, |, > and & all separate commands, so a redirection must not glue two
#     invocations together.
#   - a unit whose second token is not a real command is prose
#     ("ctxpack 0.1.9 added X"), not a command, and is skipped.
#   - `ctxpack mcp` starts a server on stdio and never exits.
#
# NOTE: this script lives in the tree the example snapshots map, so editing it
# changes examples/ctxpack-self.map.txt. Regenerate those last.
#
# Usage: scripts/check-commands.sh [binary]
# Exit status: 0 when every documented invocation runs, 1 otherwise.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE/.." || exit 1
ROOT="$(pwd)"

fail() { echo "FAIL: $*" >&2; exit 1; }

B="${1:-}"
[ -n "$B" ] || B="$ROOT/bin/ctxpack${EXE:-}"
[ -x "$B" ] || B="$ROOT/bin/ctxpack.exe"
[ -x "$B" ] || { echo "no ctxpack binary; run make build first"; exit 1; }

# Commands are taken from the binary's own help rather than hard-coded, so the
# filter cannot drift when a command is renamed.
CMDS=$("$B" --help 2>&1 | awk '/^COMMANDS/{f=1;next} f&&/^$/{exit} f{print $1}' \
  | tr '\n' ' ' | sed 's/ $//')
[ -n "$CMDS" ] || fail "could not read the command list from \`ctxpack --help\`"

DOCS=(README.md docs/ci.yml docs/examples.md docs/promote.md examples/diff-demo/README.md)
for d in "${DOCS[@]}"; do
  [ -f "$d" ] || fail "docs file $d is missing"
done

# A real repository, because several invocations are git-dependent (diff,
# --ref A..B). Six commits make HEAD~5..HEAD a range that actually resolves.
# examples/ exists because docs/examples.md writes its output there with -o.
R="$(mktemp -d)"
trap 'rm -rf "$R"' EXIT
mkdir -p "$R/internal/util" "$R/cmd/tool" "$R/examples"
printf 'secret\n' > "$R/.env"
for i in 1 2 3 4 5 6; do
  printf 'package main\n\n// c%s\nfunc f%s() { _ = %s }\n' "$i" "$i" "$i" \
    >> "$R/main.go"
  git -C "$R" init -q . 2>/dev/null
  git -C "$R" -c core.autocrlf=false add -A 2>/dev/null
  git -C "$R" -c user.name=t -c user.email=t@t commit -qm "c$i" 2>/dev/null
done
printf 'package util\n\nfunc Add(a, b int) int { return a + b }\n' \
  > "$R/internal/util/util.go"
printf 'package main\n\nfunc TestAdd() {}\n' > "$R/internal/util/util_test.go"
git -C "$R" -c core.autocrlf=false add -A 2>/dev/null
git -C "$R" -c user.name=t -c user.email=t@t commit -qm tree 2>/dev/null
git -C "$R" checkout -qb main 2>/dev/null
printf 'x\n' >> "$R/main.go"
git -C "$R" add -A && git -C "$R" -c user.name=t -c user.email=t@t \
  commit -qm dirty-parent 2>/dev/null

echo "--- every documented ctxpack invocation ---"
set +e
mapfile -t CMDCAND < <(awk -v REPO="$R" -v CMDS="$CMDS" '
  function repl(s, from, to,   out, p) {
    out = ""
    while ((p = index(s, from)) > 0) {
      out = out substr(s, 1, p - 1) to
      s = substr(s, p + length(from))
    }
    return out s
  }
  function emit_unit(u) {
    sub(/^[ \t]+/, "", u)
    sub(/[ \t]+$/, "", u)
    sub(/^ *[A-Za-z0-9_.-]+ *=.*/, "", u)      # drop VAR=... prefixes
    gsub(/<repo>/, REPO, u)                    # diff-demo scratch-repo placeholder
    if (u !~ /^ctxpack[ \t]/) return
    if (u ~ /<[^>]*>/) return                  # <command> is a placeholder
    n = split(u, toks, /[ \t]+/)
    if (toks[2] == "mcp") return               # blocking server
    if (toks[2] == "--help" || toks[2] == "-h") { print "ctxpack --help"; return }
    if (index(" " CMDS " ", " " toks[2] " ") == 0) {
      # Not a real command. Prose like "ctxpack 0.1.9 added X" is fine to skip,
      # but a token made of letters and dashes only looks like a command name,
      # so it is a typo - skipping it would let a broken invocation ship as an
      # instruction, which is the whole thing this guard exists to catch.
      if (toks[2] ~ /^[A-Za-z][A-Za-z0-9-]*$/) print "SUSPECT " u
      return
    }
    if (n == 2) { print "ctxpack " toks[2]; return }   # a bare command mention
    print u
  }
  function emit_line(line,   n, segs, i) {
    gsub(/<repo>/, REPO, line)               # before splitting: > is a separator
    gsub(/[;|>&]/, ";", line)                # each of these starts a new command
    n = split(line, segs, ";")
    for (i = 1; i <= n; i++) emit_unit(segs[i])
  }
  /^```/ { inb = !inb; pending = ""; next }
  inb {
    line = $0
    if (line ~ /^[ \t]*#/) next                # comment line
    if (pending != "") { line = pending line; pending = "" }
    if (line ~ /\\[ \t]*$/) { sub(/\\[ \t]*$/, "", line); pending = line " "; next }
    emit_line(line)
    next
  }
  !inb {
    line = $0
    while (match(line, /`[^`]*ctxpack[^`]*`/) > 0) {
      emit_unit(substr(line, RSTART + 1, RLENGTH - 2))
      line = substr(line, RSTART + RLENGTH)
    }
  }
  END { if (pending != "") emit_line(pending) }
' "${DOCS[@]}" | while read -r u; do
    u=$(printf '%s' "$u" | sed "s|\./myrepo|$R|g; s|\./my-repo|$R|g; s|\./myproject|$R|g; s|\./repo|$R|g; s|/workspace|$R|g; s|/repo|$R|g")
    u=$(printf '%s' "$u" | sed "s| \. | $R |g; s| \.$| $R|")
    printf '%s\n' "$u"
  done | sort -u)
# set -e stays off from here on: a documented command the binary rejects exits
# non-zero inside an assignment, and errexit would stop the script with no
# message instead of reporting which invocation failed.
[ "${#CMDCAND[@]}" -gt 0 ] \
  || fail "extracted no documented invocations, which means the extractor is broken"
printf '  %s candidate invocations\n' "${#CMDCAND[@]}"

# A command name the binary does not have. Reported rather than skipped.
if printf '%s\n' "${CMDCAND[@]}" | grep -q '^SUSPECT '; then
  bad=$(printf '%s\n' "${CMDCAND[@]}" | grep '^SUSPECT ' | head -1 \
    | sed 's/^SUSPECT //' | awk '{print $1 " " $2}')
  fail "docs name a command that does not exist: $bad"
fi
CMDCAND=("${CMDCAND[@]#SUSPECT }")

FAILS=0
for cmd in "${CMDCAND[@]}"; do
  out=$( ( cd "$R" && eval "${cmd/ctxpack/$B}" ) 2>&1 )
  rc=$?
  if [ "$rc" -ne 0 ]; then
    printf '  FAIL rc=%-2s %s\n' "$rc" "$cmd" >&2
    printf '       %s\n' "$(printf '%s' "$out" | head -1 | cut -c1-90)" >&2
    FAILS=$((FAILS + 1))
  else
    printf '  ok %s\n' "$cmd"
  fi
done
[ "$FAILS" -eq 0 ] \
  || fail "$FAILS of ${#CMDCAND[@]} documented invocations fail"
echo "  ${#CMDCAND[@]} invocations, all run"

echo "--- documented environment variables are still read ---"
# README names the CTXPACK_* variables a reader is told to set. Each must still
# be read somewhere real, or the docs tell the reader to set a no-op. Three are
# read by the CLI (internal/cli/cli.go); CTXPACK_INSTALL_DIR is read by the two
# installers, not the binary - searching internal/ alone reports it wrongly;
# and CTXPACK_TAP / CTXPACK_BUCKET are read by scripts/check-release.sh, which
# is neither the CLI nor an installer, so scripts/ is searched too.
for v in $(grep -oE 'CTXPACK_[A-Z_]+' README.md | sort -u); do
  if grep -rq "\"$v\"" internal/ 2>/dev/null \
     || grep -q "$v" install.sh 2>/dev/null \
     || grep -q "$v" install.ps1 2>/dev/null \
     || grep -rq "$v" scripts/ 2>/dev/null; then
    printf '  %s ok\n' "$v"
  else
    printf '  %s FAIL\n' "$v" >&2
    FAILS=$((FAILS + 1))
  fi
done
[ "$FAILS" -eq 0 ] || fail "$FAILS documented environment variables are not read"

echo "check-commands passed"
