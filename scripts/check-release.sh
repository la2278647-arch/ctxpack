#!/usr/bin/env bash
#
# check-release.sh — verify a published GitHub release against this repository.
#
# Nothing in the test suite opens a network connection: `make check` builds and
# runs against a scratch directory. A release is different. It is published from
# another machine by another process, so a green test suite says nothing about
# what a reader actually receives. This is the only script here that talks to
# the network, which is why the target that runs it is not in `ci`.
#
# It checks that the manifest is well formed and names exactly the assets the
# Makefile would build, that the Homebrew tap and scoop bucket are at the same
# version, that the formula's sha256 still matches the published archive, and
# that every bucket hash matches the manifest.
#
# Usage:
#   bash scripts/check-release.sh [--install] [--tag v0.1.10] [--base URL]
#
# --install additionally runs install.sh against a temp dir, which downloads
# the binary. --base takes a mirror URL and defaults to upstream; it exists
# because a guard that cannot be pointed at a fixture cannot be tested.
# The tap and bucket are siblings of this repository, read from ../homebrew-tap
# and ../scoop-bucket, or from CTXPACK_TAP and CTXPACK_BUCKET, and are skipped
# when absent.
#
# Exit codes: 0 ok, 1 the release disagrees with the repository,
#             2 the manifest could not be fetched, so nothing was checked.
#
# Requires curl, awk, sed, diff and sha256sum (or shasum).
set -uo pipefail

usage() { sed -n '/^# Usage:/,/^# Requires/p' "$0" | sed 's/^# \{0,1\}//'; }
[ "$#" -eq 0 ] || case "${1:-}" in -h|--help) usage; exit 0 ;; esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1

DO_INSTALL=0
TAG=""
BASE=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --install) DO_INSTALL=1 ;;
    --tag) TAG="${2:-}"; shift ;;
    --base) BASE="${2:-}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ctxpack: unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

# command -v prints a full path (/usr/bin/sha256sum), not a bare name, so
# compare on the basename - a plain string equality never matched and this
# block silently fell through to shasum.
SHA_CMD="$(command -v sha256sum 2>/dev/null || command -v shasum 2>/dev/null || true)"
[ -n "$SHA_CMD" ] || { echo "ctxpack: need sha256sum or shasum" >&2; exit 1; }
sha() {
  case "$(basename "$SHA_CMD")" in
    sha256sum) sha256sum "$1" | cut -d' ' -f1 ;;
    *)         shasum -a 256 "$1" | cut -d' ' -f1 ;;
  esac
}

dl() {
  # --retry-all-errors matters: on a slow connection github resets mid-body, and
  # curl's default retry list does not cover that, so the first attempt aborts.
  curl -fsSL --retry 6 --retry-all-errors --retry-delay 2 --max-time 240 -o "$2" "$1"
}

VER="$(sed -n 's/^VERSION[[:space:]]*?= //p' Makefile | head -1)"
[ -n "$VER" ] || fail "could not read VERSION from the Makefile"
[ -n "$TAG" ] || TAG="v$VER"
[ -n "$BASE" ] || BASE="https://github.com/la2278647-arch/ctxpack"
VER_NOV="${TAG#v}"

W="$(mktemp -d)"
DEST=""
cleanup() { [ -n "$W" ] && rm -rf "$W"; [ -n "$DEST" ] && rm -rf "$DEST"; }
trap cleanup EXIT
MAN="$W/SHA256SUMS.txt"

echo "=== release $TAG from $BASE ==="
if ! dl "$BASE/releases/download/$TAG/SHA256SUMS.txt" "$MAN"; then
  printf 'ctxpack: could not fetch the manifest, so nothing was checked.\n' >&2
  exit 2
fi
[ -s "$MAN" ] || fail "the manifest is empty"

echo "--- 1. the manifest ---"
crs="$(tr -dc '\r' < "$MAN" | wc -c | tr -d ' ')"
[ "$crs" = 0 ] || fail "the manifest has $crs CR byte(s); both installers assume LF"
echo "  line endings: LF only"

