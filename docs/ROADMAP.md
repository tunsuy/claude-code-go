# Claude Code Go — Roadmap

> 最后更新：2026-04-21
>
> 本文档基于与 claude-code-main（TypeScript 原版）的全面对比分析，规划 claude-code-go 项目从当前状态（~65% 完成度）到生产可用的完整路径。

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
│                   claude-code-go v0.1.0                     │
│                                                             │
│  ✅ 已完成（可用）          ⚠️ 部分完成            ❌ 缺失  │
│  ─────────────────     ──────────────────     ────────────  │
│  • 核心引擎 query 循环   • 工具系统 (11/22)    • 权限接入   │
│  • API Direct 客户端     • CLI 子命令 (7/27)    • Hook 接入  │
│  • OpenAI 兼容客户端     • MCP (3/4 transport)  • LSP 服务   │
│  • 上下文压缩三件套      • Bedrock/Vertex       • 插件系统   │
│  • TUI (BubbleTea)      • 插件系统 (空壳)      • Feature Flag│
│  • OAuth 认证            • 测试覆盖率           • Voice/Vim  │
│  • 配置加载              •                      • Analytics  │
│  • 会话持久化            •                      • Remote 模式│
│  • 协调器框架            •                      • 数据迁移   │
│  • 权限管线（未接入）    •                      •            │
└─────────────────────────────────────────────────────────────┘

完成度: ████████████░░░░░░░░ 65%
```

---

## 版本规划总览

```
时间线（预估）
─────────────────────────────────────────────────────────────────────────

v0.1.0 (当前)          v0.2.0             v0.3.0             v0.4.0             v0.5.0          v1.0.0
  │                      │                  │                  │                  │               │
  ├── 核心引擎 ✅        ├── 权限接入       ├── 工具补全       ├── Bedrock/Vertex  ├── LSP 服务     ├── 生产就绪
  ├── TUI ✅             ├── Hook 接入      ├── CLI 补全       ├── MCP WebSocket   ├── Voice 输入   ├── 性能调优
  ├── API 客户端 ✅      ├── 测试基线       ├── Agent 集成     ├── 插件系统        ├── Remote 模式  ├── 安全审计
  │                      ├── CI/CD 强化     ├── 斜杠命令       ├── Feature Flags   ├── Vim 模式     ├── 文档完善
  │                      │                  │                  │                  │               │
──┼──────────────────────┼──────────────────┼──────────────────┼──────────────────┼───────────────┼──
  现在                   +3 周              +3 周              +4 周              +4 周           +2 周
                                                                                            总计 ~16 周
```

---

## Phase 1：核心安全闭环（v0.2.0）

**目标**：让工具执行经过权限检查和 Hook 系统，消除安全隐患。这是最高优先级，因为当前所有工具调用都是**无权限控制**的直接执行。

**预估周期**：3 周

### 1.1 权限系统接入引擎 🔴 P0

**现状**：`internal/permissions/checker.go` 已实现完整的 9 步权限管线，但 `internal/engine/orchestration.go` 的 `executeOneTool` 直接执行工具，未调用任何权限检查。

**任务清单**：

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1.1.1 | 将 `permissions.Checker` 注入 `engine.Config` | `internal/engine/engine.go` | 新增 `Checker` 字段 |
| 1.1.2 | 在 `executeOneTool` 中调用权限检查 | `internal/engine/orchestration.go` | 在 `ValidateInput` 之后、`t.Call()` 之前插入 |
| 1.1.3 | 权限 Ask 请求通过 `Msg` channel 传递到 TUI | `internal/engine/msg.go` | 新增 `MsgTypePermissionAsk` / `MsgTypePermissionResponse` |
| 1.1.4 | TUI 处理权限对话框 | `internal/tui/permissions.go` | 已有框架，需对接引擎 Msg |
| 1.1.5 | 在 `bootstrap/wire.go` 中构建 Checker 并注入 | `internal/bootstrap/wire.go` | 组装 Hook Dispatcher + Checker |

**架构改动**：

```
executeOneTool (当前)              executeOneTool (目标)
─────────────────────              ─────────────────────
1. registry.Get(name)              1. registry.Get(name)
2. ValidateInput                   2. ValidateInput
3. t.Call()          ← 直接执行     3. checker.CanUseTool()    ← 新增
                                   4. if Ask → 发送 Msg → 等待响应
                                   5. if Deny → 返回错误结果
                                   6. t.Call()
                                   7. hookDispatcher.Run(PostToolUse) ← 新增
