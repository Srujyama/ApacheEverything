#!/usr/bin/env bash
# Sunny installer.
#
# Detects OS/arch, downloads the latest GitHub release, installs to
# /usr/local/bin (override with INSTALL_DIR=...). Set VERSION=v0.1.0 to
# pin a specific release; defaults to latest.
#
# Usage:
#   curl -fsSL https://get.sunny.dev/install.sh | sh
# Or for testing locally:
#   ./scripts/install.sh
set -euo pipefail

REPO="${SUNNY_REPO:-sunny/sunny}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -1 | cut -d'"' -f4)"
  if [ -z "$VERSION" ]; then
    echo "couldn't resolve latest version; pin with VERSION=vX.Y.Z" >&2
    exit 1
  fi
fi

ASSET="sunny_${VERSION#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"

echo "Installing sunny $VERSION ($OS/$ARCH) from $URL"

TMP="$(mktemp -d)"
trap "rm -rf $TMP" EXIT

curl -fsSL "$URL" -o "$TMP/sunny.tar.gz"
tar -xzf "$TMP/sunny.tar.gz" -C "$TMP"

if [ ! -w "$INSTALL_DIR" ]; then
  echo "$INSTALL_DIR not writable; using sudo"
  sudo mv "$TMP/sunny" "$INSTALL_DIR/sunny"
  if [ -f "$TMP/sunny-cli" ]; then sudo mv "$TMP/sunny-cli" "$INSTALL_DIR/sunny-cli"; fi
else
  mv "$TMP/sunny" "$INSTALL_DIR/sunny"
  if [ -f "$TMP/sunny-cli" ]; then mv "$TMP/sunny-cli" "$INSTALL_DIR/sunny-cli"; fi
fi

echo
echo "Installed: $INSTALL_DIR/sunny"
echo "Quick start:"
echo "  sunny                       # serves on :3000 with default connectors"
echo "  sunny-cli hash-password X   # generate SUNNY_PASSWORD_HASH"
echo "  sunny-cli backup data/ snapshot.tar.gz"
