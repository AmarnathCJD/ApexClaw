#!/bin/bash
# ApexClaw One-Line Installer
# curl https://raw.githubusercontent.com/amarnathcjd/apexclaw/master/install.sh | bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}"
cat << "EOF"
╔════════════════════════════════════════════╗
║         🐾 ApexClaw Installer              ║
║    A Telegram AI Assistant with 94 Tools   ║
╚════════════════════════════════════════════╝
EOF
echo -e "${NC}"

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

# Try /usr/local/bin first, fallback to ~/.local/bin
DIR="/usr/local/bin"
if [ ! -w "$DIR" ]; then
  DIR="${HOME}/.local/bin"
  mkdir -p "$DIR"
fi

echo -e "${BLUE}Installing for $OS ($ARCH)${NC}"
echo "Downloading $BIN..."

curl -fL "https://github.com/amarnathcjd/apexclaw/releases/latest/download/${BIN}" -o "$DIR/apexclaw"
chmod +x "$DIR/apexclaw"

# Add to PATH if in ~/.local/bin
if [ "$DIR" = "${HOME}/.local/bin" ] && [[ ":$PATH:" != *":$DIR:"* ]]; then
  RC="$HOME/.bashrc"
  [ -f "$HOME/.zshrc" ] && RC="$HOME/.zshrc"

  if ! grep -q "export PATH.*local/bin" "$RC" 2>/dev/null; then
    echo "" >> "$RC"
    echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> "$RC"
  fi

  export PATH="$DIR:$PATH"
fi

echo -e "${GREEN}✓ Installed to $DIR/apexclaw${NC}"
echo ""
echo -e "${GREEN}✓ Ready to run!${NC}"
echo ""
echo -e "${BLUE}Just run:${NC}"
echo -e "  ${GREEN}apexclaw${NC}"
echo ""
echo "First run will ask for Telegram credentials"
echo ""
