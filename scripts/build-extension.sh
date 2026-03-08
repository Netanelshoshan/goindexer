#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$REPO_ROOT/bin"
GO_DIR="$REPO_ROOT/go"

mkdir -p "$BIN_DIR"
cd "$GO_DIR"
go build -o "$BIN_DIR/goindexer" ./cmd/goindexer
echo "Built $BIN_DIR/goindexer"
