# Engineering tasks for SimpleLife.
# Every recipe forwards to scripts/*.sh: the logic lives there, so it can be run alone.

set positional-arguments := true   # Arguments reach the shell as positional parameters, never spliced into a command
set shell := ["bash", "-euo", "pipefail", "-c"]

# List the tasks
default:
    @just --list

# ── Environment ──────────────────────────────────────────────────────

# Check this machine can build the program (Go, pure-Go build, bash, just, git, data folder)
doctor:
    #!/usr/bin/env bash
    set -euo pipefail
    bash scripts/doctor.sh

# ── Development ──────────────────────────────────────────────────────

# Build into build/ and run it (the database lands in build/datas/simplelife.db)
run:
    #!/usr/bin/env bash
    set -euo pipefail
    go build -o build/simplelife.exe ./cmd/simplelife
    ./build/simplelife.exe

# Print the version (one source of truth: [app] version in viva.toml)
version:
    @bash scripts/version.sh --show

# Release: set the version in one place, the Go constant is generated from it
update-version v:
    @bash scripts/version.sh --set "{{v}}"

# Fail when the generated file is behind viva.toml; `just check` runs this first
version-check:
    @bash scripts/version.sh --check

# Rebuild internal/presentation/icons/icons.go from icons/svg/*.svg
icons:
    @bash scripts/icons.sh

# ── Gates ────────────────────────────────────────────────────────────

# Format and tidy
fmt:
    @gofmt -l -w . && go mod tidy

# Check formatting without writing
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    out="$(gofmt -l .)"
    if [ -n "$out" ]; then
        echo "✗ files need gofmt:"
        echo "$out"
        exit 1
    fi
    echo "✓ formatting is clean"

# go vet
vet:
    @go vet ./...

# Layer guard: a package that imports upward fails the build
depcheck:
    @bash scripts/check-deps.sh

# Every unit test
test:
    @go test ./... -count=1

# With coverage, written to build/coverage.html
cover:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p build
    go test ./... -count=1 -coverprofile=build/coverage.out
    go tool cover -html=build/coverage.out -o build/coverage.html
    echo "▶ total coverage: $(go tool cover -func=build/coverage.out | tail -1)"

# Interface tests only (Viva headless, no window)
test-ui:
    @go test ./internal/presentation/... -count=1 -v

# End to end: the real program and a real database, every page clicked, screenshots in build/smoke/
smoke:
    @bash scripts/smoke.sh

# What a commit must pass: the generated version is checked before any compilation
check: fmt-check version-check vet depcheck test
    @echo "✓ check passed"

# Single-instance guard on a real desktop: two copies in two folders, one window.
# Needs a desktop session, so it is not part of `just check`; skips (exit 2) when
# SimpleLife is already running.
instance-check:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p build
    # One binary, built like the release (no console window); this checks the process
    # guard, not the package.
    go build -ldflags "-H=windowsgui" -o build/instance-check.exe ./cmd/simplelife
    status=0
    powershell.exe -NoProfile -ExecutionPolicy Bypass         -File scripts/instance-check.ps1 -Exe "$(cygpath -w build/instance-check.exe 2>/dev/null || echo build/instance-check.exe)"         || status=$?
    rm -f build/instance-check.exe
    exit "$status"

# ── Build and release ────────────────────────────────────────────────

# Package for Windows: just build / just build --clean → dist/simplelife-<version>-windows-amd64.zip
build *args:
    #!/usr/bin/env bash
    set -euo pipefail
    bash scripts/build.sh "$@"

# Remove dist/ and build/
clean:
    @bash scripts/clean.sh
