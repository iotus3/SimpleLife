#!/usr/bin/env bash
#
# Remove build output. Sources and databases are untouched.
#
# Usage: bash scripts/clean.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

for dir in dist build; do
    if [ -d "$dir" ]; then
        echo "  · removing $dir/"
        rm -rf "$dir"
    fi
done

echo "✓ clean (dist/, build/)"
