#!/usr/bin/env bash
#
# Check that this machine can build and run SimpleLife.
#
# Usage: bash scripts/doctor.sh
# Exit code: 0 everything is there; 1 something blocks a build (no Go, Go too old, no bash).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

FAILED=0
WARNED=0

ok()   { printf '  ✓ %s\n' "$1"; }
warn() { printf '  ! %s\n' "$1"; WARNED=$((WARNED + 1)); }
fail() { printf '  ✗ %s\n' "$1"; FAILED=$((FAILED + 1)); }

# required_version is the minimum the go directive in go.mod asks for, e.g. 1.27.0.
required_version() {
    awk '$1 == "go" { print $2; exit }' go.mod
}

# version_ge A B succeeds when A >= B.
version_ge() {
    [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}

echo "▶ toolchain"
if command -v go >/dev/null 2>&1; then
    go_version="$(go env GOVERSION | sed 's/^go//')"
    want="$(required_version)"
    if version_ge "$go_version" "$want"; then
        ok "go $go_version (go.mod wants >= $want)"
    else
        fail "go $go_version is older than the $want go.mod asks for"
    fi
    ok "CGO_ENABLED=$(go env CGO_ENABLED) (pure Go: Viva through purego, SQLite through modernc)"
    if [ "$(go env GOOS)" = "windows" ]; then
        ok "GOOS=windows (the release platform)"
    else
        warn "GOOS=$(go env GOOS); this version targets the Windows desktop only"
    fi
else
    fail "no go: install the version go.mod asks for"
fi

if command -v just >/dev/null 2>&1; then
    ok "just $(just --version | awk '{print $2}')"
else
    warn "no just: scripts/*.sh can be run directly"
fi

if command -v bash >/dev/null 2>&1; then
    ok "bash $BASH_VERSION"
else
    fail "no bash: the scripts need bash 4 or later"
fi

if command -v git >/dev/null 2>&1; then
    ok "git $(git --version | awk '{print $3}')"
else
    warn "no git: the about page will show no commit"
fi

echo "▶ source tree"
if [ -z "$(gofmt -l .)" ]; then
    ok "gofmt reports nothing"
else
    warn "gofmt wants to change: $(gofmt -l . | tr '\n' ' ')"
fi
if go vet ./... >/dev/null 2>&1; then
    ok "go vet is clean"
else
    warn "go vet has something to say: run just vet"
fi

echo "▶ data folder"
# The database travels with the program: <exe folder>/datas/simplelife.db. During
# development the program is in build/, in a release it is wherever the zip was unpacked.
data_dir="$ROOT_DIR/build/datas"
if [ -f "$data_dir/simplelife.db" ]; then
    ok "$data_dir/simplelife.db ($(wc -c <"$data_dir/simplelife.db" | tr -d ' ') bytes)"
else
    ok "datas/simplelife.db beside the program (created on first run)"
fi

echo
if [ "$FAILED" -gt 0 ]; then
    echo "✗ $FAILED blocking, $WARNED advisory"
    exit 1
fi
echo "✓ ready ($WARNED advisory)"
