#!/usr/bin/env bash
# Download the pre-built greenagonia binary for this platform from GitHub Releases.
# Usage: ./get-binary.sh
# Result: ./greenagonia (executable, ready to run)
set -euo pipefail

REPO="justynroberts/greenagonia"

# Detect OS and architecture
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Darwin)
    case "$ARCH" in
      arm64)  ASSET="greenagonia-darwin-arm64" ;;
      x86_64) ASSET="greenagonia-darwin-amd64" ;;
      *)      echo "error: unsupported macOS arch: $ARCH" >&2; exit 1 ;;
    esac
    ;;
  Linux)
    case "$ARCH" in
      x86_64)          ASSET="greenagonia-linux-amd64" ;;
      aarch64 | arm64) ASSET="greenagonia-linux-arm64" ;;
      *)               echo "error: unsupported Linux arch: $ARCH" >&2; exit 1 ;;
    esac
    ;;
  *)
    echo "error: unsupported OS: $OS" >&2
    exit 1
    ;;
esac

DEST="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/greenagonia"

echo "  platform : $OS/$ARCH"
echo "  asset    : $ASSET"
echo "  dest     : $DEST"
echo ""

# Find latest release
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

if [[ -z "$LATEST" ]]; then
  echo "error: no releases found on ${REPO}" >&2
  echo "       Build from source with: make build" >&2
  exit 1
fi

echo "  release  : $LATEST"
echo ""

URL="https://github.com/${REPO}/releases/download/${LATEST}/${ASSET}"
echo "downloading..."
curl -fsSL "$URL" -o "$DEST"
chmod +x "$DEST"

echo "done. run ./greenagonia --help to confirm."
