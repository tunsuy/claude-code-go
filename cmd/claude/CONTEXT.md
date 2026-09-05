---
package: main
import_path: cmd/claude
layer: cli
generated_at: 2026-09-05T09:11:12Z
source_files: [main.go]
---

# cmd/claude

> Layer: **CLI** · Files: 1 · Interfaces: 0 · Structs: 0 · Functions: 0

## Dependencies

**Imports:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **ErrSilent 约定（2026-09-05，parity B2/B5）**：main 对 `bootstrap.ErrSilent` 特判——
  命令已自行向正确的流打印诊断时，只 exit 1、不再追加 `Error: …` 行。为什么：oracle
  （commander.js 系）的用户可见失败都是裸文本（无 Error: 前缀），cobra→main 的默认
  包装会破坏字节级对齐；用哨兵错误而非每个命令自己 os.Exit(1)，是为了让错误仍沿
  正常返回链传播、可被测试断言。
