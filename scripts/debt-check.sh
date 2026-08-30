#!/usr/bin/env bash
# debt-check.sh — 欠账红线检查（复盘建议 1，docs/project/discussions/2026-08-29-process-retrospective.md）
#
# 作用：把两类隐性欠账变成 CI 可失败的硬指标：
#   1. TODO(dep) 占位（并行开发期遗留，承诺回填但从未立项）
#   2. 面向用户的 "not yet implemented" 桩（运行时返回 IsError）
#
# 规则：相对基线（scripts/debt-baseline.txt）只允许下降，不允许上升。
# 下降时提示更新基线；上升时直接失败并指出新增位置。
#
# 用法：
#   scripts/debt-check.sh           # 检查（CI / make debt-check 调用）
#   scripts/debt-check.sh --update  # 欠账减少后刷新基线（随同一次 PR 提交）
set -euo pipefail

cd "$(dirname "$0")/.."

BASELINE_FILE="scripts/debt-baseline.txt"
COUNT_CMD='grep -rn "TODO(dep)" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "/vendor/" | wc -l | tr -d " "'
STUB_CMD='grep -rn "not yet implemented" --include="*.go" ./internal ./pkg ./cmd 2>/dev/null | grep -v "_test.go" | wc -l | tr -d " "'

if [[ "${1:-}" == "--update" ]]; then
  count=$(eval "$COUNT_CMD")
  stubs=$(eval "$STUB_CMD")
  printf 'TODO_DEP_BASELINE=%s\nNOT_IMPLEMENTED_BASELINE=%s\n' "$count" "$stubs" > "$BASELINE_FILE"
  echo "基线已更新: TODO(dep)=$count, not-yet-implemented=$stubs → $BASELINE_FILE"
  exit 0
fi

if [[ ! -f "$BASELINE_FILE" ]]; then
  echo "错误: 找不到 $BASELINE_FILE（应随仓库提交）。请先运行 scripts/debt-check.sh --update 生成。"
  exit 1
fi

source "$BASELINE_FILE"

count=$(eval "$COUNT_CMD")
stubs=$(eval "$STUB_CMD")

fail=0

if (( count > TODO_DEP_BASELINE )); then
  echo "❌ TODO(dep) 数量上升: $count > 基线 $TODO_DEP_BASELINE"
  echo "   新增的占位不会被任何机制跟踪回填（复盘 P6）。"
  echo "   如确需新增，请在 PR 说明中给出回填 issue 编号，并同步更新 scripts/debt-baseline.txt。"
  fail=1
elif (( count < TODO_DEP_BASELINE )); then
  echo "✅ TODO(dep) $count（基线 $TODO_DEP_BASELINE，减少了 $((TODO_DEP_BASELINE - count)) 个）"
  echo "   提示: 请运行 scripts/debt-check.sh --update 刷新基线后一并提交。"
fi

if (( stubs > NOT_IMPLEMENTED_BASELINE )); then
  echo "❌ 用户可见 not-yet-implemented 桩数量上升: $stubs > 基线 $NOT_IMPLEMENTED_BASELINE"
  echo "   新增位置:"
  grep -rn "not yet implemented" --include="*.go" ./internal ./pkg ./cmd 2>/dev/null | grep -v "_test.go" | cut -d: -f1 | sort -u | head -20
  fail=1
elif (( stubs < NOT_IMPLEMENTED_BASELINE )); then
  echo "✅ not-yet-implemented 桩 $stubs（基线 $NOT_IMPLEMENTED_BASELINE，减少了 $((NOT_IMPLEMENTED_BASELINE - stubs)) 个）"
  echo "   提示: 请运行 scripts/debt-check.sh --update 刷新基线后一并提交。"
fi

if (( fail )); then
  exit 1
fi
echo "欠账红线检查通过。"