bad="$(awk 'NF != 2 || length($1) != 64 || $1 !~ /^[0-9a-f]+$/ || $2 ~ /^\*/ {
  print "    line " NR ": " $0 }' "$MAN")"
[ -z "$bad" ] || fail "the manifest is not in text mode (hash, two spaces, name):
$bad"
echo "  format: text mode, 64 hex chars, no leading asterisk"

awk 'NF == 2 {print $2}' "$MAN" | grep -qx 'SHA256SUMS.txt' \
  && fail "the manifest lists itself" || true

# The rows the Makefile builds. Lines are backslash-continued.
OSARCHES="$(sed -n '/^OSARCHES[[:space:]]*:=/,/^[^\\]*$/p' Makefile \
  | sed '1s/^OSARCHES[[:space:]]*:=[[:space:]]*//' | tr -d '\\' \
  | tr -s '[:space:]' '\n' | grep -v '^$')"
[ -n "$OSARCHES" ] || fail "could not read OSARCHES from the Makefile"

# Execute the Makefile's own -o template. Make escapes $ as $$, so undo that
# and brace every variable: $os_ parses as the variable named os_, because _
# is an identifier character, so an unbraced template silently drops the OS.
MK_TPL="$(grep -o 'dist/ctxpack_$(VERSION)_\$\$os_\$\$arch\$\$ext' Makefile \
  | sed 's|^dist/||' \
  | sed 's/\$(VERSION)/\${VERSION}/g; s/\$\$os/\${os}/g; s/\$\$arch/\${arch}/g; s/\$\$ext/\${ext}/g')"
[ -n "$MK_TPL" ] || fail "could not find the dist asset template in the Makefile"
if printf '%s' "$MK_TPL" | grep -qE '\$\$|\$\(|\$[A-Za-z_]'; then
  fail "the Makefile asset template still has an unconverted variable: $MK_TPL"
