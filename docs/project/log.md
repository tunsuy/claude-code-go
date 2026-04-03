# Claude Code Go — PM 巡检日志

> 维护人：PM Agent
> 格式：最新记录在最上方

---

## 2026-04-03（第 5 次巡检）

**阶段**：M1 第三批（编码阶段）持续推进

**进展**：
- #15（Agent-Core）✅ 完成并提交
  - `internal/engine/`：QueryEngine 流式事件循环、工具编排（并发安全工具并行/写工具串行）、BudgetTracker（连续性 nudge + 收益递减保护）
  - `internal/permissions/`：权限决策链（bypass→deny→validate→hook→allow→ask→工具默认）、DenialTracker
  - `internal/compact/`：对话压缩（SnipCompactor、MicroCompactor、AutoCompactor）
  - 全部测试通过（含 race detector）
- #17（Agent-TUI）🔄 已启动

**风险 / 阻塞**：无

**下一步**：
- 等待 #17（Agent-TUI）完成
- #17 完成后立即启动 #18（Agent-CLI）

---



**阶段**：M1 第三批（编码阶段）持续推进

**进展**：
- #14（Agent-Services）✅ 完成并提交
  - `internal/api/`：Anthropic 流式客户端，指数退避重试，用量追踪
  - `internal/mcp/`：MCP 客户端（stdio + HTTP SSE），JSON-RPC 2.0，连接池，工具适配器
  - `internal/oauth/`：OAuth2 PKCE 流程，singleflight token 刷新（修复并发刷新竞争），darwin Keychain / 其他平台 AES-256-GCM token 存储
  - 编译通过，无测试文件（I/O 密集型，留给集成测试）
- #15（Agent-Core）🔄 已启动

**风险 / 阻塞**：无

**下一步**：
- 等待 #15（Agent-Core）完成
- #15 完成后立即启动 #17（Agent-TUI）

---



**阶段**：M1 第三批（编码阶段）持续推进

**进展**：
- #13（Agent-Infra）✅ 完成并提交
  - 所有 P0 修复均已落地：P0-1（无 panic stub）、P0-2（Policy 优先级最高）、P0-3（COW maps.Clone）
  - go.mod 版本从 `go 1.22` 降至 `go 1.21`（匹配本地工具链 1.21.6）
  - 全部测试通过（`go test ./pkg/... ./internal/config/... ./internal/state/... ./internal/session/...`）
- #16（Agent-Tools）✅ 完成并提交
  - B-1 修复：`GetPath` 已从 `Tool` 主接口移除，仅保留于 `PathTool` 子接口
  - 所有内置工具实现完成（bash、fileops、glob、grep、web、mcp、agent、interact、tasks、misc）
  - `go test ./internal/tool/... ./internal/tools/fileops/...` 通过
- #14（Agent-Services）🔄 已启动

**风险 / 阻塞**：

| 编号 | 级别 | 内容 |
|------|------|------|
| （无） | — | 所有已知阻塞已解除 |

**下一步**：
- 等待 #14（Agent-Services）完成
- #14 完成后立即启动 #15（Agent-Core）
- #15 完成后立即启动 #17（Agent-TUI）
- #17 完成后立即启动 #18（Agent-CLI）

---

## 2026-04-02（第 2 次巡检）

**阶段**：M1 第三批（编码阶段）启动

**进展**：
- M1 第一批（#4~#9 各层详细设计文档）✅ 全部完成
- M1 第二批（#10~#12 Tech Lead 评审）✅ 全部通过，评审文档已提交
  - review-infra.md、review-core.md、review-services.md、review-tools.md、review-tui.md、review-cli.md 均已输出（中文）
- M1 第三批编码阶段启动：
  - #13（Agent-Infra）🔄 进行中
  - #16（Agent-Tools）🔄 进行中
- #14 #15 #17 #18 仍处于依赖阻塞状态

**风险 / 阻塞**：

| 编号 | 级别 | 内容 |
|------|------|------|
| B-1 | 🔴 阻塞 | `GetPath` 须从 `Tool` 主接口移除，仅保留于 `PathTool` 子接口；Agent-Tools 在修复前不得实现文件类工具 |
| P0-1 | 🔴 P0 | `NewAgentId()` 中的 panic 存根须在 #13 编码时移除 |
| P0-2 | 🔴 P0 | Policy 合并顺序反转，`mergeSettings` 参数顺序需修正，Agent-Infra 编码时必须处理 |
| P0-3 | 🔴 P0 | AppState map/slice 数据竞争，须用 `maps.Clone` copy-on-write 模式修复 |
| S-1 | 🟡 安全 | FileStore（非 macOS）加密方案未指定，Agent-Services 编码前须确定（推荐 AES-256-GCM） |

**下一步**：
- 持续跟踪 #13 和 #16 进度
- #13 完成后立即通知 Agent-Services 启动 #14
- Agent-Tools B-1 修复确认后解锁文件类工具实现

---

## 2026-04-02（第 1 次巡检）

**阶段**：M0 完成，M1 启动

**进展**：
- M0 全部完成（#2 计划表、#3 骨架初始化、#19 测试策略）
- M1 第一批全员同时认领，设计文档任务全面启动

**风险 / 阻塞**：无

---