```

### 1.2 Hook 系统接入引擎 🔴 P0

**现状**：`internal/hooks/hooks.go` 已实现 `Dispatcher`，但引擎中没有调用。

**任务清单**：

| # | 任务 | 文件 | 说明 |
|---|------|------|------|
| 1.2.1 | 将 `hooks.Dispatcher` 注入 `engine.Config` | `internal/engine/engine.go` | 新增 `HookDispatcher` 字段 |
| 1.2.2 | 在工具执行前调用 `PreToolUse` Hook | `internal/engine/orchestration.go` | Hook 可以阻止工具执行 |
| 1.2.3 | 在工具执行后调用 `PostToolUse` Hook | `internal/engine/orchestration.go` | Hook 可以修改结果 |
| 1.2.4 | 在查询循环结束时调用 `Stop` Hook | `internal/engine/query.go` | 通知会话结束 |
| 1.2.5 | 在 `bootstrap/wire.go` 中从 settings 构建 Dispatcher | `internal/bootstrap/wire.go` | 从配置中读取 Hook 定义 |

### 1.3 测试覆盖率基线 🟡 P1

**现状**：`coverage.out` 显示所有生产代码覆盖率为 0。

**任务清单**：

| # | 任务 | 说明 |
|---|------|------|
| 1.3.1 | 权限接入后编写集成测试 | 覆盖 Allow/Deny/Ask 三种路径 |
| 1.3.2 | Hook 调度集成测试 | 覆盖 PreToolUse Block/Modify 路径 |
| 1.3.3 | 引擎 query 循环端到端测试 | 使用 mock API client，验证完整循环 |
| 1.3.4 | 设立覆盖率门槛 | CI 中要求核心包 ≥ 60%，逐步提升 |

### 1.4 CI/CD 强化 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
| 1.4.1 | 在 CI 中运行 `golangci-lint` | 当前 `.github/workflows/ci.yml` 存在但需验证 lint 步骤 |
| 1.4.2 | 集成 `go test -race` | 检测并发竞态条件 |
| 1.4.3 | 添加覆盖率报告上传 | codecov.yml 已存在，确保实际上传 |

### Phase 1 完成标准

- [ ] 所有写操作工具执行前经过权限检查
- [ ] Hook PreToolUse / PostToolUse / Stop 全部接入
- [ ] 核心包测试覆盖率 ≥ 60%
- [ ] CI 通过 lint + race + test

---

## Phase 2：工具与命令补全（v0.3.0）

**目标**：补齐 11 个未实现的工具和 20 个 CLI 子命令，实现与原版功能对等的工具集。

**预估周期**：3 周

### 2.1 交互工具实现 🔴 P0

| # | 工具 | 文件 | 依赖 |
|---|------|------|------|
| 2.1.1 | `TodoWrite` | `internal/tools/interact/interact.go` | 需要 `AppStateStore` 中新增 Todo 状态 |
| 2.1.2 | `AskUserQuestion` | `internal/tools/interact/interact.go` | 需要通过 Msg channel 与 TUI 交互 |
| 2.1.3 | `EnterPlanMode` / `ExitPlanMode` | `internal/tools/interact/worktree.go` | 需要 `AppStateStore` 计划模式状态机 |
| 2.1.4 | `EnterWorktree` / `ExitWorktree` | `internal/tools/interact/worktree.go` | 需要 git worktree 管理器 |

**TodoWrite 设计要点**：
```go
// internal/state/store.go 新增
type TodoItem struct {
    ID      string `json:"id"`
    Status  string `json:"status"`  // pending | in_progress | completed | cancelled
    Content string `json:"content"`
}

