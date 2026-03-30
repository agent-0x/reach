#!/bin/bash
set -euo pipefail

REPO="agent-0x/reach"
INSTALL_DIR="/usr/local/bin"
BINARY="reach"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux)  ;;
  darwin) ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest release tag
echo "Fetching latest release..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')

if [ -z "$TAG" ]; then
  echo "Error: could not determine latest release"
  exit 1
fi

# Download and extract
URL="https://github.com/${REPO}/releases/download/v${TAG}/${BINARY}_${TAG}_${OS}_${ARCH}.tar.gz"
echo "Downloading reach v${TAG} for ${OS}/${ARCH}..."

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "${TMP}/reach.tar.gz"

# Verify checksum
CHECKSUM_URL="https://github.com/${REPO}/releases/download/v${TAG}/checksums.txt"
echo "Verifying checksum..."
curl -fsSL "$CHECKSUM_URL" -o "${TMP}/checksums.txt"
EXPECTED=$(grep "${BINARY}_${TAG}_${OS}_${ARCH}.tar.gz" "${TMP}/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "Warning: could not find checksum for this archive, skipping verification"
else
  ACTUAL=$(sha256sum "${TMP}/reach.tar.gz" 2>/dev/null || shasum -a 256 "${TMP}/reach.tar.gz" 2>/dev/null)
  ACTUAL=$(echo "$ACTUAL" | awk '{print $1}')
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "Error: checksum mismatch!"
    echo "  Expected: $EXPECTED"
    echo "  Got:      $ACTUAL"
    exit 1
  fi
  echo "Checksum OK."
fi

tar xzf "${TMP}/reach.tar.gz" -C "$TMP"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo ""
echo "reach v${TAG} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Quick start:"
echo "  reach agent init --dir /etc/reach-agent   # On your server"
echo "  reach add myserver --host <ip> --token <t> # On your machine"
echo "  reach exec myserver 'uname -a'             # Run commands"
