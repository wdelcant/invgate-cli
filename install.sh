#!/usr/bin/env bash
set -euo pipefail

# invgate-cli installer — one-liner for any OS
# Usage: curl -fsSL https://raw.githubusercontent.com/wdelcant/invgate-cli/main/install.sh | bash

REPO="wdelcant/invgate-cli"
VERSION="${INVGATE_VERSION:-latest}"
BIN="invgate-cli"

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  darwin)  OS_NAME="macOS" ;;
  linux)   OS_NAME="Linux" ;;
  *)       echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH_NAME="amd64" ;;
  arm64|aarch64) ARCH_NAME="arm64" ;;
  *)            echo "Unsupported arch: $ARCH"; exit 1 ;;
esac

if [ "$OS" = "darwin" ]; then
  EXT="tar.gz"
elif [ "$OS" = "linux" ]; then
  EXT="tar.gz"
fi

ASSET="${BIN}_${VERSION}_${OS_NAME}_${ARCH_NAME}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

echo "Downloading ${BIN} ${VERSION} for ${OS_NAME}/${ARCH_NAME}..."
TMP_DIR=$(mktemp -d)
curl -fsSL "$URL" -o "${TMP_DIR}/${ASSET}"

if [ "$EXT" = "tar.gz" ]; then
  tar -xzf "${TMP_DIR}/${ASSET}" -C "${TMP_DIR}"
fi

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMP_DIR}/${BIN}" "${INSTALL_DIR}/${BIN}"
else
  mv "${TMP_DIR}/${BIN}" "${INSTALL_DIR}/${BIN}"
fi

chmod +x "${INSTALL_DIR}/${BIN}"
rm -rf "$TMP_DIR"

echo ""
echo "✓ ${BIN} ${VERSION} installed to ${INSTALL_DIR}/${BIN}"
echo "  Run '${BIN} --help' to get started."
