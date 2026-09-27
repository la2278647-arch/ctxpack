#!/usr/bin/env bash
# Check the two installers against each other and against what the Makefile
# actually publishes.
#
# The installers are the one part of this repository that a reader runs blind:
# it downloads a binary from the GitHub releases and installs it. Every step is
# a guess about something outside the script - the release asset's name, the
# checksum file's format, whether the connection survives. Nothing in `make
# check` touches any of it, and a broken installer is not discoverable until a
# reader hits it.
#
# What this checks, all of it against files that are actually built:
#   1. Syntax. install.sh parses under bash -n. install.ps1 is ASCII-only, so a
#      shell that guesses the encoding wrong still decodes the same bytes - and
#      it is then parsed with EVERY PowerShell on PATH, oldest first, not just
#      the first one found. pwsh 7 accepts source Windows PowerShell 5.1
#      rejects, and 5.1 is what Windows ships, so "the newest shell on PATH is
#      happy" is not a pass. Parsing tries Parser::ParseFile, which reads the
#      file's own bytes, and falls back to [scriptblock]::Create where a
#      restricted installation denies the type literal. If no PowerShell is on
#      PATH the parse half reports SKIP, but the ASCII assertion still runs.
#   2. Asset naming. For every row of the Makefile's OSARCHES the asset name
#      the recipe writes out must equal the name install.sh requests, and for
#      the windows rows also the name install.ps1 requests. Both sides are
#      obtained by EXECUTING the repository's own template lines, not by
#      re-typing the pattern here - a typo in one place moves it.
#   3. Checksum round trip. A fake dist/ is built and the Makefile's exact
#      checksum recipe is run on it. That file is then parsed by install.sh's
#      own awk program and by install.ps1's own parser block, both extracted
#      verbatim, and both must find the right hash - and both must find nothing
#      for an asset that is not listed.
#   4. Retry parity. curl's --retry N means N EXTRA attempts after the first,
#      so --retry 4 is 5 total; install.ps1's DownloadWithRetry attempts that
#      number directly. They must be equal, or one installer gives up before
#      the other does, which is how install.sh failed a live install that
#      install.ps1 completed on the same connection.
#   5. Shared constants: owner, repo, the checksum file's name, the user-agent.
#
# It does not run an install. That needs the network, which CI has and this
# gate may not. The two halves that cannot run without it are the ones a reader
# actually feels; until those are tested the guard is a floor, not a ceiling.
#
# NOTE: this script lives in the tree the example snapshots map, so editing it
# changes examples/ctxpack-self.map.txt. Regenerate those last.
#
# Usage: scripts/check-installers.sh [binary]
# Exit status: 0 when every check passes, 1 otherwise.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE/.." || exit 1
ROOT="$(pwd)"

fail() { echo "FAIL: $*" >&2; exit 1; }

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT

# Every PowerShell on PATH, oldest first: Windows PowerShell 5.1 before pwsh 7.
# The order is load-bearing - see the parse loop in section 1. pwsh 7 accepts
# source that 5.1 rejects, so checking only the newest shell on PATH passed a
# file that would not run for a reader on a stock Windows install.
PSHS=()
for _c in powershell.exe pwsh; do
  _p="$(command -v "$_c" 2>/dev/null || true)"
  if [ -n "$_p" ]; then PSHS+=("$_p"); fi
done
PSH="${PSHS[0]:-}"
SKIP_PS=0
[ -n "$PSH" ] || SKIP_PS=1

# PowerShell on Windows does not understand this shell's POSIX-style paths: it
# reads /tmp/tmp.xyz as D:\tmp\tmp.xyz, fails to find the file, and reports the
# failure as a warning instead of an error, so a loop over the missing file
# comes back empty. Convert before handing a path to PowerShell; a no-op on a
# host that has neither cygpath nor the problem.
nativify() {
  if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf %s "$1"; fi
}

echo "--- 1. syntax ---"
bash -n install.sh || fail "install.sh does not parse"
echo "  install.sh  bash -n ok"

