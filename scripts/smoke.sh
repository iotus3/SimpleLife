#!/usr/bin/env bash
#
# End to end smoke test: the real program (bootstrap, a real SQLite database, real
# drawing) is driven through Viva's headless transport, page by page, and screenshotted.
#
# Output:
#   build/smoke/simplelife-headless.exe   built with the vivahead tag (a release carries
#                                         no headless transport)
#   build/smoke/NN-<page>.png             one screenshot per page
#   build/smoke/session.log               the whole conversation
#
# Usage: bash scripts/smoke.sh [--keep] [--no-build]
#   --keep      keep the temporary data folder (for a post mortem)
#   --no-build  reuse build/smoke/simplelife-headless.exe
#
# Exit code: 0 everything passed; 1 an assertion failed.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

KEEP=0
BUILD=1
while [ $# -gt 0 ]; do
    case "$1" in
    --keep)
        KEEP=1
        shift
        ;;
    --no-build)
        BUILD=0
        shift
        ;;
    -h | --help)
        awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
        exit 0
        ;;
    *)
        echo "✗ unknown option: $1" >&2
        exit 2
        ;;
    esac
done

WORK_DIR="build/smoke"
BIN="$WORK_DIR/simplelife-headless.exe"
LOG="$WORK_DIR/session.log"
# The program keeps its data beside the executable, so this run uses build/smoke/datas.
# It is removed first, so every run starts from an empty database.
DATA_DIR="$WORK_DIR/datas"

cleanup() {
    if [ "$KEEP" -eq 0 ]; then
        rm -rf "$DATA_DIR"
    else
        echo "  · data folder kept at $DATA_DIR"
    fi
}
trap cleanup EXIT

