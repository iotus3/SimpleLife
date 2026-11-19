#!/usr/bin/env bash
#
# 清理构建产物。源码与数据库不受影响。
#
# 用法: bash scripts/clean.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

for dir in dist build; do
    if [ -d "$dir" ]; then
        echo "  · 删除 $dir/"
        rm -rf "$dir"
    fi
done

echo "✓ 已清理（dist/、build/）"
