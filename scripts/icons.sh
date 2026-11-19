#!/usr/bin/env bash
#
# Compile internal/presentation/icons/svg/*.svg into icons.go (gfx.Icon values).
#
# Usage: bash scripts/icons.sh
#
# The compiler is run in a temporary module: the Viva package that carries it also pulls
# in its own toolchain dependencies, which have no business in this repository's go.sum.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VIVA_VERSION="$(awk '/github.com\/vivaui\/viva/ { print $2; exit }' go.mod)"
if [ -z "$VIVA_VERSION" ]; then
    echo "✗ go.mod names no vivaui/viva, so the compiler version is unknown" >&2
    exit 1
fi

TOOL_DIR="$(mktemp -d)"
trap 'rm -rf "$TOOL_DIR"' EXIT

cat >"$TOOL_DIR/go.mod" <<EOF
module icons-tool

go 1.27.0
EOF

echo "▶ compiling icons (viva $VIVA_VERSION)"
(
    cd "$TOOL_DIR"
    GOFLAGS=-mod=mod go get "github.com/vivaui/viva@$VIVA_VERSION" >/dev/null
    GOFLAGS=-mod=mod go run github.com/vivaui/viva/cmd/viva icons \
        -o "$ROOT_DIR/internal/presentation/icons/icons.go" \
        -pkg icons \
        "$ROOT_DIR/internal/presentation/icons/svg"
)
echo "✓ internal/presentation/icons/icons.go updated"