fi
asset_mk() {
  VERSION="$1" os="$2" arch="$3" ext="" bash -c "[ \"\$os\" = windows ] && ext=\".exe\";
printf %s \"$MK_TPL\""
}
: > "$W/_names"
for row in $OSARCHES; do
  # asset_mk prints without a newline, so wrap it: appended raw, the eight
  # names ran together into a single line.
  printf '%s\n' "$(asset_mk "$VER_NOV" "${row%-*}" "${row#*-}")" >> "$W/_names"
done
awk 'NF == 2 {print $2}' "$MAN" | sort -u > "$W/_listed"
sort -u "$W/_names" -o "$W/_names"
if ! diff -q "$W/_names" "$W/_listed" >/dev/null 2>&1; then
  fail "the manifest does not name exactly what the Makefile builds for $VER_NOV:
$(diff "$W/_names" "$W/_listed")"
fi
printf '  assets: %s, the same set the Makefile builds\n' "$(awk 'NF==2' "$MAN" | wc -l | tr -d ' ')"

echo "--- 2. homebrew tap ---"
# One level up: the tap and bucket are siblings of this repository, so a
# checkout of all three has them beside it. This path is not exercised by the
# fixture test, which always passes CTXPACK_TAP, so it is asserted directly
# below against the real layout.
TAP="${CTXPACK_TAP:-$ROOT/../homebrew-tap}"
if [ -d "$TAP" ]; then
  F="$TAP/Formula/ctxpack.rb"
  [ -f "$F" ] || fail "no Formula/ctxpack.rb in $TAP"
  fver="$(sed -n 's/.*version "\(.*\)".*/\1/p' "$F" | head -1)"
  [ "$fver" = "$VER_NOV" ] \
    || fail "the formula is at $fver but the repository is at $VER_NOV"
  fwant="$(sed -n 's/.*sha256 "\(.*\)".*/\1/p' "$F" | head -1)"
  furl="$(sed -n 's/.*url "\(.*\)".*/\1/p' "$F" | head -1)"
  printf '  version %s, url %s\n' "$fver" "$furl"
  if dl "$furl" "$W/src.tar.gz" 2>/dev/null && [ -s "$W/src.tar.gz" ]; then
    got="$(sha "$W/src.tar.gz")"
    [ "$got" = "$fwant" ] || fail "the formula's sha256 no longer matches its archive;
brew install would fail
  formula: $fwant
  actual : $got"
    printf '  sha256 matches the published archive\n'
  else
    printf '  SKIP re-hash - the archive at that url could not be fetched\n'
  fi
else
  printf '  SKIP - no tap at %s (set CTXPACK_TAP)\n' "$TAP"
fi

echo "--- 3. scoop bucket ---"
BUCK="${CTXPACK_BUCKET:-$ROOT/../scoop-bucket}"
if [ -d "$BUCK" ]; then
  BJSON="$(find "$BUCK" -name 'ctxpack.json' | head -1)"
  [ -n "$BJSON" ] && [ -f "$BJSON" ] || fail "no ctxpack.json under $BUCK"
  bver="$(grep -m1 '"version"' "$BJSON" | sed 's/.*"\([0-9][^"]*\)".*/\1/')"
  [ "$bver" = "$VER_NOV" ] \
    || fail "the bucket is at $bver but the repository is at $VER_NOV"
  grep -qF '$baseurl/SHA256SUMS.txt' "$BJSON" \
    || fail "the bucket no longer reads its hashes from the manifest"
  printf '  version %s, autoupdate hashes from the manifest\n' "$bver"
  # Pair each download url with the hash that follows it. No jq: this awk
  # walks the file and emits "url hash" when it sees the two keys in order.
  # The scheme is not part of the match - a release asset is identified by its
  # /releases/download/ path - so a mirror or a file:// fixture is handled too.
  # The trailing comma is optional: JSON does not require one on the last
  # member of an object, and a missing comma leaves the closing quote behind.
  awk '
    /"url"[ \t]*:[ \t]*"/ {
      m = $0; sub(/^.*"url"[ \t]*:[ \t]*"/, "", m); sub(/"[ \t]*,?$/, "", m);
      if (m ~ /\/releases\/download\//) u = m }
    /"hash"[ \t]*:[ \t]*"/ {
      m = $0; sub(/^.*"hash"[ \t]*:[ \t]*"/, "", m); sub(/"[ \t]*,?$/, "", m);
      if (u != "") print u " " m; u = "" }' \
    "$BJSON" > "$W/_pairs"
  [ -s "$W/_pairs" ] || fail "no release download url in the bucket"
  n=0
  while read -r u h; do
    n=$((n + 1))
    f="${u##*/}"
    listed="$(awk -v a="$f" '$NF == a {print $1}' "$MAN")"
    [ -n "$listed" ] \
      || fail "the bucket points at $f, which the manifest for $TAG does not list"
    [ "$h" = "$listed" ] || fail "the bucket hash for $f disagrees with the manifest:
  bucket  : $h
  manifest: $listed"
  done < "$W/_pairs"
  printf '  %s asset hash(es) all match the manifest\n' "$n"
else
  printf '  SKIP - no bucket at %s (set CTXPACK_BUCKET)\n' "$BUCK"
fi

echo "--- 4. install.sh ---"
if [ "$DO_INSTALL" -eq 1 ]; then
  DEST="$(mktemp -d)"
  if CTXPACK_INSTALL_DIR="$DEST" bash install.sh "$TAG" >/dev/null 2>&1; then
    BIN="$DEST/ctxpack"; [ -x "$BIN.exe" ] && BIN="$DEST/ctxpack.exe"
    [ -x "$BIN" ] || fail "install.sh exited 0 but installed nothing"
    out="$("$BIN" version 2>&1 | head -1)"
    printf '%s' "$out" | grep -q "$VER_NOV" \
      || fail "the installed binary does not report $VER_NOV: $out"
    printf '  %s\n' "$out"
  else
    fail "install.sh $TAG did not install"
  fi
else
  printf '  SKIP - pass --install to run it (downloads the binary)\n'
fi

echo
echo "check-release passed: $TAG matches the repository"
