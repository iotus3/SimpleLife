#!/usr/bin/env bash
#
# SimpleLife 桌面打包（Windows 首发）。
#
# 产物：
#   dist/simplelife-<version>-windows-amd64.zip   压缩包里就是 simplelife.exe
#
# 用法: bash scripts/build.sh [--clean] [其它 viva 参数]
#
# 选项:
#   --clean   打包前清空 dist/，其余参数原样交给 `viva package`
#   -h        显示本帮助
#
# 这个脚本不再自己链接、自己写资源段：那些都是 `viva package` 的事。它从 viva.toml
# 读版本号与图标，链接参数（-s -w -H=windowsgui）由框架决定，图标与版本资源写进 PE
# 的中段也由框架完成；这里只负责选目标、挑干净的输出目录，以及把结果说清楚。
#
# 版本号只有一处真相：viva.toml 的 [app] version。打包前先把 Go 常量
# （internal/buildinfo/version.go）按它重新生成一遍，于是"忘了改第二处"这件事
# 连发生的机会都没有；`just test` 里另有一条测试盯着两者相等。
#
# 纯 Go：CGO_ENABLED=0（框架对 Windows/Linux 目标默认关闭 cgo）。Viva 的 Windows
# 后端通过 purego 直接调用系统 DLL，SQLite 驱动是纯 Go 的 modernc.org/sqlite，
# 因此不需要 C 编译器。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

CLEAN=0
ARGS=()
for argument in "$@"; do
    case "$argument" in
    --clean) CLEAN=1 ;;
    -h | --help)
        sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'
        exit 0
        ;;
    *) ARGS+=("$argument") ;;
    esac
done

if [ "$CLEAN" -eq 1 ]; then
    echo "▶ 清理 dist/"
    rm -rf dist
fi
mkdir -p dist

# 用 pin 在 go.mod 里的那个框架版本建 CLI：命令行与程序本身出自同一份源码，
# 不会出现"用新 CLI 打包旧框架程序"这种没人解释得清的组合。@version 形式在
# 临时模块里构建，不会把 CLI 自己的依赖写进本仓库的 go.sum。
VIVA_PACKAGE="github.com/vivaui/viva/cmd/viva"
VIVA_VERSION="$(go list -m -f '{{.Version}}' github.com/vivaui/viva)"

# 版本常量跟着 viva.toml 走：打包用的版本号与程序内的版本号因此不可能不一致。
bash scripts/version.sh

echo "▶ 打包 simplelife（viva $VIVA_VERSION）"
go run "$VIVA_PACKAGE@$VIVA_VERSION" package \
    --target windows --arch amd64 --format zip "${ARGS[@]+"${ARGS[@]}"}"

# 产物名由框架按 viva.toml 决定：<name>-<version>-<platform>-<arch>.zip。这里只是
# 把名字读回来打印，找不到就说明打包没有按预期落盘，直接失败而不是报"成功"。
archive="$(ls -t dist/simplelife-*-windows-amd64.zip 2>/dev/null | head -1)"
if [ -z "$archive" ]; then
    echo "✗ 打包结束但 dist/ 里没有 simplelife-*-windows-amd64.zip" >&2
    exit 1
fi
echo "✓ 打包完成：$archive"