# install.ps1 is delivered over http and parsed by whatever PowerShell the
# reader happens to have. Windows ships 5.1, and 5.1 reads a script with no BOM
# in a single-byte code page: one em dash in a comment turned
# 'Detecting latest release…' into an unterminated string and the whole file
# failed to parse, while pwsh 7 read the same bytes as UTF-8 and accepted it.
# So the check is the source bytes, not what a shell happens to decode them as.
hi="$(printf '\200-\377')"
nonascii="$(tr -cd '\200-\377' < install.ps1 | wc -c | tr -d ' ')"
if [ "$nonascii" != 0 ]; then
  printf 'FAIL: install.ps1 has %s non-ASCII byte(s). Without a BOM Windows\n' "$nonascii" >&2
  printf '      PowerShell 5.1 decodes it in a single-byte code page and stops\n' >&2
  printf '      parsing, so the install dies before it has done anything. An\n' >&2
  printf '      added BOM is not the fix: if the U+FEFF survives the decode that\n' >&2
  printf '      irm | iex performs it glues to the first token and both shells\n' >&2
  printf '      refuse to invoke the result. ASCII is encoding-independent.\n' >&2
  printf '      Offending lines:\n' >&2
  LC_ALL=C grep -n "[${hi}]" install.ps1 >&2 | sed 's/^/        /'
  exit 1
fi
echo "  install.ps1  ASCII only, so every code page decodes it identically"

if [ "$SKIP_PS" -eq 0 ]; then
  # Parser::ParseFile reads the file's own bytes and so reports the error a
  # reader would get. It is used first because it is the faithful check, but
  # some restricted PowerShell installations deny it - the type literal and the
  # generic array it needs are both blocked - so fall back to
  # [scriptblock]::Create, which is still permitted there. Note the fallback is
  # the weaker of the two: Get-Content -Raw decodes in the host's code page and
  # leaves a stray quote intact, which is exactly why it cannot see the defect
  # the ASCII check above does.
  cat > "$W/_parse.ps1" <<'PSEOF'
param([string]$Path)
try {
    $errs = $null
    $null = [System.Management.Automation.Language.Parser]::ParseFile($Path, [ref]$null, [ref]$errs)
    if ($errs -and $errs.Count -gt 0) {
        Write-Output ('parse errors: ' + $errs.Count)
        foreach ($e in $errs) { Write-Output ('  line ' + $e.Extent.StartLineNumber + '  ' + $e.Message) }
        exit 1
    }
    Write-Output 'parse errors: 0 (Parser::ParseFile)'
} catch {
    try {
        [scriptblock]::Create((Get-Content -Raw $Path)) | Out-Null
        Write-Output 'parse errors: 0 ([scriptblock]::Create fallback)'
    } catch {
        Write-Output 'parse errors: 1'
        Write-Output ('  ' + $_.Exception.Message)
        exit 1
    }
}
PSEOF
  for _p in "${PSHS[@]}"; do
    _v="$("$_p" -NoProfile -NoLogo -Command '$PSVersionTable.PSVersion.ToString()' 2>/dev/null)"
    [ -n "$_v" ] || _v=unknown
    out="$("$_p" -NoProfile -NoLogo -File "$(nativify "$W/_parse.ps1")" \
      -Path "$(nativify "$ROOT/install.ps1")" 2>&1)"
    printf '  install.ps1  powershell %-8s %s\n' "$_v" "$(printf '%s\n' "$out" | head -1)"
    printf '%s\n' "$out" | grep -q '^parse errors: 0' \
      || fail "install.ps1 does not parse under PowerShell $_v"
  done
else
  echo "  install.ps1  SKIP - no pwsh or powershell.exe on PATH"
fi

echo "--- 2. asset naming ---"
VER="0.1.12"

# The rows the Makefile builds. Lines are \ -continued.
OSARCHES="$(sed -n '/^OSARCHES[[:space:]]*:=/,/^[^\\]*$/p' Makefile \
  | sed '1s/^OSARCHES[[:space:]]*:=[[:space:]]*//' | tr -d '\\' \
  | tr -s '[:space:]' '\n' | grep -v '^$')"
[ -n "$OSARCHES" ] || fail "could not read OSARCHES from the Makefile"

# Execute install.sh's own ASSET lines in a subshell.
SH_SRC="$(grep -E '^(ASSET=|if \[ "\$OS" = "windows" \])' install.sh)"
[ -n "$SH_SRC" ] || fail "could not find the ASSET line in install.sh"
asset_sh() {
  VER="$1" OS="$2" ARCH="$3" bash -c "$SH_SRC
printf %s \"\$ASSET\""
}