// AppState 新增
Todos []TodoItem `json:"todos,omitempty"`
```

### 2.2 Agent/MCP 工具实现 🔴 P0

| # | 工具 | 文件 | 依赖 |
|---|------|------|------|
| 2.2.1 | `Agent` tool | `internal/tools/agent/agent.go` | 集成 `coordinator.Coordinator` |
| 2.2.2 | `SendMessage` tool | `internal/tools/agent/sendmessage.go` | 集成 `coordinator.SendMessage` |
| 2.2.3 | `MCPProxyTool` | `internal/tools/mcp/mcp.go` | 集成 `mcp.Pool` + `MCPClient.CallTool` |
| 2.2.4 | `ListMcpResources` | `internal/tools/mcp/mcp.go` | 集成 `MCPClient.ListResources` |
| 2.2.5 | `ReadMcpResource` | `internal/tools/mcp/mcp.go` | 集成 `MCPClient.ReadResource` |

**Agent Tool 设计要点**：
```go
func (a *AgentTool) Call(input Input, uctx *UseContext, onProgress OnProgressFn) (*Result, error) {
    // 1. 解析 SpawnRequest
    // 2. coordinator.SpawnAgent(ctx, req)
    // 3. coordinator.Subscribe(agentID) 等待完成
    // 4. 返回 agent 结果
}
```

### 2.3 斜杠命令补全 🟡 P1

| # | 命令 | 当前状态 | 实现要点 |
|---|------|----------|----------|
| 2.3.1 | `/config` | 占位 | 打开配置编辑对话框（TUI modal） |
| 2.3.2 | `/mcp` | 占位 | 调用 `mcp.Pool.GetAll()` 展示连接状态 |
| 2.3.3 | `/resume` | 占位 | 调用 `session.Resume()` 恢复会话 |
| 2.3.4 | `/terminal-setup` | 占位 | 生成 shell 补全脚本和快捷键绑定 |
| 2.3.5 | `/review` | 仅发文本 | 自动执行 `git diff` 并注入 |
| 2.3.6 | `/commit` | 仅发文本 | 自动检测 staged changes 并格式化 |
| 2.3.7 | `/diff` | 仅发文本 | 自动执行并展示 `git diff` 输出 |
| 2.3.8 | `/init` | 仅发文本 | 扫描项目结构后生成 CLAUDE.md 模板 |

### 2.4 CLI 子命令补全 🟡 P1

按优先级排序：

**高优先级（用户直接使用）**：

| # | 命令 | 文件 | 说明 |
|---|------|------|------|
| 2.4.1 | `mcp add` | `internal/bootstrap/mcp.go` | 添加 MCP 服务器配置到 settings.json |
| 2.4.2 | `mcp remove` | 同上 | 移除 MCP 服务器配置 |
| 2.4.3 | `mcp list` | 同上 | 列出已配置的 MCP 服务器 |
| 2.4.4 | `mcp get` | 同上 | 查看单个 MCP 服务器详情 |

**中优先级**：

| # | 命令 | 说明 |
|---|------|------|
| 2.4.5 | `update` | 检查并下载最新版本 |
| 2.4.6 | `install` | 安装 shell 集成 |
| 2.4.7 | `mcp add-json` | 从 JSON 批量添加 MCP |
| 2.4.8 | `mcp add-from-claude-desktop` | 从 Claude Desktop 导入 MCP 配置 |

**低优先级（可延后）**：

| # | 命令 | 说明 |
|---|------|------|
| 2.4.9 | `agents list/add/remove` | 自定义子代理管理 |
| 2.4.10 | `plugin list/install/uninstall/...` | 依赖插件系统完善 |
| 2.4.11 | `mcp reset-project-choices` | 重置 MCP 项目选择 |

### Phase 2 完成标准

- [ ] 22/22 工具全部可执行（非占位）
- [ ] `/review`、`/commit`、`/diff`、`/init` 有专属逻辑
- [ ] `mcp add/remove/list/get` 可正常操作配置文件
- [ ] Agent tool 能通过 Coordinator 启动子代理

---

## Phase 3：多 Provider 与 MCP 完善（v0.4.0）

**目标**：支持真实的 AWS Bedrock / GCP Vertex 调用，完善 MCP 协议实现，引入 Feature Flags 和插件系统。

**预估周期**：4 周

### 3.1 Bedrock Provider 实现 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
| 3.1.1 | 引入 `aws-sdk-go-v2` 依赖 | 用于 AWS Signature V4 签名 |
| 3.1.2 | 实现 `bedrockClient.Stream` / `Complete` | 使用 Bedrock Runtime API 端点 |
| 3.1.3 | 实现 AWS 凭证链（env → profile → IMDS） | 使用 `aws-sdk-go-v2/config` |
| 3.1.4 | 添加 `awsAuthRefresh` 支持 | 从 settings.json 读取刷新命令 |
| 3.1.5 | 请求/响应格式转换 | Bedrock API 的 body 格式与 Direct API 不同 |

### 3.2 Vertex Provider 实现 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
| 3.2.1 | 引入 `golang.org/x/oauth2/google` 依赖 | 用于 GCP 认证 |
| 3.2.2 | 实现 `vertexClient.Stream` / `Complete` | 使用 Vertex AI 端点 |
| 3.2.3 | 实现 GCP ADC（Application Default Credentials） | 自动发现凭证 |
| 3.2.4 | 添加 `gcpAuthRefresh` 支持 | 从 settings.json 读取刷新命令 |

### 3.3 MCP WebSocket Transport 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
| 3.3.1 | 引入 `nhooyr.io/websocket` 依赖 | 轻量级 WebSocket 库 |
| 3.3.2 | 实现 `wsTransport` | 实现 `Transport` 接口 |
| 3.3.3 | 自动重连逻辑 | 连接断开后指数退避重连 |
| 3.3.4 | 集成测试 | 使用 mock WS server 测试 |

### 3.4 插件系统实现 🟡 P2

| # | 任务 | 说明 |
|---|------|------|
| 3.4.1 | 定义 `Plugin` 接口 | 包含 Init/Tools/Commands/Hooks 方法 |
| 3.4.2 | 实现 Go plugin 加载 | 使用 `plugin.Open` 或 hashicorp/go-plugin |
| 3.4.3 | 实现插件沙箱 | 限制插件的文件系统和网络访问 |
| 3.4.4 | 补全 `plugin` CLI 子命令 | list/install/uninstall/enable/disable |

### 3.5 Feature Flags 系统 🟡 P2

| # | 任务 | 说明 |
|---|------|------|
| 3.5.1 | 定义 Feature Flag 接口 | `IsEnabled(flag string) bool` |
| 3.5.2 | 实现本地文件 Flag Store | 从 `~/.claude/statsig.json` 读取 |
| 3.5.3 | 在关键路径添加 Flag 检查 | 工具注册、UI 功能等 |

### 3.6 类型安全修复 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
| 3.6.1 | 定义 `MCPTool` 接口 | `pkg/types` 中 `ID() string` + `Name() string` |
| 3.6.2 | 定义 `MCPCommand` 接口 | 同上 |
| 3.6.3 | 替换 `AppState.MCPTools []any` | 改为 `[]types.MCPTool` |
| 3.6.4 | 替换 `AppState.MCPCommands []any` | 改为 `[]types.MCPCommand` |

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

### 4.1 LSP 服务集成 🟡 P2

| # | 任务 | 说明 |
|---|------|------|
| 4.1.1 | 实现 LSP 客户端 | 使用 `go.lsp.dev/protocol` |
| 4.1.2 | 集成诊断信息到上下文 | 将 lint 错误作为用户上下文注入 |
| 4.1.3 | 实现 Go-to-definition | 增强代码导航能力 |

### 4.2 Remote/Server 模式 🟡 P2

| # | 任务 | 说明 |
|---|------|------|
| 4.2.1 | 实现 HTTP API Server | 暴露 query/tools/session API |
| 4.2.2 | 实现 WebSocket 流式接口 | 实时推送 Msg 事件 |
| 4.2.3 | SDK 模式 | 可作为 Go 库嵌入其他项目 |

### 4.3 Voice 语音输入 🟢 P3

| # | 任务 | 说明 |
|---|------|------|
| 4.3.1 | 集成系统麦克风 | 使用 portaudio/跨平台音频库 |
| 4.3.2 | 对接 Whisper API | 语音转文字 |
| 4.3.3 | TUI 语音模式切换 | 快捷键触发录音 |

### 4.4 Vim 模式完善 🟢 P3

| # | 任务 | 说明 |
|---|------|------|
| 4.4.1 | 实现 Normal/Insert/Visual 模式 | 当前只有 toggle 开关 |
| 4.4.2 | 常用 Vim 操作 | hjkl/dd/yy/p/u/w/b/0/$/G 等 |
| 4.4.3 | 命令行模式 | `:w` `:q` `:set` 等 |

### 4.5 Extended Thinking 完善 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
| 4.5.1 | 实现 `--thinking` CLI 标志控制 | enabled/adaptive/disabled |
| 4.5.2 | thinking budget 传递到 API 请求 | `MessageRequest.ThinkingBudget` |
| 4.5.3 | TUI 中 thinking 内容折叠展示 | 可展开/收起 |

### 4.6 Cost Tracker 🟡 P1

| # | 任务 | 说明 |
|---|------|------|
| 4.6.1 | 实现按模型计价逻辑 | 不同模型不同费率 |
| 4.6.2 | 实现 `--max-budget-usd` 限制 | 达到预算自动停止 |
| 4.6.3 | `/cost` 命令展示详细费用明细 | 输入/输出/缓存分别计算 |

### 4.7 数据迁移系统 🟡 P2

| # | 任务 | 说明 |
|---|------|------|
| 4.7.1 | 定义迁移接口 | `Version() int` + `Migrate() error` |
| 4.7.2 | 实现配置格式迁移 | settings.json schema 升级 |
| 4.7.3 | 实现会话格式迁移 | JSONL entry 格式兼容 |

### 4.8 Analytics 服务 🟢 P3

| # | 任务 | 说明 |
|---|------|------|
| 4.8.1 | 匿名使用统计收集 | 遵守隐私合规 |
| 4.8.2 | 本地性能指标记录 | 启动时间、query 延迟等 |
| 4.8.3 | `doctor` 命令诊断信息 | 环境检查、配置验证 |

### Phase 4 完成标准

- [ ] Extended Thinking 完整可用
- [ ] Cost Tracker 可按模型计费
- [ ] `/cost` 显示详细费用明细
- [ ] Server 模式可通过 HTTP API 调用

---

## Phase 5：生产就绪（v1.0.0）

**目标**：性能调优、安全审计、文档完善，达到生产发布标准。

**预估周期**：2 周

### 5.1 性能优化

| # | 任务 | 说明 |
|---|------|------|
| 5.1.1 | 启动时间基准测试 | 目标 < 200ms（原版 ~65ms headless） |
| 5.1.2 | 内存占用优化 | 长对话场景下的 GC 压力分析 |
| 5.1.3 | 并发工具执行优化 | goroutine pool 调优 |
| 5.1.4 | SSE 流式解析优化 | 减少内存分配 |

### 5.2 安全审计

| # | 任务 | 说明 |
|---|------|------|
| 5.2.1 | Shell 注入防护审计 | BashTool 的命令注入检查 |
| 5.2.2 | 路径遍历防护审计 | FileRead/Write/Edit 的路径校验 |
| 5.2.3 | API Key 泄露防护 | 确保日志/错误信息不泄露密钥 |
| 5.2.4 | 依赖安全扫描 | `govulncheck` + `nancy` |
| 5.2.5 | OAuth Token 存储安全 | macOS Keychain / Linux secret-service |

### 5.3 文档完善

| # | 任务 | 说明 |
|---|------|------|
| 5.3.1 | 用户指南 README 更新 | 安装、配置、使用完整指南 |
| 5.3.2 | API 文档（GoDoc） | 所有导出符号有文档注释 |
| 5.3.3 | ARCHITECTURE.md 更新 | 反映最终架构 |
| 5.3.4 | CONTRIBUTING.md 更新 | 开发流程、测试要求 |
| 5.3.5 | CHANGELOG.md 补全 | 各版本变更记录 |

### 5.4 发布准备

| # | 任务 | 说明 |
|---|------|------|
| 5.4.1 | goreleaser 配置验证 | 多平台交叉编译 |
| 5.4.2 | Homebrew Formula | macOS 安装支持 |
| 5.4.3 | Docker 镜像 | 容器化部署 |
| 5.4.4 | Release notes 模板 | 自动生成 changelog |

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
