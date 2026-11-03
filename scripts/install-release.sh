#!/usr/bin/env bash
# Install a claude-sync release binary from this fork's GitHub Releases (Linux only).
#
# Usage: install-release.sh <version> [install-dir]
#   version      release tag, e.g. v2.0.1
#   install-dir  where to put claude-sync (default: ~/.local/bin)
#
# Example:
#   curl -fsSL https://raw.githubusercontent.com/gj-quangna/f-claude-sync/me/scripts/install-release.sh \
#     | bash -s -- v2.0.1
set -euo pipefail

REPO="gj-quangna/f-claude-sync"

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
  echo "Usage: $0 <version> [install-dir]" >&2
  exit 2
fi

VERSION="$1"
INSTALL_DIR="${2:-$HOME/.local/bin}"
case "$VERSION" in
  v*) ;;
  *) VERSION="v$VERSION" ;;
esac

if [ "$(uname -s)" != "Linux" ]; then
  echo "error: only Linux binaries are released (got $(uname -s))" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *)
    echo "error: unsupported architecture $(uname -m)" >&2
    exit 1
    ;;
esac

BIN="claude-sync-linux-$ARCH"
BASE="https://github.com/$REPO/releases/download/$VERSION"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $BIN $VERSION ..."
curl -fsSL -o "$TMP/$BIN" "$BASE/$BIN"
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt"

echo "Verifying checksum ..."
(cd "$TMP" && grep " $BIN\$" checksums.txt | sha256sum -c -)

mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMP/$BIN" "$INSTALL_DIR/claude-sync"
echo "Installed: $("$INSTALL_DIR/claude-sync" --version) -> $INSTALL_DIR/claude-sync"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "note: $INSTALL_DIR is not in PATH; add: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
