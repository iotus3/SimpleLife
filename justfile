# SimpleLife 工程任务入口
# 原则：just 只做「一行转发」，真正的逻辑写在 scripts/*.sh 里，便于单独调试与复用。

set positional-arguments := true   # 参数以位置参数传给 shell，避免 {{args}} 拼接被解释（防注入）
set shell := ["bash", "-euo", "pipefail", "-c"]

# 列出所有任务
default:
    @just --list

# ── 环境 ─────────────────────────────────────────────────────────────

# 检查本机是否具备构建条件（Go 版本、纯 Go 构建、bash、just、git、数据目录）
doctor:
    #!/usr/bin/env bash
    set -euo pipefail
    bash scripts/doctor.sh

# ── 开发 ─────────────────────────────────────────────────────────────

# 直接运行（先构建到 build/ 再启动：数据库落在 build/datas/simplelife.db，
# 而不是 go run 的临时目录里）
run:
    #!/usr/bin/env bash
    set -euo pipefail
    go build -o build/simplelife.exe ./cmd/simplelife
    ./build/simplelife.exe

# 显示当前版本（唯一真相：viva.toml 的 [app] version）
version:
    @bash scripts/version.sh --show

# 发版改版本号：只改一处，Go 常量自动跟着生成（just update-version 0.2.0）
update-version v:
    @bash scripts/version.sh --set "{{v}}"

# 只检查生成物是否落后于 viva.toml（不改文件）；just check 里就跑这一条
version-check:
    @bash scripts/version.sh --check

# 把 internal/presentation/icons/svg/*.svg 重新编译成 icons.go
icons:
    @bash scripts/icons.sh

# ── 质量门禁 ─────────────────────────────────────────────────────────

# 格式化并整理依赖
fmt:
    @gofmt -l -w . && go mod tidy

# 只检查格式，不写文件
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    out="$(gofmt -l .)"
    if [ -n "$out" ]; then
        echo "✗ 未格式化的文件："
        echo "$out"
        exit 1
    fi
    echo "✓ 格式检查通过"

# go vet
vet:
    @go vet ./...

# 依赖方向守卫（§8.4）：违反分层红线即失败
depcheck:
    @bash scripts/check-deps.sh

# 全部单元测试
test:
    @go test ./... -count=1

# 带覆盖率，输出到 build/coverage.html
cover:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p build
    go test ./... -count=1 -coverprofile=build/coverage.out
    go tool cover -html=build/coverage.out -o build/coverage.html
    echo "▶ 总覆盖率：$(go tool cover -func=build/coverage.out | tail -1)"

# 表现层无头测试（viva headless，不开窗口）
test-ui:
    @go test ./internal/presentation/... -count=1 -v

# 端到端冒烟：真实程序 + 真实 SQLite，无头点完八个页面并截图（build/smoke/*.png）
smoke:
    @bash scripts/smoke.sh

# 提交前必须全绿（version-check 盯"生成物落后于 viva.toml"这一条，不靠编译缓存）
check: fmt-check version-check vet depcheck test
    @echo "✓ check 全部通过"

# 单实例守卫的真机验证：构建发布版 → 在**两个目录**各放一份（模拟 dist/ 留了多个
# 构建目录、快捷键指向旧副本的真实情况）→ 启动 A → 把窗口藏起来（等同最小化到托盘）
# → 启动 B → 断言 B 立即退出、进程数仍为 1、A 的窗口被唤回 → 再点 A 本身同样被拒。
# 需要桌面会话，所以不进 `just check`；若已有 SimpleLife 在运行会跳过（退出码 2）。
instance-check:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p build
    # 只编一个 exe：这一条验证的是进程守卫，不是打包产物，所以不走 viva package。
    # -H=windowsgui 让它跑起来不弹控制台，与发布版一致。
    go build -ldflags "-H=windowsgui" -o build/instance-check.exe ./cmd/simplelife
    status=0
    powershell.exe -NoProfile -ExecutionPolicy Bypass         -File scripts/instance-check.ps1 -Exe "$(cygpath -w build/instance-check.exe 2>/dev/null || echo build/instance-check.exe)"         || status=$?
    rm -f build/instance-check.exe
    exit "$status"

# ── 构建与发布 ───────────────────────────────────────────────────────

# 桌面打包：just build / just build --clean → dist/simplelife-<version>-windows-amd64.zip
build *args:
    #!/usr/bin/env bash
    set -euo pipefail
    bash scripts/build.sh "$@"

# 清理 dist/ 与 build/
clean:
    @bash scripts/clean.sh
