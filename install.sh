#!/bin/bash
# ApexClaw One-Line Installer
# curl https://raw.githubusercontent.com/amarnathcjd/apexclaw/master/install.sh | bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}🐾 Installing ApexClaw...${NC}"

OS=$(uname -s)
ARCH=$(uname -m)

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l) ARCH="arm" ;;
  *) echo -e "${RED}✗ Unsupported: $ARCH${NC}"; exit 1 ;;
esac

case "$OS" in
  Linux) BIN="apexclaw-linux-${ARCH}" ;;
  Darwin) BIN="apexclaw-macos-${ARCH}" ;;
  *) echo -e "${RED}✗ Unsupported: $OS${NC}"; exit 1 ;;
esac

DIR="${HOME}/.local/bin"
mkdir -p "$DIR"

echo "Downloading $BIN..."
curl -fL "https://github.com/amarnathcjd/apexclaw/releases/latest/download/${BIN}" -o "$DIR/apexclaw"
chmod +x "$DIR/apexclaw"

if [[ ":$PATH:" != *":$DIR:"* ]]; then
  RC="$HOME/.bashrc"
  [ -f "$HOME/.zshrc" ] && RC="$HOME/.zshrc"
  echo "" >> "$RC"
  echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> "$RC"
fi

echo -e "${GREEN}✓ Installed to $DIR/apexclaw${NC}"
echo ""
echo "Next: mkdir -p ~/.apexclaw && nano ~/.apexclaw/.env"
echo "Then: apexclaw"
