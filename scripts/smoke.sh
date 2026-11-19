#!/usr/bin/env bash
#
# 端到端冒烟测试：启动真实程序（bootstrap + 真实 SQLite + 真实绘制），
# 通过 Viva 的无头协议逐页点击并截图，确认九个页面都能打开。
#
# 产物：
#   build/smoke/simplelife-headless.exe   带 vivahead 标签的构建（发布构建不含该传输层）
#   build/smoke/NN-<page>.png             每个页面的截图
#   build/smoke/session.log               无头会话的完整往来
#
# 用法: bash scripts/smoke.sh [--keep] [--no-build]
#   --keep      保留临时数据目录（排查用）
#   --no-build  复用已有的 build/smoke/simplelife-headless.exe
#
# 退出码: 0 全部通过；1 有断言失败。
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
        sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
        exit 0
        ;;
    *)
        echo "✗ 未知选项：$1" >&2
        exit 2
        ;;
    esac
done

WORK_DIR="build/smoke"
BIN="$WORK_DIR/simplelife-headless.exe"
LOG="$WORK_DIR/session.log"
# 程序的数据目录在 exe 旁边（datas/），所以无头会话用的就是 build/smoke/datas。
# 每次运行前清掉，保证从空库开始。
DATA_DIR="$WORK_DIR/datas"

cleanup() {
    if [ "$KEEP" -eq 0 ]; then
        rm -rf "$DATA_DIR"
    else
        echo "  · 数据目录保留在 $DATA_DIR"
    fi
}
trap cleanup EXIT

echo "▶ 构建无头版本"
mkdir -p "$WORK_DIR"
rm -rf "$DATA_DIR"
# Old screenshots go first. The page list has grown and shifted numbers before, and a
# stale file with a familiar name is worse than no file at all: it is what a reviewer
# looks at instead of the run they just made.
rm -f "$WORK_DIR"/*.png
if [ "$BUILD" -eq 1 ]; then
    # 版本号是源码里的常量（viva.toml 是唯一真相，测试盯着两者相等），提交号与
    # 构建日期来自 go 自己的 VCS 打戳 —— 与 viva package 打出来的发布版同一套来路，
    # 所以冒烟截图上的关于页显示的就是真实版本，不需要在这里注入 -X。
    go build -tags vivahead -trimpath -ldflags "-s -w" -o "$BIN" ./cmd/simplelife
fi
[ -x "$BIN" ] || {
    echo "✗ 缺少 $BIN，去掉 --no-build 重跑" >&2
    exit 1
}

# 每页一条：点侧栏 -> 截图 -> 导出控件树（供断言页标题出现）-> 检查排版与可访问名。
# 顺序与 internal/presentation/run.go 的 routes 一致。
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
        # 排版自检：没有文字被裁切/溢出/压字，没有互相压住的控件；每个可交互控件
        # 都有可访问名（无名字的控件读屏和本脚本的选择器都点不到）。
        echo "expect layout clean"
        echo "expect named"
    } >>"$commands"
done
# 关键交互：新建一条待办、勾选完成、记一笔支出、开始一次专注、加高窗口拍导入卡、
# 切深色外观，最后切到英文再逐页截图：英文文案比中文长，溢出在这几张图里才看得出来。
# 每一步都用 expect 断言窗口里的文字，失败即退出非零。
# 名字在页面上不是唯一的（侧栏「待办」与导出页的同名复选框），所以导航一律写成
# 「角色:名字」，行内控件则用无名控件选择器 checkbox:。
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

echo "▶ 无头运行（数据目录 $DATA_DIR）"
# 数据落在 exe 旁边的 datas/ 里，测试因此不会碰到真实的数据库。
set +e
# SIMPLELIFE_ALLOW_SECOND_INSTANCE=1：单实例守卫是给真实程序的，而冒烟跑的这份可能
# 与开发者正开着的 SimpleLife 撞名。守卫本身由 `just instance-check` 真机验证，这里
# 明确让开，保证门禁不会因为"你正好开着应用"而失败。
VIVA_HEADLESS=1 SIMPLELIFE_ALLOW_SECOND_INSTANCE=1 "$BIN" <"$commands" >"$LOG" 2>&1
status=$?
set -e
if [ "$status" -ne 0 ]; then
    echo "✗ 进程退出码 $status，日志：$LOG" >&2
    tail -20 "$LOG" >&2
    exit 1
fi

echo "▶ 断言"
FAILED=0
fail() {
    printf '  ✗ %s\n' "$1"
    FAILED=$((FAILED + 1))
}
pass() { printf '  ✓ %s\n' "$1"; }

if grep -q '^ok ready' "$LOG"; then
    pass "窗口在 $((0)) 帧内就绪：$(grep -m1 '^ok ready' "$LOG" | sed 's/^ok ready //')"
else
    fail "没有收到就绪应答"
fi

if grep -q '^error ' "$LOG"; then
    fail "有命令报错：$(grep -m3 '^error ' "$LOG" | tr '\n' ' ')"
else
    pass "全部命令无错误应答"
fi

expected=$(wc -l <"$commands" | tr -d ' ')
# 首行的 "ok ready <w>x<h>" 是会话握手，不算某条命令的应答。
actual=$(grep '^ok' "$LOG" | grep -vc '^ok ready' || true)
if [ "$actual" -eq "$expected" ]; then
    pass "$actual/$expected 条命令均有 ok 应答"
else
    fail "应答数 $actual 与命令数 $expected 不一致"
fi

for entry in "${pages[@]}"; do
    label="${entry%%:*}"
    if grep -q "label=\"$label\"" "$LOG"; then
        pass "侧栏「$label」可点击"
    else
        fail "控件树里找不到「$label」"
    fi
done

index=0
for entry in "${pages[@]}"; do
    index=$((index + 1))
    shot="$WORK_DIR/$(printf '%02d' "$index")-${entry##*:}.png"
    if [ -s "$shot" ]; then
        pass "截图 $(basename "$shot")"
    else
        fail "缺少截图或截图是空文件：$shot"
    fi
done
for name in 38-tasks-filled 11-focus-running 12-data-import 13-settings-dark 14-about-dark \
    15-settings-en 16-home-en 17-tasks-en 18-ledger-en 19-about-en \
    20-date-picker 21-preview-sheets 22-stats-14 23-stats-30 24-stats-custom     25-tasks-done 26-tasks-history 27-goal-editor 28-goal-card 29-task-session 30-goals-en     31-ledger-accounts 32-budget-editor 33-ledger-budgets 34-task-editor 35-task-editor-en     36-template-editor 37-template-card; do
    shot="$WORK_DIR/$name.png"
    if [ -s "$shot" ]; then
        pass "截图 $(basename "$shot")"
    else
        fail "缺少交互截图：$shot"
    fi
done

db="$DATA_DIR/simplelife.db"
if [ -s "$db" ]; then
    pass "数据库已建立：$db（$(wc -c <"$db" | tr -d ' ') 字节）"
else
    fail "临时数据目录里没有数据库：$db"
fi

echo
if [ "$FAILED" -gt 0 ]; then
    echo "✗ $FAILED 项失败，日志：$LOG"
    exit 1
fi
echo "✓ 冒烟测试通过（日志：$LOG，截图：$WORK_DIR/*.png）"
