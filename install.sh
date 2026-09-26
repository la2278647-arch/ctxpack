#!/usr/bin/env bash
#
# install.sh — download the right ctxpack binary for your platform from the
# latest GitHub release, verify it against the published SHA256SUMS.txt, and
# put it on PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash
#   # or pin a version:
#   curl -fsSL ... | bash -s -- v0.1.9
#
# Environment:
#   CTXPACK_INSTALL_DIR  override the install directory (default: /usr/local/bin
#                        when writable, else $HOME/.local/bin).
set -euo pipefail

OWNER="la2278647-arch"
REPO="ctxpack"

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
    # The REST API is precise but is rate-limited at 60 anonymous requests per
    # hour, which turns a routine install into a 403 on a busy machine. The
    # /releases/latest page redirects to /releases/tag/vX.Y.Z instead and is
    # not rate-limited at all, so it is the fallback.
    echo "Detecting latest release…" >&2
    # grep -o, not grep -m1: the API may answer on one line, and a whole
    # response line fed to a greedy sed can match inside the release body and
    # produce a "version" that is really a paragraph of release notes.
    VERSION="$(curl -fsSL --retry 3 -H 'User-Agent: ctxpack-installer' \
        "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
        | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
        | head -n 1 | sed -E 's/.*"([^"]+)"$/\1/' 2>/dev/null || true)"
    if [ -z "$VERSION" ]; then
        # || true: under "set -euo pipefail" a curl failure here would abort the
        # script before the friendly message below could be printed.
        LOC="$(curl -fsL -I "https://github.com/${OWNER}/${REPO}/releases/latest" \
            | tr -d '\r' | awk -F': ' 'tolower($1)=="location"{print $2; exit}' || true)"
        case "$LOC" in
            */releases/tag/*) VERSION="${LOC##*/tag/}" ;;
        esac
    fi
    if [ -z "$VERSION" ]; then
        echo "ctxpack: could not determine the latest release." >&2
        exit 1
    fi
fi
VER="${VERSION#v}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    linux*)  OS=linux ;;
    darwin*) OS=darwin ;;
    mingw*|msys*|cygwin*) OS=windows ;;
    *) echo "ctxpack: unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)        ARCH=amd64 ;;
    aarch64|arm64|armv8) ARCH=arm64 ;;
    i386|i686)           ARCH=386 ;;
    *) echo "ctxpack: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

ASSET="ctxpack_${VER}_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then ASSET="${ASSET}.exe"; fi
BASE="https://github.com/${OWNER}/${REPO}/releases/download/v${VER}"

INSTALL_DIR="${CTXPACK_INSTALL_DIR:-}"
if [ -z "$INSTALL_DIR" ]; then
    if [ -w /usr/local/bin ]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="${HOME}/.local/bin"
    fi
fi
mkdir -p "$INSTALL_DIR"

BIN="${INSTALL_DIR}/ctxpack"
[ "$OS" = "windows" ] && BIN="${BIN}.exe"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Download to a scratch directory and move into place only after the checksum
# is verified, so a truncated or tampered download cannot leave a bad binary
# sitting on PATH.
echo "Downloading ${BASE}/${ASSET}" >&2
curl -fsSL "${BASE}/${ASSET}" -o "${TMP}/ctxpack"
curl -fsSL "${BASE}/SHA256SUMS.txt" -o "${TMP}/SHA256SUMS.txt"

# A sha256sum line is "<hash> <marker><name>", where the marker is "*" in
# binary mode and a space in text mode. Which marker a host emits is
# platform-dependent: GNU coreutils 8.32 defaults to binary mode when given
# file arguments, so a checksum file produced on one machine is not guaranteed
# to parse on another. Accept both and compare the name after stripping the
# marker.
# || true: an awk that prints nothing would otherwise abort under "set -e" and
# skip the message below.
WANT="$(awk -v a="${ASSET}" '{ n = $NF; sub(/^\*/, "", n); if (n == a) { print $1; exit } }' "${TMP}/SHA256SUMS.txt" || true)"
if [ -z "$WANT" ]; then
    echo "ctxpack: ${ASSET} is not listed in SHA256SUMS.txt" >&2
    exit 1
fi

# sha256sum is GNU coreutils (Linux); shasum is BSD (macOS).
if command -v sha256sum >/dev/null 2>&1; then
    GOT="$(sha256sum "${TMP}/ctxpack" | cut -d' ' -f1)"
elif command -v shasum >/dev/null 2>&1; then
    GOT="$(shasum -a 256 "${TMP}/ctxpack" | cut -d' ' -f1)"
else
    echo "ctxpack: need sha256sum or shasum to verify the download" >&2
    exit 1
fi
if [ "$GOT" != "$WANT" ]; then
    echo "ctxpack: SHA256 mismatch for ${ASSET}" >&2
    echo "  expected ${WANT}" >&2
    echo "  got      ${GOT}" >&2
    exit 1
fi
echo "SHA256 verified: ${GOT}" >&2

chmod +x "${TMP}/ctxpack"
mv "${TMP}/ctxpack" "$BIN"

echo "Installed ctxpack v${VER} → ${BIN}" >&2
if command -v "$BIN" >/dev/null 2>&1; then
    "$BIN" version
else
    echo "Note: ${INSTALL_DIR} is not on your PATH. Add it:" >&2
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\"" >&2
fi
