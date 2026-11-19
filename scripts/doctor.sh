#!/usr/bin/env bash
#
# 检查本机是否具备构建 / 运行 SimpleLife 的条件。
#
# 用法: bash scripts/doctor.sh
# 退出码: 0 全部满足；1 存在阻塞项（缺 Go、Go 版本过低、缺 bash）。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

FAILED=0
WARNED=0

ok()   { printf '  ✓ %s\n' "$1"; }
warn() { printf '  ! %s\n' "$1"; WARNED=$((WARNED + 1)); }
fail() { printf '  ✗ %s\n' "$1"; FAILED=$((FAILED + 1)); }

# required_version 从 go.mod 里取出 go 指令要求的最低版本，例如 1.27.0。
required_version() {
    awk '$1 == "go" { print $2; exit }' go.mod
}

# version_ge A B：A >= B 时返回 0。
version_ge() {
    [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}

echo "▶ 工具链"
if command -v go >/dev/null 2>&1; then
    go_version="$(go env GOVERSION | sed 's/^go//')"
    want="$(required_version)"
    if version_ge "$go_version" "$want"; then
        ok "go $go_version（go.mod 要求 ≥ $want）"
    else
        fail "go $go_version 低于 go.mod 要求的 $want"
    fi
    ok "CGO_ENABLED=$(go env CGO_ENABLED)（纯 Go：Viva 走 purego，SQLite 用 modernc）"
    if [ "$(go env GOOS)" = "windows" ]; then
        ok "GOOS=windows（首发平台）"
    else
        warn "GOOS=$(go env GOOS)，本版本只支持 Windows 桌面端"
    fi
else
    fail "找不到 go：请安装 Go 并使用 go.mod 要求的版本"
fi

if command -v just >/dev/null 2>&1; then
    ok "just $(just --version | awk '{print $2}')"
else
    warn "找不到 just：只能直接调用 scripts/*.sh"
fi

if command -v bash >/dev/null 2>&1; then
    ok "bash $BASH_VERSION"
else
    fail "找不到 bash：工程脚本需要 bash 4 以上"
fi

if command -v git >/dev/null 2>&1; then
    ok "git $(git --version | awk '{print $3}')"
else
    warn "找不到 git：版本号会退回 v0.0.0-dev"
fi

echo "▶ 源码树"
if [ -z "$(gofmt -l .)" ]; then
    ok "gofmt 无差异"
else
    warn "gofmt 有差异：$(gofmt -l . | tr '\n' ' ')"
fi
if go vet ./... >/dev/null 2>&1; then
    ok "go vet 通过"
else
    warn "go vet 有报告：运行 just vet 查看"
fi

echo "▶ 数据目录"
# 数据库跟着 exe 走：<exe 所在目录>/datas/simplelife.db。开发时 exe 在 build/，
# 发布包里在解压目录。
data_dir="$ROOT_DIR/build/datas"
if [ -f "$data_dir/simplelife.db" ]; then
    ok "$data_dir/simplelife.db（$(wc -c <"$data_dir/simplelife.db" | tr -d ' ') 字节）"
else
    ok "exe 旁的 datas/simplelife.db（首次运行时创建）"
fi

echo
if [ "$FAILED" -gt 0 ]; then
    echo "✗ $FAILED 个阻塞项，$WARNED 个提示项"
    exit 1
fi
echo "✓ 环境就绪（$WARNED 个提示项）"
