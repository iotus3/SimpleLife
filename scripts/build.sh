#!/usr/bin/env bash
#
# Package SimpleLife for Windows.
#
# Output: dist/simplelife-<version>-windows-amd64.zip, holding simplelife.exe.
#
# Usage: bash scripts/build.sh [--clean] [more viva flags]
#
#   --clean   remove dist/ first
#   -h        print this help
#
# Linking, the icon, the version resource and the archive name all belong to `viva
# package`; this script only picks the target, keeps the output folder clean and says
# what it produced. The version comes from viva.toml, and the Go constant is regenerated
# from it first, so the program and the package can never disagree.
#
# Pure Go: CGO_ENABLED=0. Viva's Windows backend calls into the system through purego and
# SQLite is modernc.org/sqlite, so no C compiler is involved.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

help() {
    # Every leading comment line, minus the shebang.
    awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
}

CLEAN=0
ARGS=()
for argument in "$@"; do
    case "$argument" in
    --clean) CLEAN=1 ;;
    -h | --help)
        help
        exit 0
        ;;
    *) ARGS+=("$argument") ;;
    esac
done

if [ "$CLEAN" -eq 1 ]; then
    echo "▶ clearing dist/"
    rm -rf dist
fi
mkdir -p dist

# The CLI is built from the framework version pinned in go.mod: the command line and the
# program come from the same source. The @version form builds it in a temporary module,
# so its own dependencies never reach this repository's go.sum.
VIVA_PACKAGE="github.com/vivaui/viva/cmd/viva"
VIVA_VERSION="$(go list -m -f '{{.Version}}' github.com/vivaui/viva)"

# The version constant follows viva.toml before anything is packaged.
bash scripts/version.sh

echo "▶ packaging simplelife (viva $VIVA_VERSION)"
go run "$VIVA_PACKAGE@$VIVA_VERSION" package \
    --target windows --arch amd64 --format zip "${ARGS[@]+"${ARGS[@]}"}"

# The framework names the archive from viva.toml. Reading it back is the check that it
# really landed: a missing file fails here instead of being reported as success.
archive="$(ls -t dist/simplelife-*-windows-amd64.zip 2>/dev/null | head -1)"
if [ -z "$archive" ]; then
    echo "✗ packaging finished but dist/ holds no simplelife-*-windows-amd64.zip" >&2
    exit 1
fi
echo "✓ packaged: $archive"
