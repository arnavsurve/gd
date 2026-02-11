#!/bin/sh
set -e

cd "$(dirname "$0")"
go build -o gd .
echo "Built gd binary at $(pwd)/gd"
echo ""
echo "Add this to your shell config:"
echo "  alias gd=\"$HOME/dev/gd/gd\""
