# Claude Code Go — Roadmap

> 最后更新：2026-09-05
>
> 本文档基于与 claude-code-main（TypeScript 原版）的全面对比分析，规划 claude-code-go 项目从当前状态到生产可用的完整路径。
>
> **2026-08-29 状态刷新**：Phase 1 已全部完成（权限接入 + 安全增强 T-060..T-064 已合并 main，CI 含 lint + race + 覆盖率）；Phase 2 大部分完成（34 个工具注册、20 个斜杠命令、mcp/agents/update/install 子命令落地）。各节标注 ✅ 已完成 / ⚠️ 部分完成 / ❌ 未开始。
>
> **2026-09-05 状态刷新（B2/B5 落地，PR #23）**：`mcp` 命令组（7 命令）与 `plugin` 命令组（8 命令 + marketplace 子树）从桩变为真实实现，输出与 oracle（claude v2.1.261）字节级一致；用户可见桩 42 → 27。插件系统 CLI 层落地（§3.4 的 3.4.4），加载引擎仍为骨架。同日起项目按维护模式运转（见 [`test/parity/cases.md`](../test/parity/cases.md) 差距台账与 [`2026-08-29 流程复盘`](discussions/2026-08-29-process-retrospective.md)）。

---

## 目录

- [当前状态概览](#当前状态概览)
- [版本规划总览](#版本规划总览)
- [Phase 1：核心安全闭环（v0.2.0）](#phase-1核心安全闭环v020)
- [Phase 2：工具与命令补全（v0.3.0）](#phase-2工具与命令补全v030)
- [Phase 3：多 Provider 与 MCP 完善（v0.4.0）](#phase-3多-provider-与-mcp-完善v040)
- [Phase 4：高级功能与体验优化（v0.5.0）](#phase-4高级功能与体验优化v050)
- [Phase 5：生产就绪（v1.0.0）](#phase-5生产就绪v100)
- [各阶段依赖关系图](#各阶段依赖关系图)
- [风险与缓解](#风险与缓解)
- [度量标准](#度量标准)

---

## 当前状态概览

```
┌─────────────────────────────────────────────────────────────┐
│                   claude-code-go v0.8.0                     │
│                                                             │
│  ✅ 已完成（可用）          ⚠️ 部分完成            ❌ 缺失  │
│  ─────────────────     ──────────────────     ────────────  │
│  • 核心引擎 query 循环   • Hook 配置接入       • LSP 服务   │
│  • API Direct 客户端     • MCP (3/4 transport) • Feature Flag│
│  • OpenAI 兼容客户端     • Bedrock/Vertex      • Voice/Vim  │
│  • 上下文压缩三件套      • 插件加载引擎        • Analytics  │
│  • TUI (BubbleTea)      • 测试覆盖率          • Remote 模式│
│  • OAuth 认证            │                      • 数据迁移   │
│  • 配置加载              │                      │            │
│  • 会话持久化            │                      │            │
│  • 协调器框架            │                      │            │
│  • 权限管线（含安全增强） │                      │            │
│  • 工具系统 (34 个)      │                      │            │
│  • CLI 子命令：mcp(7)    │                      │            │
│    plugin(8)+marketplace │                      │            │
│    auth/doctor/update    │                      │            │
└─────────────────────────────────────────────────────────────┘

完成度: ███████████████░░░░░ 75%
（维护模式：以 parity 差距台账驱动，不再按本表时间线推进）
```

---

## 版本规划总览

> ⚠️ 下表为构建期（2026-04）的历史规划，时间线已作废——项目自 2026-08-29 起转入维护模式，
> 由 [`test/parity/cases.md`](../test/parity/cases.md) 差距台账驱动。保留此图仅为阶段划分参照；
> 版本号实际推进到 v0.8.0，与下表阶段并不一一对应。各 Phase 的完成状态见后文各节标注。

```
时间线（预估）
─────────────────────────────────────────────────────────────────────────

v0.1.0 (当前)          v0.2.0             v0.3.0             v0.4.0             v0.5.0          v1.0.0
  │                      │                  │                  │                  │               │
  ├── 核心引擎 ✅        ├── 权限接入 ✅    ├── 工具补全 ✅    ├── Bedrock/Vertex  ├── LSP 服务     ├── 生产就绪
  ├── TUI ✅             ├── Hook 接入 ⚠️   ├── CLI 补全 ⚠️    ├── MCP WebSocket   ├── Voice 输入   ├── 性能调优
  ├── API 客户端 ✅      ├── 测试基线 ✅    ├── Agent 集成 ✅  ├── 插件系统        ├── Remote 模式  ├── 安全审计
  │                      ├── CI/CD 强化 ✅  ├── 斜杠命令 ✅    ├── Feature Flags   ├── Vim 模式     ├── 文档完善
  │                      │                  │                  │                  │               │
──┼──────────────────────┼──────────────────┼──────────────────┼──────────────────┼───────────────┼──
  现在                   +3 周              +3 周              +4 周              +4 周           +2 周
                                                                                            总计 ~16 周
```

---

## Phase 1：核心安全闭环（v0.2.0）

**目标**：让工具执行经过权限检查和 Hook 系统，消除安全隐患。这是最高优先级，因为当前所有工具调用都是**无权限控制**的直接执行。

**预估周期**：3 周

### 1.1 权限系统接入引擎 ✅ 已完成

**现状**：`internal/engine/orchestration.go` 的 `executeOneTool` 已调用 `permissions.Checker`，Ask 请求经 Msg channel 到达 TUI 权限对话框，`bootstrap/wire.go` 完成 Checker 组装（含 Hook Dispatcher + 规则持久化）。

**任务清单**：

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1.1.1 | ✅ 将 `permissions.Checker` 注入 `engine.Config` | `internal/engine/engine.go` | 已新增 `Checker` 字段 |
| 1.1.2 | ✅ 在 `executeOneTool` 中调用权限检查 | `internal/engine/orchestration.go` | 已插入 `RequestPermission` 流程 |
| 1.1.3 | ✅ 权限 Ask 请求通过 `Msg` channel 传递到 TUI | `internal/engine/msg.go` | 已有 `MsgTypePermissionAsk` / `MsgTypePermissionResponse` |
| 1.1.4 | ✅ TUI 处理权限对话框 | `internal/tui/permissions.go` | 已对接引擎 Msg |
| 1.1.5 | ✅ 在 `bootstrap/wire.go` 中构建 Checker 并注入 | `internal/bootstrap/wire.go` | 已组装 Hook Dispatcher + Checker |

### 1.1.6 权限系统安全增强 ✅ 已完成（T-060..T-064，2026-08-29 合并 main）

> 详细分析见：[权限系统差异分析报告](analysis/permission-system-gap-analysis.md)

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1.1.6.1 | ✅ 实现危险文件/目录保护列表 | `internal/permissions/filesystem.go` | `DangerousPathResult`，防修改 `.bashrc`、`.gitconfig` 等 |
| 1.1.6.2 | ✅ 实现 Bash 命令基础安全检查 | `internal/tools/shell/security.go` | 检测 `sudo`、`rm -rf`、`eval`、复合/管道命令分段 |
| 1.1.6.3 | ✅ 实现权限持久化（Always Allow 生效） | `internal/permissions/persist.go` | `PersistConfig`，保存到 `.claude/settings.json` |
| 1.1.6.4 | ✅ 实现拒绝降级机制 | `internal/permissions/denial.go` | 连续 3 次/累计 20 次拒绝后改路 askUser |
| 1.1.6.5 | ✅ 完善 Shell 规则通配符匹配 | `internal/tools/shell/matcher.go` | `git *`、`go test:*`、`:*` 前缀语义，复合命令全 stage 命中 |

**实际架构**（比原计划更进一步）：

```
executeOneTool (已实现)
─────────────────────
1. registry.Get(name)
2. checker.RequestPermission()
     └─ CanUseTool 九层链（bypass→deny→validate→hook→allow→ask→mode→tool-specific→default）
3. if Ask → askCh → TUI 对话框 → respCh（含 Always Allow 持久化）
4. if Deny → 拒绝降级检查（阈值内直接 Deny，超阈值转 askUser）
5. t.Call()
```

### 1.2 Hook 系统接入引擎 ⚠️ 部分完成

**现状**：`internal/hooks/hooks.go` 的 `Dispatcher` 已接入权限管线第 4 层（PreToolUse 可阻断）；引擎有独立的 `StopHookRegistry`（每轮结束后触发，记忆提取/auto-dream 已挂载）。**缺口**：PostToolUse 未在引擎工具执行后触发；`bootstrap/wire.go` 用空 Dispatcher（`NewDispatcher(nil, false)`），settings.json 中配置的 Hook 定义尚未加载。

**任务清单**：

| # | 任务 | 文件 | 状态 | 说明 |
|---|------|------|------|------|
| 1.2.1 | 将 `hooks.Dispatcher` 注入 `engine.Config` | `internal/engine/engine.go` | ⚠️ | 已注入 permissions.CheckerConfig，引擎本体未直接持有 |
| 1.2.2 | 在工具执行前调用 `PreToolUse` Hook | `internal/permissions/checker.go` | ✅ | 管线第 4 层，Hook 可阻断 |
| 1.2.3 | 在工具执行后调用 `PostToolUse` Hook | `internal/engine/orchestration.go` | ❌ | 未触发 |
| 1.2.4 | 在查询循环结束时调用 `Stop` Hook | `internal/engine/stop_hooks.go` | ✅ | StopHookRegistry 已工作（memory extraction 挂载） |
| 1.2.5 | 在 `bootstrap/wire.go` 中从 settings 构建 Dispatcher | `internal/bootstrap/wire.go` | ❌ | 目前硬编码空 Dispatcher |

### 1.3 测试覆盖率基线 ✅ 已完成

**现状**：38 个包平均覆盖率 ~59%，核心包（engine/permissions/tools）达标；全库测试含 `-race` 全绿。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 1.3.1 | ✅ 权限接入后编写集成测试 | 完成 | Allow/Deny/Ask/降级路径全覆盖（denial_test.go 等） |
| 1.3.2 | ✅ Hook 调度集成测试 | 完成 | PreToolUse Block 路径已覆盖 |
| 1.3.3 | ✅ 引擎 query 循环端到端测试 | 完成 | mock API client 端到端用例 |
| 1.3.4 | ⚠️ 设立覆盖率门槛 | 部分 | CI 已产出 coverage.out 并上传 codecov；硬门槛未设 |

### 1.4 CI/CD 强化 ✅ 已完成

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 1.4.1 | ✅ CI 运行 `golangci-lint` | 完成 | `golangci-lint-action@v6` |
| 1.4.2 | ✅ 集成 `go test -race` | 完成 | ci.yml test job |
| 1.4.3 | ✅ 覆盖率报告上传 | 完成 | coverage.out → codecov；另有 codeql.yml |

### Phase 1 完成标准

- [x] 所有写操作工具执行前经过权限检查（2026-08-29 验证：main 全绿）
- [x] Hook PreToolUse / Stop 已接入；PostToolUse 与 settings 加载仍缺（见 1.2）
- [x] 核心包测试覆盖率 ≥ 60%（全库均值 ~59%，核心包达标）
- [x] CI 通过 lint + race + test

---

## Phase 2：工具与命令补全（v0.3.0）✅ 已完成（2026-09-05）

**目标**：补齐 11 个未实现的工具和 20 个 CLI 子命令，实现与原版功能对等的工具集。

**预估周期**：3 周（历史规划；实际经维护模式 parity 驱动完成，见 §2.4）

### 2.1 交互工具实现 ✅ 已完成

| # | 工具 | 文件 | 状态 |
|---|------|------|------|
| 2.1.1 | ✅ `TodoWrite` | `internal/tools/interact/` | 已注册 |
| 2.1.2 | ✅ `AskUserQuestion` | `internal/tools/interact/` | 已注册，经 Msg channel 与 TUI 交互 |
| 2.1.3 | ✅ `EnterPlanMode` / `ExitPlanMode` | `internal/tools/interact/` | 已注册 |
| 2.1.4 | ✅ `EnterWorktree` / `ExitWorktree` | `internal/tools/interact/` | 已注册 |

（Todo 状态后来演进为 Task 系统：`internal/tools/tasks/` 提供 TaskCreate/Get/List/Update/Stop/Output 六个工具，比原计划的单一 TodoWrite 更完整。）

### 2.2 Agent/MCP 工具实现 ✅ 已完成

| # | 工具 | 文件 | 状态 |
|---|------|------|------|
| 2.2.1 | ✅ `Agent` tool | `internal/tools/agent/` | 已集成 `coordinator` |
| 2.2.2 | ✅ `SendMessage` tool | `internal/tools/agent/` | 已集成 |
| 2.2.3 | ✅ `MCPProxyTool` | `internal/tools/mcp/` | 已集成 `mcp.Pool` |
| 2.2.4 | ✅ `ListMcpResources` | `internal/tools/mcp/` | 已注册 |
| 2.2.5 | ✅ `ReadMcpResource` | `internal/tools/mcp/` | 已注册 |

（另有 `GetAgentStatus` 工具与 `misc/`（Skill/Brief/ToolSearch/Sleep/SyntheticOutput）、`memory/`（MemoryRead/Write/Delete）、`web/`、`shell/` 等，当前共 **34 个工具**注册。）

### 2.3 斜杠命令补全 ✅ 已完成

| # | 命令 | 状态 | 实现要点 |
|---|------|------|----------|
| 2.3.1 | ✅ `/config` | 完成 | `internal/commands/builtins.go` |
| 2.3.2 | ✅ `/mcp` | 完成 | 展示 MCP 连接状态 |
| 2.3.3 | ✅ `/resume` | 完成 | 会话恢复（Terminal/Resume 深化见 [terminal-resume 差距分析](analysis/terminal-resume-system-gap-analysis.md)） |
| 2.3.4 | ✅ `/terminal-setup` | 完成 | shell 补全脚本 |
| 2.3.5 | ✅ `/review` | 完成 | 有专属逻辑 |
| 2.3.6 | ✅ `/commit` | 完成 | 有专属逻辑 |
| 2.3.7 | ✅ `/diff` | 完成 | 有专属逻辑 |
| 2.3.8 | ✅ `/init` | 完成 | 有专属逻辑 |

（现共 **20 个内建斜杠命令**：clear/help/exit/vim/theme/model/effort/status/cost/session/config/compact/memory/dream/mcp/review/commit/diff/init/resume/terminal-setup。）

### 2.4 CLI 子命令补全 ✅ 已完成（2026-09-05，B2/B5）

**高优先级（用户直接使用）**：

| # | 命令 | 状态 | 说明 |
|---|------|------|------|
| 2.4.1 | ✅ `mcp add` | 完成 | `internal/bootstrap/mcp_run.go`（2026-09-05 从桩变为真实实现，字节级对齐 oracle） |
| 2.4.2 | ✅ `mcp remove` | 完成 | 同上 |
| 2.4.3 | ✅ `mcp list` | 完成 | 同上 |
| 2.4.4 | ✅ `mcp get` | 完成 | 同上（含连接健康检查） |

**中优先级**：

| # | 命令 | 状态 | 说明 |
|---|------|------|------|
| 2.4.5 | ✅ `update` | 完成 | `internal/bootstrap/misc.go` |
| 2.4.6 | ✅ `install` | 完成 | shell 集成 |
| 2.4.7 | ✅ `mcp add-json` | 完成 | |
| 2.4.8 | ✅ `mcp add-from-claude-desktop` | 完成 | |

**低优先级（可延后）**：

| # | 命令 | 状态 | 说明 |
|---|------|------|------|
| 2.4.9 | ✅ `agents list/add/remove` | 完成 | `internal/bootstrap/misc.go` |
| 2.4.10 | ✅ `plugin list/install/uninstall/...` | 完成 | 2026-09-05 落地（PR #23）：list/install/uninstall/enable/disable/update/validate + marketplace 子树，字节级对齐 oracle；分歧与未验证边缘见 cases.md §B12 |
| 2.4.11 | ✅ `mcp reset-project-choices` | 完成 | |

（另有 `auth login/logout/status`、`doctor`、`mcp serve` 等命令。）
（`plugin details/eval/init/prune/tag` 为有意 waive，见 cases.md §B11 二级缺口表。）

### Phase 2 完成标准

- [x] 工具全部可执行（非占位）— 34 个工具注册
- [x] `/review`、`/commit`、`/diff`、`/init` 有专属逻辑
- [x] `mcp add/remove/list/get` 可正常操作配置文件
- [x] Agent tool 能通过 Coordinator 启动子代理
- [x] `plugin` 子命令组（2026-09-05，PR #23；插件加载引擎见 Phase 3.4）

---

## Phase 3：多 Provider 与 MCP 完善（v0.4.0）

**目标**：支持真实的 AWS Bedrock / GCP Vertex 调用，完善 MCP 协议实现，引入 Feature Flags 和插件系统。

**预估周期**：4 周

### 3.1 Bedrock Provider 实现 ⚠️ 部分完成（结构占位）

**现状**（2026-08-29 核查）：`internal/api/factory.go` 已有 `bedrockClient`（正确的 endpoint URL 拼装 + region 配置），但源码注释明确 "Full Bedrock signing is out of scope" —— 请求未做 AWS SigV4 签名，真实调用不可用。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 3.1.1 | ❌ 引入 `aws-sdk-go-v2` 依赖 | 未开始 | 用于 AWS Signature V4 签名 |
| 3.1.2 | ⚠️ 实现 `bedrockClient.Stream` / `Complete` | 结构就绪 | 委托 directClient，端点正确，缺签名 |
| 3.1.3 | ❌ 实现 AWS 凭证链（env → profile → IMDS） | 未开始 | |
| 3.1.4 | ❌ 添加 `awsAuthRefresh` 支持 | 未开始 | |
| 3.1.5 | ❌ 请求/响应格式转换 | 未开始 | Bedrock body 格式与 Direct API 不同 |

### 3.2 Vertex Provider 实现 ⚠️ 部分完成（结构占位）

**现状**（2026-08-29 核查）：`internal/api/factory.go` 已有 `vertexClient`（aiplatform URL 模板 + project/region 配置），但没有 GCP OAuth2 token 获取，真实调用不可用。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 3.2.1 | ❌ 引入 `golang.org/x/oauth2/google` 依赖 | 未开始 | 用于 GCP 认证 |
| 3.2.2 | ⚠️ 实现 `vertexClient.Stream` / `Complete` | 结构就绪 | 委托 directClient，缺认证 |
| 3.2.3 | ❌ 实现 GCP ADC（Application Default Credentials） | 未开始 | |
| 3.2.4 | ❌ 添加 `gcpAuthRefresh` 支持 | 未开始 | |

### 3.3 MCP WebSocket Transport ❌ 未开始

**现状**（2026-08-29 核查）：`internal/mcp/transport.go` 中 `TransportWS` 已声明但构造直接返回 "not yet implemented"；stdio/sse/http 三种可用。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 3.3.1 | ❌ 引入 `nhooyr.io/websocket` 依赖 | 未开始 | 轻量级 WebSocket 库 |
| 3.3.2 | ❌ 实现 `wsTransport` | 未开始 | 实现 `Transport` 接口 |
| 3.3.3 | ❌ 自动重连逻辑 | 未开始 | 连接断开后指数退避重连 |
| 3.3.4 | ❌ 集成测试 | 未开始 | 使用 mock WS server 测试 |

### 3.4 插件系统实现 ⚠️ 部分完成（CLI 层已落地，加载引擎待做）

**现状**（2026-09-05 更新）：CLI 管理层已完整落地（PR #23）——`plugin list/install/uninstall/enable/disable/update/validate` + `marketplace` 子树，持久化在 `~/.claude/plugins/`（`internal/config/pluginstore.go`），输出与 oracle 字节级一致。但 `internal/plugin/plugin.go` 的 `Manager` 仍只做配置加载/启停标记（`plugin.go:43` 标注 TODO：完整插件加载待做）——已安装插件的 skills/commands/hooks 尚不会注入会话。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 3.4.1 | ❌ 定义 `Plugin` 接口 | 未开始 | 包含 Init/Tools/Commands/Hooks 方法 |
| 3.4.2 | ❌ 实现 Go plugin 加载 | 未开始 | 使用 `plugin.Open` 或 hashicorp/go-plugin |
| 3.4.3 | ❌ 实现插件沙箱 | 未开始 | 限制插件的文件系统和网络访问 |
| 3.4.4 | ✅ 补全 `plugin` CLI 子命令 | 完成 | 2026-09-05（PR #23）：list/install/uninstall/enable/disable/update/validate + marketplace 子树 |

### 3.5 Feature Flags 系统 ❌ 未开始

**现状**（2026-08-29 核查）：`internal/config/loader.go` 定义了 `StatsFile = "statsig.json"` 常量，但没有 Flag Store / `IsEnabled` 检查逻辑。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 3.5.1 | ❌ 定义 Feature Flag 接口 | 未开始 | `IsEnabled(flag string) bool` |
| 3.5.2 | ❌ 实现本地文件 Flag Store | 未开始 | 从 `~/.claude/statsig.json` 读取 |
| 3.5.3 | ❌ 在关键路径添加 Flag 检查 | 未开始 | 工具注册、UI 功能等 |

### 3.6 类型安全修复 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
### 3.6 类型安全修复 ❌ 未开始

**现状**（2026-08-29 核查）：`internal/state/store.go:151-152` 仍是 `MCPTools []any` / `MCPCommands []any`，带 `TODO(dep)` 标注。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 3.6.1 | ❌ 定义 `MCPTool` 接口 | 未开始 | `pkg/types` 中 `ID() string` + `Name() string` |
| 3.6.2 | ❌ 定义 `MCPCommand` 接口 | 未开始 | 同上 |
| 3.6.3 | ❌ 替换 `AppState.MCPTools []any` | 未开始 | 改为 `[]types.MCPTool` |
| 3.6.4 | ❌ 替换 `AppState.MCPCommands []any` | 未开始 | 改为 `[]types.MCPCommand` |

### Phase 3 完成标准

- [ ] `claude --provider bedrock` 可正常调用 AWS Bedrock
- [ ] `claude --provider vertex` 可正常调用 GCP Vertex AI
- [ ] MCP WebSocket transport 测试通过
- [ ] 至少 5 个 Feature Flags 生效
- [ ] `AppState` 中无 `[]any` 类型

---

## Phase 4：高级功能与体验优化（v0.5.0）

**目标**：补齐原版的高级功能模块，提升用户体验。

**预估周期**：4 周

### 4.1 LSP 服务集成 ❌ 未开始

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.1.1 | ❌ 实现 LSP 客户端 | 未开始 | 使用 `go.lsp.dev/protocol` |
| 4.1.2 | ❌ 集成诊断信息到上下文 | 未开始 | 将 lint 错误作为用户上下文注入 |
| 4.1.3 | ❌ 实现 Go-to-definition | 未开始 | 增强代码导航能力 |

### 4.2 Remote/Server 模式 ❌ 未开始（`mcp serve` 是 MCP 服务端，不是 Remote 模式）

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.2.1 | ❌ 实现 HTTP API Server | 未开始 | 暴露 query/tools/session API |
| 4.2.2 | ❌ 实现 WebSocket 流式接口 | 未开始 | 实时推送 Msg 事件 |
| 4.2.3 | ⚠️ SDK 模式 | 部分 | Go 库形态天然可用（internal/ 不导出，需整理公共 API 面） |

### 4.3 Voice 语音输入 ❌ 未开始 🟢 P3

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.3.1 | ❌ 集成系统麦克风 | 未开始 | 使用 portaudio/跨平台音频库 |
| 4.3.2 | ❌ 对接 Whisper API | 未开始 | 语音转文字 |
| 4.3.3 | ❌ TUI 语音模式切换 | 未开始 | 快捷键触发录音 |

### 4.4 Vim 模式完善 ⚠️ 部分完成 🟢 P3

**现状**（2026-08-29 核查）：`internal/tui/input.go` 已有 Insert/Normal/Visual 三模式状态机与基本键处理（esc/i 等）。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.4.1 | ⚠️ 实现 Normal/Insert/Visual 模式 | 部分完成 | 三模式已存在，操作覆盖不全 |
| 4.4.2 | ❌ 常用 Vim 操作 | 未开始 | hjkl/dd/yy/p/u/w/b/0/$/G 等 |
| 4.4.3 | ❌ 命令行模式 | 未开始 | `:w` `:q` `:set` 等 |

### 4.5 Extended Thinking 完善 ⚠️ 部分完成 🟡 P1

**现状**（2026-08-29 核查）：API 层已支持 `thinking_budget_tokens`（`internal/api/client.go`）与 thinking_delta 流式事件（`internal/engine/query.go`）。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.5.1 | ❌ 实现 `--thinking` CLI 标志控制 | 未开始 | enabled/adaptive/disabled |
| 4.5.2 | ⚠️ thinking budget 传递到 API 请求 | 部分 | `MessageRequest.ThinkingBudget` 字段存在，CLI 入口未接 |
| 4.5.3 | ❌ TUI 中 thinking 内容折叠展示 | 未开始 | 可展开/收起 |

### 4.6 Cost Tracker ⚠️ 部分完成 🟡 P1

**现状**（2026-08-29 核查）：`/cost` 斜杠命令与状态栏 cost 显示已有（`internal/tui/statusbar.go`、`internal/commands/builtins.go:212`）；`--max-budget-usd` 标志已定义（`internal/bootstrap/root.go:98`）。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.6.1 | ❌ 实现按模型计价逻辑 | 未开始 | 不同模型不同费率 |
| 4.6.2 | ❌ 实现 `--max-budget-usd` 限制 | 未开始 | 标志已定义，达到预算自动停止未实现 |
| 4.6.3 | ⚠️ `/cost` 命令展示详细费用明细 | 部分 | 命令存在，输入/输出/缓存分别计算未做 |

### 4.7 数据迁移系统 ❌ 未开始 🟡 P2

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.7.1 | ❌ 定义迁移接口 | 未开始 | `Version() int` + `Migrate() error` |
| 4.7.2 | ❌ 实现配置格式迁移 | 未开始 | settings.json schema 升级 |
| 4.7.3 | ❌ 实现会话格式迁移 | 未开始 | JSONL entry 格式兼容 |

### 4.8 Analytics 服务 ⚠️ 部分完成 🟢 P3

**现状**（2026-08-29 核查）：`doctor` 子命令已实现（`internal/bootstrap/misc.go`）。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 4.8.1 | ❌ 匿名使用统计收集 | 未开始 | 遵守隐私合规 |
| 4.8.2 | ❌ 本地性能指标记录 | 未开始 | 启动时间、query 延迟等 |
| 4.8.3 | ✅ `doctor` 命令诊断信息 | 完成 | 环境检查、配置验证 |

### Phase 4 完成标准

- [ ] Extended Thinking 完整可用
- [ ] Cost Tracker 可按模型计费
- [ ] `/cost` 显示详细费用明细
- [ ] Server 模式可通过 HTTP API 调用

---

## Phase 5：生产就绪（v1.0.0）

**目标**：性能调优、安全审计、文档完善，达到生产发布标准。

**预估周期**：2 周

### 5.1 性能优化 ❌ 未开始

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 5.1.1 | ❌ 启动时间基准测试 | 未开始 | 目标 < 200ms（原版 ~65ms headless） |
| 5.1.2 | ❌ 内存占用优化 | 未开始 | 长对话场景下的 GC 压力分析 |
| 5.1.3 | ❌ 并发工具执行优化 | 未开始 | goroutine pool 调优 |
| 5.1.4 | ❌ SSE 流式解析优化 | 未开始 | 减少内存分配 |

### 5.2 安全审计 ⚠️ 部分完成

**现状**（2026-08-29 核查）：5.2.1/5.2.2 的基础防护已随权限安全增强落地（`internal/tools/shell/security.go` 命令分段检测、`internal/permissions/filesystem.go` 危险路径）；`codeql.yml` 已在 CI。正式的审计报告未产出。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 5.2.1 | ⚠️ Shell 注入防护审计 | 基础已做 | BashTool 命令注入检查（security.go），审计报告未写 |
| 5.2.2 | ⚠️ 路径遍历防护审计 | 基础已做 | FileRead/Write/Edit 路径校验 + 危险路径列表 |
| 5.2.3 | ❌ API Key 泄露防护 | 未开始 | 确保日志/错误信息不泄露密钥 |
| 5.2.4 | ❌ 依赖安全扫描 | 未开始 | `govulncheck` + `nancy`（codeql 已有） |
| 5.2.5 | ✅ OAuth Token 存储安全 | 完成 | macOS Keychain（internal/oauth） |

### 5.3 文档完善 ⚠️ 部分完成

**现状**（2026-08-29 核查）：`docs/` 下有架构文档、设计文档、QA 报告、自动生成 CONTEXT.md；仓库根有 README/CHANGELOG/CONTRIBUTING。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 5.3.1 | ⚠️ 用户指南 README 更新 | 部分 | 有 README，安装/配置/使用完整指南待补 |
| 5.3.2 | ⚠️ API 文档（GoDoc） | 部分 | 导出符号注释已养成规范，`make docs` 生成 CONTEXT.md |
| 5.3.3 | ❌ ARCHITECTURE.md 更新 | 未开始 | 目前是 docs/project/architecture.md（v2.0），根目录无该文件 |
| 5.3.4 | ⚠️ CONTRIBUTING.md 更新 | 部分 | 已存在 |
| 5.3.5 | ⚠️ CHANGELOG.md 补全 | 部分 | 已存在 |

### 5.4 发布准备 ⚠️ 部分完成

**现状**（2026-08-29 核查）：`.goreleaser.yaml` 已存在，`release.yml` workflow 已有。

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| 5.4.1 | ⚠️ goreleaser 配置验证 | 待验证 | 配置文件存在 |
| 5.4.2 | ❌ Homebrew Formula | 未开始 | macOS 安装支持 |
| 5.4.3 | ❌ Docker 镜像 | 未开始 | 容器化部署 |
| 5.4.4 | ❌ Release notes 模板 | 未开始 | 自动生成 changelog |

### Phase 5 完成标准

- [ ] 测试覆盖率 ≥ 80%
- [ ] 无 `golangci-lint` 错误
- [ ] 无已知安全漏洞
- [ ] 所有导出符号有 GoDoc 注释
- [ ] 多平台二进制文件可正常发布

---

## 各阶段依赖关系图

```
Phase 1 (安全闭环)
  │
  ├── 1.1 权限接入 ─────────────────────────────────┐
  ├── 1.2 Hook 接入 ─────────────────────────────────┤
  ├── 1.3 测试基线 ──────────────────────────────────┤
  └── 1.4 CI 强化 ───────────────────────────────────┤
                                                      │
Phase 2 (工具补全)   ←──── 依赖 Phase 1 ─────────────┘
  │
  ├── 2.1 交互工具 ──── 依赖 1.1 权限接入
  ├── 2.2 Agent 工具 ── 依赖 Coordinator（已完成）
  ├── 2.3 斜杠命令 ──── 无额外依赖
  └── 2.4 CLI 命令 ──── 无额外依赖
       │
Phase 3 (Provider/MCP)  ←── 可与 Phase 2 后半段并行
  │
  ├── 3.1 Bedrock ────── 无依赖
  ├── 3.2 Vertex ─────── 无依赖
  ├── 3.3 MCP WS ─────── 无依赖
  ├── 3.4 插件系统 ───── 依赖 2.4 plugin CLI
  └── 3.5 Feature Flags ── 无依赖
       │
Phase 4 (高级功能)   ←── 依赖 Phase 2 + 3
  │
  ├── 4.1 LSP ──────────── 可独立开发
  ├── 4.2 Remote ────────── 依赖引擎完整
  ├── 4.5 Thinking ──────── 依赖 API 层完整
  └── 4.6 Cost Tracker ──── 依赖 Provider 完整
       │
Phase 5 (生产就绪)   ←── 依赖所有 Phase
```

---

## 风险与缓解

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|----------|
| 权限系统接入引擎改动面大 | 可能引入回归 Bug | 中 | 先写集成测试，再改引擎；用 Feature Flag 控制灰度 |
| Bedrock/Vertex 认证复杂 | 延期 2+ 周 | 高 | 优先使用官方 SDK 而非自写；接受最小可用实现 |
| 原版 API 变更 | 功能不对等 | 低 | 定期对比原版 changelog；保持接口抽象层 |
| 插件系统设计复杂 | 可能过度工程 | 中 | 先实现最小 Go plugin.Open 方案；不做 WASM |
| Go 版 TUI 体验差异 | 用户反馈负面 | 中 | BubbleTea 生态成熟，可复用组件；必要时自定义渲染器 |

---

## 度量标准

### 质量指标

| 指标 | v0.2.0 目标 | v0.5.0 目标 | v1.0.0 目标 |
|------|-------------|-------------|-------------|
| 测试覆盖率 | ≥ 60% | ≥ 75% | ≥ 80% |
| golangci-lint 错误 | 0 | 0 | 0 |
| 安全漏洞（govulncheck） | 0 critical | 0 high | 0 all |
| GoDoc 覆盖率（导出符号） | ≥ 70% | ≥ 90% | 100% |

### 功能对等指标

| 指标 | 当前 | v0.3.0 | v0.5.0 | v1.0.0 |
|------|------|--------|--------|--------|
| 工具实现数 | 11/22 | 22/22 | 22/22 | 22/22 |
| CLI 子命令 | 7/27 | 20/27 | 25/27 | 27/27 |
| 斜杠命令 | 14/18 | 18/18 | 18/18 | 18/18 |
| Provider 支持 | 2/4 | 2/4 | 4/4 | 4/4 |
| MCP Transport | 3/4 | 3/4 | 4/4 | 4/4 |

### 性能指标

| 指标 | 目标 |
|------|------|
| 冷启动时间（headless -p） | < 200ms |
| 冷启动时间（interactive） | < 500ms |
| 内存占用（空闲） | < 50MB |
| 内存占用（100 轮对话后） | < 200MB |
| 首个 token 延迟（不含 API） | < 50ms |

---

## 附录：快速参考

### 文件改动热力图（Phase 1-2）

```
internal/
├── engine/
│   ├── engine.go          ██████  [新增 Checker + Dispatcher 字段]
│   ├── orchestration.go   ████████████  [权限检查 + Hook 调用]
│   ├── query.go           ████  [Stop Hook]
│   └── msg.go             ████  [新增 Msg 类型]
├── bootstrap/
│   └── wire.go            ██████  [构建 Checker + Dispatcher]
├── tools/
│   ├── interact/          ████████████  [6 个工具实现]
│   ├── agent/             ██████  [2 个工具实现]
│   └── mcp/               ██████  [3 个工具实现]
├── commands/
│   └── builtins.go        ████████  [8 个命令补全]
├── state/
│   └── store.go           ████  [Todo 状态 + 类型修复]
└── tui/
    ├── permissions.go     ████  [权限 Msg 处理]
    └── update.go          ██  [新 Msg 类型分发]
```

### 优先级标记说明

- 🔴 **P0（阻塞性）**：当前版本必须完成，阻碍核心功能或存在安全隐患
- 🟡 **P1（重要）**：应在目标版本完成，影响用户体验或功能完整度
- 🟢 **P2/P3（可选）**：增强功能，可根据资源情况延后
