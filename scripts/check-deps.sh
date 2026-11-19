#!/usr/bin/env bash
#
# 依赖方向守卫（设计文档 §4.1 / §8.4）：违反分层红线即失败。
#
# 规则：
#   1. domain         不得依赖 application / infrastructure / presentation / bootstrap / viva
#   2. application    不得依赖 infrastructure / presentation / bootstrap / viva
#   3. infrastructure 不得依赖 presentation / bootstrap
#   4. presentation   不得依赖 infrastructure / bootstrap
#      （导出编码器经 presentation.Deps.Encode 注入，见 §7.4）
#   5. contracts      只描述端口，不得依赖任何实现层
#
# 用法: bash scripts/check-deps.sh
# 退出码: 0 合规；1 有违规（逐条列出）。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODULE="github.com/iotus3/simplelife/app"
VIOLATIONS=0

# forbidden <包目录> <规则说明> <禁止前缀...>
#
# 逐个包检查直接 import：路径以某个禁止前缀开头即记一次违规。
forbidden() {
    local dir="$1" rule="$2"
    shift 2
    local imports import prefix
    if ! imports="$(go list -f '{{join .Imports "\n"}}' "./$dir" 2>&1)"; then
        printf '  ✗ %s: 无法列出依赖\n      %s\n' "$dir" "$imports"
        VIOLATIONS=$((VIOLATIONS + 1))
        return
    fi
    while IFS= read -r import; do
        [ -z "$import" ] && continue
        for prefix in "$@"; do
            case "$import" in
            "$prefix"*)
                printf '  ✗ %s 依赖 %s\n      %s\n' "$dir" "$import" "$rule"
                VIOLATIONS=$((VIOLATIONS + 1))
                ;;
            esac
        done
    done <<<"$imports"
}

# find_dirs <find 参数...>：列出含 Go 文件的包目录（testdata 与纯目录不算包）。
find_dirs() {
    find "$@" -type d \( -name testdata -o -name vendor \) -prune -o -type d -print |
        sort |
        while IFS= read -r dir; do
            if compgen -G "$dir/*.go" >/dev/null; then
                echo "$dir"
            fi
        done
}

echo "▶ 分层依赖方向"

echo "  · domain"
for dir in $(find_dirs internal/domain -mindepth 1 -maxdepth 1); do
    forbidden "$dir" "领域层必须是纯 Go：不依赖框架，也不依赖上层" \
        "$MODULE/internal/application" "$MODULE/internal/infrastructure" \
        "$MODULE/internal/presentation" "$MODULE/internal/bootstrap" \
        "$MODULE/internal/contracts" "github.com/vivaui/viva"
done

echo "  · application"
for dir in $(find_dirs internal/application -mindepth 1 -maxdepth 1); do
    forbidden "$dir" "应用层只依赖 domain 与端口" \
        "$MODULE/internal/infrastructure" "$MODULE/internal/presentation" \
        "$MODULE/internal/bootstrap" "github.com/vivaui/viva"
done

echo "  · infrastructure"
for dir in $(find_dirs internal/infrastructure -mindepth 1 -maxdepth 2); do
    forbidden "$dir" "基础设施层实现端口，不反过来依赖界面" \
        "$MODULE/internal/presentation" "$MODULE/internal/bootstrap"
done

echo "  · presentation"
for dir in $(find_dirs internal/presentation -mindepth 0 -maxdepth 1); do
    forbidden "$dir" "表现层不直连基础设施：编码器经 Deps 注入" \
        "$MODULE/internal/infrastructure" "$MODULE/internal/bootstrap"
done

echo "  · contracts"
for dir in $(find_dirs internal/contracts -maxdepth 0); do
    forbidden "$dir" "端口契约只描述接口，不引入任何实现" \
        "$MODULE/internal/application" "$MODULE/internal/infrastructure" \
        "$MODULE/internal/presentation" "$MODULE/internal/bootstrap"
done

echo
if [ "$VIOLATIONS" -gt 0 ]; then
    echo "✗ 发现 $VIOLATIONS 处依赖方向违规"
    exit 1
fi
echo "✓ 依赖方向合规"
