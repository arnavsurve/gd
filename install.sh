#!/bin/sh
set -e

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

cd "$(dirname "$0")"
go build -o gd .

mkdir -p "$INSTALL_DIR"
mv gd "$INSTALL_DIR/gd"

echo "Installed gd to $INSTALL_DIR/gd"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Add $INSTALL_DIR to your PATH if it isn't already:"; echo "  export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
