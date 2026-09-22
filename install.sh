#!/usr/bin/env bash
#
# install.sh — download the right ctxpack binary for your platform from the
# latest GitHub release and put it on PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.sh | bash
#   # or pin a version:
#   curl -fsSL ... | bash -s -- v0.1.2
#
# Environment:
#   CTXPACK_INSTALL_DIR  override the install directory (default: /usr/local/bin
#                        when writable, else $HOME/.local/bin).
set -euo pipefail

OWNER="la2278647-arch"
REPO="ctxpack"

if [ -n "${BASH_EXE:-}" ]; then :; fi

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
    echo "Detecting latest release…" >&2
    VERSION="$(curl -fsSL "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
        | grep -m1 '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')"
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
URL="https://github.com/${OWNER}/${REPO}/releases/download/v${VER}/${ASSET}"

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

echo "Downloading ${URL}" >&2
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$URL" -o "${TMP}/ctxpack-download"
chmod +x "${TMP}/ctxpack-download"
mv "${TMP}/ctxpack-download" "$BIN"

echo "Installed ctxpack v${VER} → ${BIN}" >&2
if command -v "$BIN" >/dev/null 2>&1; then
    "$BIN" version
else
    echo "Note: ${INSTALL_DIR} is not on your PATH. Add it:" >&2
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\"" >&2
fi