# Execute the Makefile's own -o template. Make escapes $ as $$, so undo that
# and wrap each variable in braces. The braces are not optional: $os_ parses
# as the variable named os_, not os followed by a literal _, because _ is an
# identifier character. A greedy group has the same flaw - sed's
# \([A-Za-z_][A-Za-z_0-9]*\) captures os_ - so convert the names one at a
# time, and treat a leftover variable as a failure instead of an empty one.
MK_TPL="$(grep -o 'dist/ctxpack_$(VERSION)_\$\$os_\$\$arch\$\$ext' Makefile | sed 's|^dist/||' \
  | sed 's/\$(VERSION)/\${VERSION}/g; s/\$\$os/\${os}/g; s/\$\$arch/\${arch}/g; s/\$\$ext/\${ext}/g')"
[ -n "$MK_TPL" ] || fail "could not find the dist asset template in the Makefile"
if printf '%s' "$MK_TPL" | grep -qE '\$\$|\$\(|\$[A-Za-z_]'; then
  fail "the Makefile asset template still has an unconverted variable: $MK_TPL"
fi
asset_mk() {
  VERSION="$1" os="$2" arch="$3" ext="" bash -c "[ \"\$os\" = windows ] && ext=\".exe\";
printf %s \"$MK_TPL\""
}

# install.ps1 writes ctxpack_<Ver>_windows_<Arch>.exe. Evaluate its own $Asset
# line in PowerShell instead of transliterating the pattern into shell syntax:
# a hand-translated template gets expanded by the wrong shell, which is how this
# block lost the OS on its first run.
if [ "$SKIP_PS" -eq 0 ]; then
  PS_ASSET_LINE="$(grep -E '^[[:space:]]*\$Asset[[:space:]]*=' install.ps1 | head -1)"
  [ -n "$PS_ASSET_LINE" ] || fail "could not find the \$Asset assignment in install.ps1"
  cat > "$W/_asset.ps1" <<PSEOF
param([string]\$Arch)
\$Ver = '$VER'
$PS_ASSET_LINE
Write-Output \$Asset
PSEOF
  asset_ps() { "$PSH" -NoProfile -NoLogo -File "$(nativify "$W/_asset.ps1")" -Arch "$1" 2>&1 | tail -1; }
else
  # No PowerShell: install.ps1 cannot be asked at all, and section 1 already
  # reported the SKIP. Leave asset_ps defined so the loop below reads the same.
  asset_ps() { return 0; }
fi

for row in $OSARCHES; do
  os="${row%-*}"
  arch="${row#*-}"
  want="$(asset_mk "$VER" "$os" "$arch")"
  got="$(asset_sh "$VER" "$os" "$arch")"
  [ "$want" = "$got" ] || fail "asset name disagrees for $os-$arch: Makefile=$want install.sh=$got"
  if [ "$os" = windows ] && [ "$SKIP_PS" -eq 0 ]; then
    gotps="$(asset_ps "$arch")"
    [ "$want" = "$gotps" ] \
      || fail "asset name disagrees for $os-$arch: Makefile=$want install.ps1=$gotps"
  fi
  printf '  %-24s ok  %s\n' "$os-$arch" "$want"
done

echo "--- 3. checksum round trip ---"
mkdir -p "$W/dist"
printf 'binary-a\n' > "$W/dist/ctxpack_${VER}_linux_amd64"
printf 'binary-b\n' > "$W/dist/ctxpack_${VER}_windows_amd64.exe"
printf 'binary-c\n' > "$W/dist/ctxpack_${VER}_darwin_arm64"

# The Makefile's own recipe, verbatim.
cd "$W/dist"
( sha256sum -t * > SHA256SUMS.txt 2>/dev/null || sha256 -a 256 * > SHA256SUMS.txt ) \
  || fail "the Makefile's checksum recipe failed"
cd "$ROOT"
echo "  produced:"
sed 's/^/    /' "$W/dist/SHA256SUMS.txt"

ASSET="ctxpack_${VER}_windows_amd64.exe"

