#!/usr/bin/env bash
#
# 把 internal/presentation/icons/svg/*.svg 编译成 icons.go（gfx.Icon 值）。
#
# 用法: bash scripts/icons.sh
#
# 为什么不在本仓库里直接 go run：Viva 的图标编译器所在包依赖 bsdiff 等工具链
# 依赖，直接跑会把它们写进本项目的 go.sum。这里在临时模块里跑，生成的文件只
# 依赖 gfx，仓库的依赖保持不变。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VIVA_VERSION="$(awk '/github.com\/vivaui\/viva/ { print $2; exit }' go.mod)"
if [ -z "$VIVA_VERSION" ]; then
    echo "✗ go.mod 里没有 vivaui/viva，无法确定编译器版本" >&2
    exit 1
fi

TOOL_DIR="$(mktemp -d)"
trap 'rm -rf "$TOOL_DIR"' EXIT

cat >"$TOOL_DIR/go.mod" <<EOF
module icons-tool

go 1.27.0
EOF

echo "▶ 编译图标（viva $VIVA_VERSION）"
(
    cd "$TOOL_DIR"
    GOFLAGS=-mod=mod go get "github.com/vivaui/viva@$VIVA_VERSION" >/dev/null
    GOFLAGS=-mod=mod go run github.com/vivaui/viva/cmd/viva icons \
        -o "$ROOT_DIR/internal/presentation/icons/icons.go" \
        -pkg icons \
        "$ROOT_DIR/internal/presentation/icons/svg"
)
echo "✓ internal/presentation/icons/icons.go 已更新"
