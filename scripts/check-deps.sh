#!/usr/bin/env bash
#
# Layer guard: a package that imports upward fails this check.
#
# Rules:
#   1. domain          never imports application / infrastructure / presentation / bootstrap / viva
#   2. application     never imports infrastructure / presentation / bootstrap / viva
#   3. infrastructure  never imports presentation / bootstrap
#   4. presentation    never imports infrastructure / bootstrap
#                      (the export encoder arrives through presentation.Deps.Encode)
#   5. contracts       describes ports only, never an implementation layer
#
# Usage: bash scripts/check-deps.sh
# Exit code: 0 the direction holds; 1 violations, listed one by one.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODULE="github.com/iotus3/simplelife/app"
VIOLATIONS=0

# forbidden <package dir> <rule> <forbidden prefix...>
#
# One package at a time: an import that starts with a forbidden prefix is a violation.
forbidden() {
    local dir="$1" rule="$2"
    shift 2
    local imports import prefix
    if ! imports="$(go list -f '{{join .Imports "\n"}}' "./$dir" 2>&1)"; then
        printf '  ✗ %s: cannot list its imports\n      %s\n' "$dir" "$imports"
        VIOLATIONS=$((VIOLATIONS + 1))
        return
    fi
    while IFS= read -r import; do
        [ -z "$import" ] && continue
        for prefix in "$@"; do
            case "$import" in
            "$prefix"*)
                printf '  ✗ %s imports %s\n      %s\n' "$dir" "$import" "$rule"
                VIOLATIONS=$((VIOLATIONS + 1))
                ;;
            esac
        done
    done <<<"$imports"
}

# find_dirs <find arguments...> lists the folders that hold Go files; testdata and
# vendor are not packages.
find_dirs() {
    find "$@" -type d \( -name testdata -o -name vendor \) -prune -o -type d -print |
        sort |
        while IFS= read -r dir; do
            if compgen -G "$dir/*.go" >/dev/null; then
                echo "$dir"
            fi
        done
}

echo "▶ layer direction"

echo "  · domain"
for dir in $(find_dirs internal/domain -mindepth 1 -maxdepth 1); do
    forbidden "$dir" "the domain is pure Go: no framework, nothing above it" \
        "$MODULE/internal/application" "$MODULE/internal/infrastructure" \
        "$MODULE/internal/presentation" "$MODULE/internal/bootstrap" \
        "$MODULE/internal/contracts" "github.com/vivaui/viva"
done

echo "  · application"
for dir in $(find_dirs internal/application -mindepth 1 -maxdepth 1); do
    forbidden "$dir" "the application layer sees the domain and the ports, nothing else" \
        "$MODULE/internal/infrastructure" "$MODULE/internal/presentation" \
        "$MODULE/internal/bootstrap" "github.com/vivaui/viva"
done

echo "  · infrastructure"
for dir in $(find_dirs internal/infrastructure -mindepth 1 -maxdepth 2); do
    forbidden "$dir" "infrastructure implements the ports; it never imports the interface" \
        "$MODULE/internal/presentation" "$MODULE/internal/bootstrap"
done

echo "  · presentation"
for dir in $(find_dirs internal/presentation -mindepth 0 -maxdepth 1); do
    forbidden "$dir" "the interface reaches infrastructure only through Deps" \
        "$MODULE/internal/infrastructure" "$MODULE/internal/bootstrap"
done

echo "  · contracts"
for dir in $(find_dirs internal/contracts -maxdepth 0); do
    forbidden "$dir" "a port describes an interface and imports no implementation" \
        "$MODULE/internal/application" "$MODULE/internal/infrastructure" \
        "$MODULE/internal/presentation" "$MODULE/internal/bootstrap"
done

echo
if [ "$VIOLATIONS" -gt 0 ]; then
    echo "✗ $VIOLATIONS layer violations"
    exit 1
fi
echo "✓ layer direction holds"