echo "▶ building the headless binary"
mkdir -p "$WORK_DIR"
rm -rf "$DATA_DIR"
# Old screenshots go first. The page list has grown and shifted numbers before, and a
# stale file with a familiar name is worse than no file at all: it is what a reviewer
# looks at instead of the run they just made.
rm -f "$WORK_DIR"/*.png
if [ "$BUILD" -eq 1 ]; then
    # The version is a constant generated from viva.toml, and the commit and date come
    # from Go's own VCS stamping: the same sources the packaged build has. The about page
    # in these screenshots therefore shows the real version, with no -X injection.
    go build -tags vivahead -trimpath -ldflags "-s -w" -o "$BIN" ./cmd/simplelife
fi
[ -x "$BIN" ] || {
    echo "✗ $BIN is missing; run again without --no-build" >&2
    exit 1
}

# One block per page: click the rail, screenshot, dump the control tree (so the page
# title can be asserted), then check the layout and the accessible names. The order
# matches the routes in internal/presentation/run.go.
pages=(
    "今日:home"
    "目标:goals"
    "待办:tasks"
    "专注:focus"
    "账单:ledger"
    "统计:stats"
    "模板:templates"
    "设置:settings"
    "数据:data"
    "关于:about"
)

commands="$WORK_DIR/commands.txt"
: >"$commands"
index=0
for entry in "${pages[@]}"; do
    label="${entry%%:*}"
    name="${entry##*:}"
    index=$((index + 1))
    {
        echo "click $label"
        echo "settle"
        echo "shot $WORK_DIR/$(printf '%02d' "$index")-$name.png"
        echo "tree"
        echo "expect text $label"
        # Layout check: nothing clipped, overflowing or overlapping, and every
        # interactive control has an accessible name (an unnamed control can be reached
        # neither by a screen reader nor by this script's selectors).
        echo "expect layout clean"
        echo "expect named"
    } >>"$commands"
done
# The interactions that matter: create a goal, add a todo, finish it, start a focus
# session, record an expense, open the import card in a taller window, switch to the
# dark appearance, then switch the language to English and walk the pages again (English
# copy is longer, and overflow only shows there). Every step asserts painted text with
# expect, and a failure exits non zero.
# A name is not unique on a page (the rail entry for tasks and a checkbox of the same
# name on the data page), so navigation is written as role:name; controls inside a row use
# the unnamed selector checkbox:.
cat >>"$commands" <<'CMDS'
click button:目标
settle
click button:新建目标
settle
expect text 名称
expect text 截止日期
click textfield:名称
type 软考高项
click textfield:备注
type 高级项目经理
shot build/smoke/27-goal-editor.png
click button:确认添加
settle
expect text 软考高项
expect text 高级项目经理
expect text 还剩
expect text 进行中
shot build/smoke/28-goal-card.png
click button:待办
settle
click textfield:快速添加
type 写周报
click 添加
settle
expect text 写周报
shot build/smoke/38-tasks-filled.png
click button:开始专注
settle
expect exists button:暂停
expect exists button:完成
expect exists button:放弃
expect absent button:开始专注
shot build/smoke/29-task-session.png
click button:专注
settle
expect text 进行中
shot build/smoke/11-focus-running.png
click button:待办
settle
click button:完成
settle
expect text 已完成
shot build/smoke/25-tasks-done.png
click 历史
settle
expect text 写周报
shot build/smoke/26-tasks-history.png
click 今天
settle
click button:新建待办
settle
expect text 一个番茄的时长
expect text 预估番茄
shot build/smoke/34-task-editor.png
click button:取消
settle
click button:模板
settle
click button:新建模板
settle
expect text 一个番茄的时长
expect text 预估番茄
expect layout clean
shot build/smoke/36-template-editor.png
click textfield:模板名
type 精读文献
click textfield:待办标题
type 精读一篇
click textfield:一个番茄的时长（分钟）
type 40
click button:保存
settle
expect text "40 分钟/个"
shot build/smoke/37-template-card.png
click button:账单
settle
click 记一笔
settle
expect text 保存
click 选择日期
settle
expect text 今天
shot build/smoke/20-date-picker.png
key Escape
settle
expect text 保存
click button:取消
settle
click 账户
settle
expect text 内置
expect exists button:编辑账户
shot build/smoke/31-ledger-accounts.png
click button:编辑账户
settle
expect text 币种
click button:取消
settle
click 预算
settle
expect text 本月还没有预算
click button:新增预算
settle
expect text 限额
shot build/smoke/32-budget-editor.png
click textfield:限额
type 300
click button:保存
settle
expect no-text 本月还没有预算
expect text ¥300.00
shot build/smoke/33-ledger-budgets.png
click button:数据
settle
resize 1240 1700
settle
expect text 导入数据
expect text 选择文件
shot build/smoke/12-data-import.png
click checkbox:账单流水
settle
expect text 1/2
expect text 2/2
shot build/smoke/21-preview-sheets.png
expect layout clean
resize 1240 820
settle
click button:统计
settle
expect text 近14天
shot build/smoke/22-stats-14.png
click 近30天
settle
expect text 自定义
expect layout clean
shot build/smoke/23-stats-30.png
click 自定义
settle
expect text 开始日期
expect text 结束日期
shot build/smoke/24-stats-custom.png
click button:设置
settle
click 深色
settle
shot build/smoke/13-settings-dark.png
click button:关于
settle
expect text 检查更新
expect text 隐私更自主
shot build/smoke/14-about-dark.png
click button:设置
settle
click 跟随系统
settle
click 设置
settle
click English
settle
expect text Settings
expect layout clean
shot build/smoke/15-settings-en.png
click button:Today
settle
expect layout clean
shot build/smoke/16-home-en.png
click button:Tasks
settle
expect layout clean
shot build/smoke/17-tasks-en.png
click "button:New task"
settle
expect text Pomodoros
expect layout clean
shot build/smoke/35-task-editor-en.png
key Escape
settle
click button:Goals
settle
expect layout clean
shot build/smoke/30-goals-en.png
click button:Ledger
settle
expect layout clean
shot build/smoke/18-ledger-en.png
click button:About
settle
expect text updates
expect layout clean
shot build/smoke/19-about-en.png
quit
CMDS

echo "▶ headless run (data folder $DATA_DIR)"
# The data lives in datas/ beside the executable, so this never touches the real database.
set +e
# SIMPLELIFE_ALLOW_SECOND_INSTANCE=1: the single-instance guard is for the real program,
# and this copy would otherwise collide with one the developer has open. The guard itself
# is verified on a real desktop by `just instance-check`; here it steps aside, so the gate
# cannot fail because somebody left the application running.
VIVA_HEADLESS=1 SIMPLELIFE_ALLOW_SECOND_INSTANCE=1 "$BIN" <"$commands" >"$LOG" 2>&1
status=$?
set -e
if [ "$status" -ne 0 ]; then
    echo "✗ the program exited with $status; log: $LOG" >&2
    tail -20 "$LOG" >&2
    exit 1
fi

echo "▶ assertions"
FAILED=0
fail() {
    printf '  ✗ %s\n' "$1"
    FAILED=$((FAILED + 1))
}
pass() { printf '  ✓ %s\n' "$1"; }

if grep -q '^ok ready' "$LOG"; then
    pass "window ready: $(grep -m1 '^ok ready' "$LOG" | sed 's/^ok ready //')"
else
    fail "no ready answer"
fi

if grep -q '^error ' "$LOG"; then
    fail "a command failed: $(grep -m3 '^error ' "$LOG" | tr '\n' ' ')"
else
    pass "every command answered without error"
fi

expected=$(wc -l <"$commands" | tr -d ' ')
# The leading "ok ready <w>x<h>" is the session handshake, not an answer to a command.
actual=$(grep '^ok' "$LOG" | grep -vc '^ok ready' || true)
if [ "$actual" -eq "$expected" ]; then
    pass "$actual/$expected commands answered ok"
else
    fail "$actual answers for $expected commands"
fi

for entry in "${pages[@]}"; do
    label="${entry%%:*}"
    if grep -q "label=\"$label\"" "$LOG"; then
        pass "rail item $label is clickable"
    else
        fail "the control tree has no $label"
    fi
done

index=0
for entry in "${pages[@]}"; do
    index=$((index + 1))
    shot="$WORK_DIR/$(printf '%02d' "$index")-${entry##*:}.png"
    if [ -s "$shot" ]; then
        pass "screenshot $(basename "$shot")"
    else
        fail "missing or empty screenshot: $shot"
    fi
done
for name in 38-tasks-filled 11-focus-running 12-data-import 13-settings-dark 14-about-dark \
    15-settings-en 16-home-en 17-tasks-en 18-ledger-en 19-about-en \
    20-date-picker 21-preview-sheets 22-stats-14 23-stats-30 24-stats-custom     25-tasks-done 26-tasks-history 27-goal-editor 28-goal-card 29-task-session 30-goals-en     31-ledger-accounts 32-budget-editor 33-ledger-budgets 34-task-editor 35-task-editor-en     36-template-editor 37-template-card; do
    shot="$WORK_DIR/$name.png"
    if [ -s "$shot" ]; then
        pass "screenshot $(basename "$shot")"
    else
        fail "missing interaction screenshot: $shot"
    fi
done

db="$DATA_DIR/simplelife.db"
if [ -s "$db" ]; then
    pass "database created: $db ($(wc -c <"$db" | tr -d ' ') bytes)"
else
    fail "no database in the temporary data folder: $db"
fi

echo
if [ "$FAILED" -gt 0 ]; then
    echo "✗ $FAILED checks failed; log: $LOG"
    exit 1
fi
echo "✓ smoke test passed (log: $LOG, screenshots: $WORK_DIR/*.png)"