# Run install.sh's own lookup line, not a copy of it. A re-typed awk here would
# pass forever while the installer rotted, which is the exact hole this guard is
# meant to close; it also made an awk broken in install.sh invisible.
SH_WANT_LINE="$(grep -E '^WANT="\$\(awk' install.sh | head -1)"
[ -n "$SH_WANT_LINE" ] || fail "could not find the WANT= lookup line in install.sh"
lookup_sh() {
  ASSET="$1" TMP="$W/dist" bash -c "$SH_WANT_LINE
printf %s \"\$WANT\""
}
WANT="$(lookup_sh "$ASSET")"
echo "    install.sh own parser -> ${WANT:-<none>}"
[ -n "$WANT" ] || fail "install.sh's parser did not find ${ASSET}"
printf 'binary-b\n' | sha256sum | cut -d' ' -f1 | grep -qx "$WANT" \
  || fail "install.sh's parser returned the wrong hash"

if [ "$SKIP_PS" -eq 0 ]; then
  # install.ps1's own parser block, extracted between its anchors.
  awk '/^\s*\$Want = \$null$/{s=1} s{print} s&&/^    \}$/{exit}' install.ps1 > "$W/_block.txt"
  [ -s "$W/_block.txt" ] || fail "could not extract install.ps1's parser block"
  echo "    install.ps1 block extracted: $(wc -l < "$W/_block.txt") lines"
  SUMS_WIN="$(nativify "$W/dist/SHA256SUMS.txt")"
  cat > "$W/_psblock.ps1" <<PSEOF
\$SumsTmp = '$SUMS_WIN'
\$Asset = '${ASSET}'
$(cat "$W/_block.txt")
if (-not \$Want) { Write-Output '<none>'; exit }
Write-Output \$Want
PSEOF
  WANTS="$("$PSH" -NoProfile -NoLogo -File "$(nativify "$W/_psblock.ps1")" 2>&1 | tail -1)"
  echo "    install.ps1 loop -> ${WANTS:-<none>}"
  [ "$WANTS" = "$WANT" ] \
    || fail "install.ps1 found '${WANTS}' where install.sh found '${WANT}'"
else
  echo "    install.ps1  SKIP - no pwsh or powershell.exe on PATH"
fi

# A parser that matches names it was not given is worse than one that does not.
MISS="$(lookup_sh "ctxpack_${VER}_nowhere_arm64")"
[ -z "$MISS" ] || fail "install.sh's parser matched an asset that is not listed"
echo "    negative case (an asset that is not listed) -> no match, as required"

echo "--- 4. retry parity ---"
SH_RETRIES="$(grep -F '${BASE}' install.sh | grep -oE -- '--retry [0-9]+' | awk '{print $2}' | sort -u)"
PS_ATTEMPTS="$(grep -oE '\[int\]\$Attempts = [0-9]+' install.ps1 | grep -oE '[0-9]+$')"
[ -n "$SH_RETRIES" ] || fail "found no --retry on install.sh's download calls"
[ -n "$PS_ATTEMPTS" ] || fail "found no \$Attempts default in install.ps1"
for n in $SH_RETRIES; do
  total=$((n + 1))
  [ "$total" = "$PS_ATTEMPTS" ] \
    || fail "install.sh retries $n times ($total total) but install.ps1 attempts $PS_ATTEMPTS"
  printf '  --retry %s = %s total attempts; install.ps1 $Attempts = %s\n' "$n" "$total" "$PS_ATTEMPTS"
done

echo "--- 5. shared constants ---"
for pat in 'la2278647-arch' 'ctxpack' 'ctxpack-installer'; do
  grep -qF "$pat" install.sh || fail "install.sh does not mention $pat"
  grep -qF "$pat" install.ps1 || fail "install.ps1 does not mention $pat"
  printf '  %-20s in both\n' "$pat"
done

# The checksum filename needs an exact check, not a substring one: a rename to
# SHA256SUMS.txt.bak still contains SHA256SUMS.txt and would slip past grep -qF
# while 404-ing on every real install.
for f in install.sh install.ps1; do
  got="$(grep -oE 'SHA256SUMS[A-Za-z0-9_.]*' "$f" | sort -u)"
  [ "$got" = "SHA256SUMS.txt" ] \
    || fail "$f refers to '$(printf '%s' "$got" | tr '\n' ' ')', not SHA256SUMS.txt"
done
printf '  %-20s in both, spelled exactly\n' "SHA256SUMS.txt"

[ "$SKIP_PS" -eq 0 ] && echo "check-installers passed" || echo "check-installers passed (install.ps1 skipped: no PowerShell)"
