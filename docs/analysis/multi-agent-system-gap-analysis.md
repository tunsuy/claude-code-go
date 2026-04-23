# 多Agent系统差异分析报告

> **文档状态**：功能规划  
> **创建日期**：2026-04-23  
> **跟踪 Issue**：#TBD（待创建）  
> **相关文档**：[`multi-agent-system-design.md`](./origin/multi-agent-system-design.md)

---

## 概述

本文档基于对 Claude Code TypeScript 原版多Agent系统的深入分析（见 `origin/multi-agent-system-design.md`），
与当前 Go 版本实现进行逐项对比，识别差距并规划改进路径。

原版多Agent系统包含 **3层递进的协作模式**（同步子Agent、异步后台Agent、Swarm团队模式），
**5种内置Agent类型**，完整的**文件邮箱通信**、**权限同步**、**执行后端**和**UI呈现**体系。
当前 Go 版本仅实现了 Layer 1（同步子Agent）的基本框架，Layer 2 和 Layer 3 几乎全部缺失。

---

## 目录

- [一、已实现功能](#一已实现功能)
- [二、未实现/不完整功能](#二未实现不完整功能)
- [三、架构差异总结](#三架构差异总结)
- [四、优先改进计划](#四优先改进计划)
- [五、实现建议](#五实现建议)
- [六、代码质量问题](#六代码质量问题)

---

## 一、已实现功能

### 1.1 "Tool-as-Agent" 范式 ✅

Go 版本正确实现了核心设计理念——Agent 本身是一个 Tool，由主 Agent（LLM）通过调用 `AgentTool` 来创建子Agent：

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| `AgentTool` 工具接口 | ✅ 完成 | `internal/tools/agent/agent.go` |
| `AgentInput` 输入类型（prompt/system_prompt/allowed_tools/max_turns） | ✅ 完成 | `internal/tools/agent/agent.go` |
| `AgentOutput` 输出类型 | ✅ 完成 | `internal/tools/agent/agent.go` |
| `IsConcurrencySafe=true`（支持并行spawn） | ✅ 完成 | `internal/tools/agent/agent.go:75` |
| 工具注册 | ✅ 完成 | `internal/bootstrap/tools.go` |

**并发安全设计**（与TS原版一致）：
```go
// internal/tools/agent/agent.go
func (t *agentTool) IsConcurrencySafe(_ tools.Input) bool { return true }
// 引擎可以在同一LLM响应轮次中并行spawn多个子Agent
```

### 1.2 Coordinator 基础设施 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| `AgentCoordinator` 接口 | ✅ 完成 | `internal/tools/tool.go` |
| `Coordinator` 实现（goroutine + RWMutex） | ✅ 完成 | `internal/coordinator/coordinator.go` |
| `SpawnAgent` — 非阻塞创建（goroutine） | ✅ 完成 | `internal/coordinator/coordinator.go` |
| `WaitForAgent` — 等待完成 | ✅ 完成 | `internal/coordinator/coordinator.go` |
| `StopAgent` — 终止Agent | ✅ 完成 | `internal/coordinator/coordinator.go` |
| `GetAgentResult` — 获取结果 | ✅ 完成 | `internal/coordinator/coordinator.go` |
| `ListAgents` — 列出所有Agent | ✅ 完成 | `internal/coordinator/coordinator.go` |
| `SendMessage` — Agent间消息传递 | ✅ 完成 | `internal/coordinator/coordinator.go` |

**并发控制**：
```go
// internal/coordinator/coordinator.go
// SpawnAgent: 使用 sync.RWMutex 保护 agents map
// 每个 agent entry 使用独立 sync.Mutex
// runAgentLoop: 在独立 goroutine 中运行，不阻塞主循环
```

### 1.3 Task 工具集 ✅

| 工具 | 状态 | 文件位置 |
|------|------|----------|
| `TaskCreateTool` — 创建后台任务 | ✅ 完成 | `internal/tools/tasks/tasks.go` |
| `TaskGetTool` — 获取任务状态 | ✅ 完成 | `internal/tools/tasks/tasks.go` |
| `TaskListTool` — 列出任务（支持状态过滤） | ✅ 完成 | `internal/tools/tasks/tasks.go` |
| `TaskUpdateTool` — 更新任务描述/状态 | ✅ 完成 | `internal/tools/tasks/tasks.go` |
| `TaskStopTool` — 停止任务 | ✅ 完成 | `internal/tools/tasks/tasks.go` |
| `TaskOutputTool` — 读取任务输出（含 since 偏移） | ✅ 完成 | `internal/tools/tasks/tasks.go` |
| `TaskStatus` 枚举（pending/running/completed/failed/stopped） | ✅ 完成 | `internal/tools/tasks/tasks.go` |

**与原版对比**：Go 版本任务工具集基本覆盖原版 `TaskXxxTool` 体系，
`TaskCreateTool` 使用 Agent Coordinator 的 `SpawnAgent` 作为底层实现，语义一致。

### 1.4 SendMessage 工具（基础版）⚠️

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| `SendMessageTool` 工具接口 | ✅ 完成 | `internal/tools/agent/sendmessage.go` |
| `SendMessageInput`（agent_id + content） | ✅ 完成 | `internal/tools/agent/sendmessage.go` |
| 通过 Coordinator.SendMessage 路由消息 | ✅ 完成 | `internal/tools/agent/sendmessage.go` |
| `IsConcurrencySafe=false`（串行发送） | ✅ 完成 | `internal/tools/agent/sendmessage.go` |

---

## 二、未实现/不完整功能

### 2.1 ❌ 内置 Agent 类型（Built-in Agents）— 最大功能差距之一

**原版实现**：5种专门化内置Agent，每种有精心设计的系统提示词、工具约束和模型选择：

```typescript
// TS原版: 5种内置Agent类型
// src/tools/AgentTool/built-in/

// 1. Explore Agent — 严格只读，Glob/Grep/FileRead/Bash(只读)，外部用户使用Haiku
// 2. Plan Agent    — 只读，输出结构化实施计划，inherit模型
// 3. Verification Agent — 禁止修改项目文件，对抗性探测，/tmp允许写入
// 4. General Purpose Agent — 全工具集('*')，通用任务执行
// 5. Claude Code Guide Agent — WebFetch/WebSearch，专注Claude文档
```

**当前 Go 实现**：
```go
// internal/tools/agent/agent.go
// 只有一个通用的 AgentTool，无 agent_type 参数
type AgentInput struct {
    Prompt       string   `json:"prompt"`
    SystemPrompt string   `json:"system_prompt,omitempty"`
    AllowedTools []string `json:"allowed_tools,omitempty"`
    MaxTurns     *int     `json:"max_turns,omitempty"`
    // ❌ 缺少: AgentType string `json:"agent_type,omitempty"`
    // ❌ 缺少: Background bool   `json:"background,omitempty"`
    // ❌ 缺少: TeamName   string `json:"team_name,omitempty"`
    // ❌ 缺少: AgentName  string `json:"agent_name,omitempty"`
}
```

**差距分析**：
- ❌ 没有 `agent_type` 参数（Explore/Plan/Verification/General/Guide）
- ❌ 没有内置Agent的系统提示词模板
- ❌ 没有Agent特定的工具过滤（Explore严格只读、Verification禁写项目目录等）
- ❌ 没有基于Agent类型的模型选择策略（Explore外部用户→Haiku，Plan→inherit等）
- ❌ 没有 `whenToUse` 描述供主Agent决策何时调用

**建议优先级**：🔴 P1（核心功能差距，影响多Agent任务质量）

**修复难度**：高（~800 行代码，需要设计5种Agent的系统提示词）

---

### 2.2 ❌ 自定义 Agent 定义（.claude/agents/）

**原版实现**：
```typescript
// src/tools/AgentTool/loadAgentsDir.ts
// 从7个来源加载Agent定义（优先级从高到低）:
// 1. User agents:    ~/.claude/agents/
// 2. Project agents: .claude/agents/
// 3. Local agents:   本地配置
// 4. Managed agents: 策略配置
// 5. Plugin agents:  插件提供
// 6. CLI arg agents: 命令行参数
// 7. Built-in agents: 内置

// Agent定义支持 Markdown 或 JSON 格式:
type AgentDefinition = {
  agentType: string
  description?: string
  whenToUse?: string
  prompt?: string
  tools?: string[]              // '*' 表示全部
  disallowedTools?: string[]
  model?: string                // 'inherit' | 'sonnet' | 'haiku' | 'opus'
  permissionMode?: PermissionMode
  mcpServers?: AgentMcpServerSpec[]
  hooks?: HooksSettings
  maxTurns?: number
  background?: boolean
  isolation?: 'worktree' | 'remote'
}
```

**当前 Go 实现**：
- **完全没有实现**
- 没有 `loadAgentsDir` 等效函数
- 没有 Agent 定义的 Markdown/JSON 加载
- 没有 `.claude/agents/` 目录支持

**建议优先级**：🟡 P2（扩展性功能）

**修复难度**：中（~400 行代码）

---

### 2.3 ❌ 异步后台 Agent 模式（Layer 2）

**原版实现**：
```typescript
// AgentTool.tsx — background: true 触发异步模式
if (isBackgroundAgent) {
  const taskId = registerAsyncAgent(agentId, description)
  runAsyncAgentLifecycle(context, {  // ← 不阻塞主循环
    taskId,
    abortController,
    ...
  })
  return { type: 'result', content: `Task ${taskId} started in background` }
}

// 异步生命周期管理
async function runAsyncAgentLifecycle(context, config) {
  await runWithAgentContext(agentId, () => runAgent(context, config))
  completeAsyncAgent(taskId, result)  // 更新状态 + 发通知 + 生成摘要
}
```

**当前 Go 实现**：
```go
// internal/tools/agent/agent.go
func (t *agentTool) Call(...) (*tools.Result, error) {
    agentID, _ := ctx.Coordinator.SpawnAgent(...)   // 非阻塞 goroutine spawn
    result, _ := ctx.Coordinator.WaitForAgent(...)  // ← 立即阻塞等待
    // ❌ 没有 background: true 的异步模式
    // AgentTool.Call 始终是同步阻塞的，无法实现真正的"fire-and-forget"
}
```

**差距分析**：
- ❌ 没有 `background: true` 参数支持
- ❌ `AgentTool.Call` 始终阻塞（spawn + 立即 wait），无法作为后台任务返回
- ❌ 没有 `runAsyncAgentLifecycle` 等效机制
- ❌ 没有后台Agent的进度追踪（通知、摘要生成）
- ❌ 没有 `AppState.tasks` 中的任务注册机制

**建议优先级**：🔴 P1（核心功能，Coordinator模式必须）

**修复难度**：中（~300 行代码）

---

### 2.4 ❌ Swarm/团队模式（Layer 3）— 最大架构差距

**原版实现**：完整的多进程/多上下文协作基础设施：

```typescript
// TeamCreateTool — 创建团队配置文件
writeTeamFile(`~/.claude/teams/{team_name}/team.json`)

// AgentTool(team_name=...) — 将Agent加入团队
detectAndGetBackend() → 选择执行后端:
  ├── tmux → 新pane启动独立Claude进程
  ├── iTerm2 → 新面板启动独立Claude进程
  └── in-process → AsyncLocalStorage隔离

// SendMessage 支持团队内广播和名称路由
{ to: "researcher" }  // 按名字发
{ to: "*" }           // 广播
```

**当前 Go 实现**：
- ❌ 没有 `TeamCreateTool` / `TeamDeleteTool`
- ❌ 没有团队配置文件（`~/.claude/teams/`）
- ❌ 没有多进程/多上下文执行后端
- ❌ `AgentTool` 不支持 `team_name` 参数
- ❌ `SendMessageTool` 只支持 `agent_id` 路由，不支持名称路由或广播

**建议优先级**：🟢 P3（高级功能，需大量前置工作）

**修复难度**：极高（~3000 行代码，需要全新基础设施）

---

### 2.5 ❌ 执行后端架构（tmux/iTerm2/in-process）

**原版实现**：
```typescript
// src/utils/swarm/backends/types.ts
interface PaneBackend {
  type: 'tmux' | 'iterm2' | 'in-process'
  isAvailable(): Promise<boolean>
  createTeammatePaneInSwarmView(name, color): Promise<CreatePaneResult>
  sendCommand(paneId, command): Promise<void>
  killPane(paneId): Promise<void>
}

// 后端选择优先级:
// 1. 用户保存的偏好 → 2. 非交互模式→in-process
// 3. tmux内 → 4. iTerm2 → 5. tmux已安装 → 6. in-process
```

**当前 Go 实现**：
- Go版本只有 in-process（goroutine）模式，没有抽象后端接口
- 没有 tmux 后端支持
- 没有 iTerm2 后端支持
- 没有后端检测和自动选择逻辑

**建议优先级**：🟢 P3（Swarm 功能的一部分）

**修复难度**：极高（依赖 Swarm 整体架构）

---

### 2.6 ❌ 权限同步机制（Permission Sync）

**原版实现**：
```
Worker Agent                   Leader Agent                  User
     │                              │                          │
     │── permission_request ────────►│                          │
     │   {toolName, input, id}      │── 展示权限提示 ──────────►│
     │                              │◄── approve/deny ─────────│
     │◄── permission_response ──────│                          │
     │   {id, decision}             │                          │
```

Worker Agent 遇到需要审批的操作时，通过邮箱向 Leader 发送 `permission_request`；
Leader 在其 UI 中展示给用户；响应通过邮箱返回。

**当前 Go 实现**：
- ❌ 没有 Agent 间的权限请求路由机制
- ❌ 没有 `permissionSync.ts` 等效模块
- ❌ 每个 goroutine Agent 运行在同一权限上下文中（无隔离）

**建议优先级**：🟢 P3（Swarm 功能配套）

**修复难度**：高（依赖邮箱系统和 Leader/Worker 角色区分）

---

### 2.7 ❌ 文件邮箱通信系统（Teammate Mailbox）

**原版实现**：
```
存储路径：~/.claude/teams/{team_name}/inboxes/{agent_name}.json

消息类型（7种）:
  普通消息           → { from, text, timestamp, read, color, summary }
  关闭请求/响应      → { type: "shutdown_request/response", request_id }
  计划审批请求/响应  → { type: "plan_approval_request/response", ... }
  权限请求/响应      → { type: "permission_request/response", ... }
  空闲通知           → { type: "idle_notification", agentName, reason }

关键特性：
  - 文件锁（lockfile）防并发写冲突
  - 10次重试 + 5-100ms指数退避
  - 零外部依赖，天然持久化，跨进程/跨机器兼容
```

**当前 Go 实现**：
```go
// internal/coordinator/coordinator.go
// 使用 channel + map 实现内存消息队列
// 仅支持进程内通信，不支持跨进程/跨机器
// SendMessage 仅支持 agent_id 路由
```

**差距分析**：
- ❌ 没有基于文件系统的持久化邮箱
- ❌ 没有跨进程通信能力
- ❌ 没有 7 种协议消息类型
- ❌ 没有 `to: "researcher"` 名称路由（当前只支持 agent_id UUID 路由）
- ❌ 没有广播支持（`to: "*"`）
- ❌ 没有 UDS socket 支持（`to: "uds:/path.sock"`）

**建议优先级**：🟢 P3（Swarm 前置依赖）

**修复难度**：高（~500 行代码）

---

### 2.8 ❌ CacheSafeParams / Prompt Cache 共享

**原版实现**：
```typescript
// src/utils/forkedAgent.ts
type CacheSafeParams = {
  systemPrompt: SystemPrompt          // 必须与父Agent一致（保证命中缓存）
  userContext: { [k: string]: string } // 前缀消息（影响缓存命中）
  systemContext: { [k: string]: string }
  toolUseContext: ToolUseContext
  forkContextMessages: Message[]       // 父上下文消息，作为子Agent消息前缀
}

// stopHooks.ts — 每个 Turn 结束后保存
saveCacheSafeParams(createCacheSafeParams(context))

// Fork 子Agent 时复用（命中父Agent的 Prompt Cache）
const cachedParams = loadCacheSafeParams()
await runForkedAgent({ cacheSafeParams: cachedParams, ... })
```

**当前 Go 实现**：
- ❌ 没有 `CacheSafeParams` 类型
- ❌ 没有 `saveCacheSafeParams` / `loadCacheSafeParams`
- ❌ 子Agent每次都以空上下文启动，无法共享父Agent的 Prompt Cache
- ❌ 没有 Turn 结束时的 StopHooks 机制

**影响**：每个子Agent调用都需要重新处理完整系统提示词，增加 token 消耗和延迟。

**建议优先级**：🟡 P2（性能优化，节约 token 成本）

**修复难度**：高（依赖引擎层深度集成）

---

### 2.9 ❌ Agent 上下文隔离（AsyncLocalStorage 等价物）

**原版实现**：
```typescript
// src/utils/agentContext.ts
const agentContextStorage = new AsyncLocalStorage<AgentContext>()

// 每个Agent在自己的异步上下文中运行，互不干扰
function runWithAgentContext<T>(context: AgentContext, fn: () => T): T {
  return agentContextStorage.run(context, fn)
}

// 任何位置都能获取当前Agent的上下文
function getAgentContext(): AgentContext | undefined {
  return agentContextStorage.getStore()
}
```

**当前 Go 实现**：
```go
// Go 没有 AsyncLocalStorage 的直接等价物
// goroutine-local storage 需要通过 context.Context 传递
// internal/coordinator/coordinator.go: 通过 sync.RWMutex 保护共享 agents map
// 但无法在调用链任意位置获取"当前Agent ID"
```

**差距分析**：
- ❌ 没有 goroutine-local 的 Agent 上下文机制
- ❌ 多个并发 Agent 无法自动区分各自的上下文
- ❌ 底层工具（如日志、记忆、权限）无法感知"当前是哪个Agent在调用"

**建议优先级**：🟡 P2（并发Agent隔离的基础）

**修复难度**：中（Go 中通过 context.Context 传播 AgentID 可部分解决）

---

### 2.10 ❌ 模型选择策略（Per-Agent Model）

**原版实现**：
```typescript
// src/utils/model/agent.ts
export function getAgentModel(agentModel, parentModel, toolSpecifiedModel, permissionMode) {
  // 优先级：
  // 1. 环境变量 CLAUDE_CODE_SUBAGENT_MODEL
  // 2. 工具层指定模型 (toolSpecifiedModel)
  // 3. Agent定义中的模型 (agentModel / 'inherit')
  // 4. 'inherit' → 继承父Agent精确模型字符串（防意外降级）
  // 5. 别名匹配父层级 → 继承父Model版本
  // 6. parseUserSpecifiedModel()
}
// 关键：Explore外部用户→Haiku，Plan→inherit（享受Opus时Opus执行Plan）
```

**当前 Go 实现**：
- ❌ 子Agent始终使用主Agent的相同模型，无法按Agent类型差异化配置
- ❌ 没有 `CLAUDE_CODE_SUBAGENT_MODEL` 环境变量支持
- ❌ 没有 `inherit` / 模型别名解析逻辑

**建议优先级**：🟡 P2（与内置Agent类型配套）

**修复难度**：中（~150 行代码）

---

### 2.11 ❌ Agent 颜色管理（Agent Color Manager）

**原版实现**：
```typescript
// src/tools/AgentTool/agentColorManager.ts
// 为每个 Agent 分配不同颜色，用于：
// - TUI 中的 Spinner 颜色标识
// - 邮箱消息的发送者颜色
// - TeamsDialog 中的成员颜色
// 循环使用调色板，确保同时运行的Agent视觉可区分
```

**当前 Go 实现**：
- ❌ 没有 Agent 颜色分配
- ❌ TUI 中无法区分不同 Agent 的输出
- ❌ 没有 `agentColor` 参数在 Coordinator 中

**建议优先级**：🟢 P3（UI 体验）

**修复难度**：低（~80 行代码，但需要 TUI 集成）

---

### 2.12 ❌ Agent 摘要生成（Agent Summary）

**原版实现**：
```typescript
// src/services/AgentSummary/agentSummary.ts
// Agent 完成后自动生成自然语言摘要
// 用途：
// 1. 向父Agent/Leader报告完成情况（"完成了哪些工作"）
// 2. 在BackgroundTasksDialog中展示
// 3. 在SDK事件流中传递进度信息
// 4. 空闲通知中的summary字段
```

**当前 Go 实现**：
- ❌ 没有 `agentSummary` 等效模块
- ❌ Agent 完成后只返回原始结果文本，没有结构化摘要
- ❌ `WaitForAgent` 返回的是原始 result，不含摘要

**建议优先级**：🟡 P2（提升 Coordinator 模式用户体验）

**修复难度**：中（~200 行代码 + Forked Agent 前置依赖）

---

### 2.13 ❌ Agent 记忆（Per-Agent Memory）

**原版实现**：
```typescript
// src/tools/AgentTool/agentMemory.ts
type AgentMemoryScope = 'user' | 'project' | 'local'

// 每种Agent类型有独立记忆目录
// 'user'    → ~/.claude/agent-memory/{agentType}/
// 'project' → {cwd}/.claude/agent-memory/{agentType}/
// 'local'   → {cwd}/.claude/agent-memory-local/{agentType}/

// agentMemorySnapshot.ts — 跨会话记忆快照保存/恢复
checkAgentMemorySnapshot()   // 检查可恢复快照
initializeFromSnapshot()      // 从快照恢复Agent记忆
```

**当前 Go 实现**：
- ❌ 没有按 Agent 类型区分的记忆目录
- ❌ 所有 Agent 共用同一记忆系统（无类型隔离）
- ❌ 没有 `agentMemorySnapshot` 等效模块

**建议优先级**：🟢 P3（高级功能，依赖内置Agent类型）

**修复难度**：中（~300 行代码，依赖 P1 内置Agent类型）

---

### 2.14 ❌ SendMessage 高级路由

**原版实现**：
```typescript
// src/tools/SendMessageTool/prompt.ts
// 支持4种路由目标
{ to: "researcher" }          // 按名称发送给队友
{ to: "*" }                   // 广播给所有队友
{ to: "uds:/path/to.sock" }   // 本地UDS socket（跨会话）
{ to: "bridge:session_xxx" }  // 远程Bridge（跨机器）

// 消息包含 summary 预览字段（5-10字）供UI展示
```

**当前 Go 实现**：
```go
// internal/tools/agent/sendmessage.go
type SendMessageInput struct {
    AgentID string `json:"agent_id"`   // ❌ 只支持UUID路由
    Content string `json:"content"`    // ❌ 无 summary 字段
    // ❌ 缺少: To string `json:"to"` 支持名称/广播/UDS/bridge
}
```

**差距分析**：
- ❌ 只支持 `agent_id` UUID 路由，原版支持名称（"researcher"）路由
- ❌ 不支持广播（`to: "*"`）
- ❌ 不支持 UDS socket 跨会话通信
- ❌ 不支持 Bridge 跨机器通信
- ❌ 没有 `summary` 字段

**建议优先级**：🟡 P2（基础通信能力补全）

**修复难度**：中（~200 行代码，名称路由需要 Coordinator 支持）

---

### 2.15 ❌ Agent 恢复机制（Resume Agent）

**原版实现**：
```typescript
// src/tools/AgentTool/resumeAgent.ts
// 允许从历史对话记录中恢复并续接一个Agent
// 对话历史保存于: ~/.claude/sessions/{sessionId}/agents/{agentId}/transcript.json
// 用途：长时间任务中断后继续，避免从头开始
```

**当前 Go 实现**：
- ❌ 没有 Agent 对话历史持久化
- ❌ 没有 `resumeAgent` 功能
- ❌ Agent 中断后只能重新创建

**建议优先级**：🟢 P3（高级功能）

**修复难度**：高（依赖会话持久化基础设施）

---

### 2.16 ❌ 工具过滤（Agent Tool Filtering）

**原版实现**：
```typescript
// src/tools/AgentTool/agentToolUtils.ts
// 不同Agent类型有严格的工具白名单/黑名单：

// 所有Agent禁止的工具（防止Agent间递归创建）：
const ALL_AGENT_DISALLOWED_TOOLS = [
  'TeamCreate', 'TeamDelete', 'SendMessage',
  'TaskCreate', 'TaskGet', 'TaskList', 'TaskUpdate', 'TaskStop',
  'TaskOutput', 'EnterPlanMode', 'ExitPlanMode',
]

// 异步Agent工具白名单：
const ASYNC_AGENT_ALLOWED_TOOLS = [
  'Bash', 'FileRead', 'FileEdit', 'FileWrite',
  'Glob', 'Grep', 'Agent', 'WebFetch', 'WebSearch', ...
]

// 进程内队友的额外工具：
const IN_PROCESS_TEAMMATE_ALLOWED_TOOLS = [
  ...ASYNC_AGENT_ALLOWED_TOOLS,
  'SendMessage', 'TaskCreate', 'TaskGet', ...
]
```

**当前 Go 实现**：
- ❌ 没有 Agent 级别的工具过滤
- ❌ 子Agent可以调用主Agent的全部工具（包括 TeamCreate、EnterPlanMode 等）
- ❌ 没有防止递归创建 Agent 的保护机制

**建议优先级**：🔴 P1（安全性，防止无限递归）

**修复难度**：中（~200 行代码）

---

### 2.17 ❌ Task 类型体系（TaskType）

**原版实现**：
```typescript
// src/Task.ts — 7种任务类型
export type TaskType =
  | 'local_bash'           // 本地Shell命令
  | 'local_agent'          // 本地后台Agent
  | 'remote_agent'         // 远程Agent（CCR环境）
  | 'in_process_teammate'  // 进程内队友（Swarm）
  | 'local_workflow'       // 本地工作流
  | 'monitor_mcp'          // MCP监控
  | 'dream'                // 后台"梦境"任务

// TaskStatus: pending/running/completed/failed/killed（注意：killed vs stopped）
```

**当前 Go 实现**：
```go
// internal/tools/tasks/tasks.go
// 只有 TaskStatus，没有 TaskType
const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusStopped   TaskStatus = "stopped"   // 对应原版 "killed"
)
// ❌ 没有 TaskType 区分
// ❌ 没有 remote_agent、in_process_teammate 等类型
```

**建议优先级**：🟡 P2（扩展任务系统所需）

**修复难度**：低（~50 行代码，类型补充）

---

### 2.18 ❌ UI 呈现组件（Agent-specific UI）

**原版实现**：完整的多Agent UI组件体系：

```
AgentProgressLine         — 对话流中的Agent进度内联展示
BackgroundTasksDialog     — 后台任务管理对话框（113KB，最大UI组件之一）
CoordinatorTaskPanel      — Coordinator模式下Worker状态面板
TeamsDialog               — 团队成员状态和消息历史
TeamStatus                — 单个团队状态展示
TeammateViewHeader        — "进入"队友视角查看其对话历史
TeammateSpinnerLine       — 队友工作状态树形Spinner
AgentsList / AgentsMenu   — Agent列表和管理菜单
```

**当前 Go 实现**：
- ⚠️ 有基础的 Spinner（等待 AI 响应时）
- ❌ 没有 Agent 进度内联展示
- ❌ 没有后台任务管理对话框
- ❌ 没有 Coordinator 状态面板
- ❌ 没有团队视图相关组件
- ❌ 没有颜色区分不同 Agent 的机制

**建议优先级**：🟡 P2（Coordinator 模式基本可用性）

**修复难度**：高（~2000 行 TUI 代码）

---

## 三、架构差异总结

| 维度 | 原版 TypeScript | 当前 Go | 完成度 |
|------|----------------|---------|--------|
| **Tool-as-Agent 范式** | AgentTool + LLM自主决策 | AgentTool（通用） | ✅ 80% |
| **Coordinator 基础设施** | coordinatorMode + AppState | coordinator.go + RWMutex | ✅ 75% |
| **Task 工具集** | TaskCreate/Get/List/Update/Stop/Output | 全部实现 | ✅ 90% |
| **内置 Agent 类型（5种）** | Explore/Plan/Verification/General/Guide | 无类型区分 | ❌ 0% |
| **自定义 Agent 定义** | .claude/agents/（7层优先级） | 无 | ❌ 0% |
| **异步后台 Agent（Layer 2）** | background: true + 独立生命周期 | 无（Call始终阻塞） | ❌ 10% |
| **Swarm/团队模式（Layer 3）** | TeamCreate + 多后端 + 邮箱 | 无 | ❌ 0% |
| **执行后端（tmux/iTerm/in-proc）** | 3种后端 + 自动选择 | 仅goroutine（无抽象） | ❌ 5% |
| **文件邮箱通信** | 7种协议消息 + 文件锁 | 内存channel（仅进程内） | ⚠️ 20% |
| **SendMessage 路由** | 名称/广播/UDS/Bridge | 仅UUID | ⚠️ 30% |
| **权限同步机制** | Worker→Leader邮箱路由 | 无 | ❌ 0% |
| **CacheSafeParams（Prompt Cache共享）** | 完整实现 + StopHooks保存 | 无 | ❌ 0% |
| **Agent上下文隔离** | AsyncLocalStorage | sync.RWMutex（部分） | ⚠️ 30% |
| **模型选择策略** | 多级策略 + inherit + 防降级 | 无（继承主Agent模型） | ❌ 5% |
| **Agent工具过滤** | 白名单+黑名单+类型差异化 | 无过滤 | ❌ 0% |
| **Agent颜色管理** | 调色板 + UI集成 | 无 | ❌ 0% |
| **Agent摘要生成** | 完成后自动生成 | 无 | ❌ 0% |
| **Agent记忆（按类型）** | 三级scope + 快照 | 无 | ❌ 0% |
| **Agent恢复机制** | transcript持久化 + resume | 无 | ❌ 0% |
| **TeamCreate/TeamDelete** | 完整工具 | 无 | ❌ 0% |
| **TaskType体系（7种）** | 完整 | 仅Status（无Type） | ⚠️ 40% |
| **UI组件（Agent/Team）** | 8+个专用组件 | 无专用组件 | ❌ 0% |

**整体评估**：约 **20-25%** 功能覆盖率。Layer 1（同步子Agent）基本可用，Layer 2（异步后台）和 Layer 3（Swarm团队）几乎全部缺失。

---

## 四、优先改进计划

### 🔴 P0 — 关键前置依赖

| # | 功能 | 工作量估算 | 影响范围 | 说明 |
|---|------|-----------|----------|------|
| 1 | **子Agent工具过滤** | 1 天 | 安全性 | 防止无限递归和越权调用；AgentTool Call 中注入工具黑名单 |
| 2 | **异步后台Agent模式（`background: true`）** | 3 天 | Layer 2 | 让 AgentTool 支持fire-and-forget；TaskCreate 关联；是Coordinator模式核心 |

**P0 总工作量**：~4 天

### 🔴 P1 — 核心功能

| # | 功能 | 工作量估算 | 前置依赖 |
|---|------|-----------|----------|
| 3 | **内置Agent类型（5种）** | 5 天 | P0-1 |
| 4 | **SendMessage 名称路由** | 2 天 | 无 |
| 5 | **AgentID 上下文传播（context.Context）** | 2 天 | 无 |
| 6 | **Coordinator UI面板（基础版）** | 3 天 | P0-2 |

**P1 总工作量**：~12 天

### 🟡 P2 — 体验完善

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 7 | **自定义Agent定义（.claude/agents/）** | 3 天 |
| 8 | **CacheSafeParams / StopHooks** | 3 天 |
| 9 | **模型选择策略（per-agent）** | 1.5 天 |
| 10 | **Agent摘要生成** | 2 天 |
| 11 | **TaskType 体系扩展** | 0.5 天 |
| 12 | **SendMessage 广播支持** | 1 天 |
| 13 | **Agent颜色管理 + TUI集成** | 1.5 天 |

**P2 总工作量**：~12.5 天

### 🟢 P3 — 高级功能

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 14 | **文件邮箱通信系统** | 5 天 |
| 15 | **Swarm/团队模式（Team工具集）** | 8 天 |
| 16 | **执行后端（tmux/iTerm2）** | 6 天 |
| 17 | **权限同步机制** | 4 天 |
| 18 | **Agent记忆（按类型）** | 3 天 |
| 19 | **Agent恢复机制** | 3 天 |
| 20 | **BackgroundTasksDialog / TeamsDialog** | 5 天 |

**P3 总工作量**：~34 天

---

## 五、实现建议

### 5.1 子 Agent 工具过滤（P0，最优先）

```go
// 建议修改: internal/tools/agent/agent.go

// agentDisallowedTools 列出所有子Agent不允许调用的工具
// 防止无限递归和越权操作
var agentDisallowedTools = map[string]bool{
    "TeamCreate":    true,
    "TeamDelete":    true,
    "EnterPlanMode": true,
    "ExitPlanMode":  true,
    // TaskXxx 工具视 agent_type 而定，异步Agent可以用
}

func (t *agentTool) Call(input tools.Input, ctx *tools.UseContext, onProgress tools.OnProgressFn) (*tools.Result, error) {
    // ...现有逻辑...

    // 计算最终的 AllowedTools（原始白名单 - 黑名单）
    effectiveTools := filterDisallowedTools(in.AllowedTools, agentDisallowedTools)

    agentID, err := ctx.Coordinator.SpawnAgent(ctx.Ctx, tools.AgentSpawnRequest{
        // ...
        AllowedTools: effectiveTools,
    })
    // ...
}

func filterDisallowedTools(allowed []string, disallowed map[string]bool) []string {
    if len(allowed) == 0 {
        // 默认：全部工具但排除黑名单
        return nil // coordinator层处理
    }
    result := make([]string, 0, len(allowed))
    for _, t := range allowed {
        if !disallowed[t] {
            result = append(result, t)
        }
    }
    return result
}
```

### 5.2 异步后台 Agent 模式（P0）

```go
// 建议修改: internal/tools/agent/agent.go

type AgentInput struct {
    Prompt       string   `json:"prompt"`
    SystemPrompt string   `json:"system_prompt,omitempty"`
    AllowedTools []string `json:"allowed_tools,omitempty"`
    MaxTurns     *int     `json:"max_turns,omitempty"`
    Background   bool     `json:"background,omitempty"`  // ← 新增
    AgentName    string   `json:"agent_name,omitempty"`  // ← 新增（用于路由）
    AgentType    string   `json:"agent_type,omitempty"`  // ← 新增（内置类型）
}

func (t *agentTool) Call(input tools.Input, ctx *tools.UseContext, onProgress tools.OnProgressFn) (*tools.Result, error) {
    // ...解析输入、spawn agent...

    if in.Background {
        // 异步模式：spawn 后立即返回，不等待
        if onProgress != nil {
            onProgress(map[string]string{
                "agent_id": agentID,
                "status":   "started",
                "message":  "Agent started in background",
            })
        }
        out := AgentOutput{
            Response: fmt.Sprintf("Agent %s started in background. Use TaskGet(id=%q) to check status.", agentID, agentID),
            AgentID:  agentID,
        }
        outBytes, _ := json.Marshal(out)
        return &tools.Result{Content: string(outBytes)}, nil
    }

    // 同步模式（现有逻辑）：等待完成
    result, err := ctx.Coordinator.WaitForAgent(ctx.Ctx, agentID)
    // ...
}
```

### 5.3 内置 Agent 类型系统（P1）

```go
// 建议新建: internal/tools/agent/builtin_agents.go

// BuiltinAgent 定义内置Agent的配置
type BuiltinAgent struct {
    Type         string
    Description  string
    WhenToUse    string
    SystemPrompt string
    AllowedTools []string   // nil 表示继承全部
    DisallowedTools []string
    Model        string     // "" = inherit, "haiku", "sonnet", "opus"
    MaxTurns     int
}

// 内置Agent注册表
var builtinAgents = map[string]*BuiltinAgent{
    "explore": {
        Type:        "explore",
        Description: "Fast agent specialized for exploring codebases.",
        WhenToUse:   "Use when you need to quickly find files by patterns or search code for keywords.",
        SystemPrompt: exploreAgentSystemPrompt, // 严格只读指令
        AllowedTools: []string{"Glob", "Grep", "Read", "Bash"},
        Model:        "haiku",
        MaxTurns:     20,
    },
    "plan": {
        Type:        "plan",
        Description: "Software architect agent for designing implementation plans.",
        SystemPrompt: planAgentSystemPrompt,
        AllowedTools: []string{"Glob", "Grep", "Read", "Bash"},
        Model:        "inherit",
    },
    "verify": {
        Type:        "verify",
        Description: "Verification agent that attempts to break implementations.",
        SystemPrompt: verifyAgentSystemPrompt,
        DisallowedTools: []string{"Write", "Edit"},
        Model:        "inherit",
    },
    "general-purpose": {
        Type:         "general-purpose",
        Description:  "General-purpose agent for complex multi-step tasks.",
        SystemPrompt: generalPurposeSystemPrompt,
        AllowedTools: nil, // all tools
        Model:        "inherit",
    },
    "claude-code-guide": {
        Type:        "claude-code-guide",
        Description: "Expert on Claude Code CLI, Agent SDK, and Claude API.",
        SystemPrompt: claudeCodeGuideSystemPrompt,
        AllowedTools: []string{"Glob", "Grep", "Read", "WebFetch", "WebSearch"},
        Model:        "inherit",
    },
}

// GetBuiltinAgent 根据 agent_type 获取内置Agent配置
func GetBuiltinAgent(agentType string) (*BuiltinAgent, bool) {
    a, ok := builtinAgents[agentType]
    return a, ok
}
```

### 5.4 AgentID 上下文传播（P1）

```go
// 建议新建: internal/tools/agent/context.go

// agentIDKey 是 context.Context 中存储 AgentID 的 key
type agentIDKeyType struct{}

var agentIDKey = agentIDKeyType{}

// WithAgentID 将 AgentID 注入 context.Context
func WithAgentID(ctx context.Context, agentID string) context.Context {
    return context.WithValue(ctx, agentIDKey, agentID)
}

// GetAgentID 从 context.Context 中获取 AgentID
func GetAgentID(ctx context.Context) (string, bool) {
    id, ok := ctx.Value(agentIDKey).(string)
    return id, ok
}

// 在 coordinator.runAgentLoop 中注入：
func (c *Coordinator) runAgentLoop(ctx context.Context, agentID string, req AgentSpawnRequest) {
    ctx = agent.WithAgentID(ctx, agentID)  // ← 注入 AgentID
    // 后续所有工具调用都能通过 ctx 获取当前Agent ID
    // ...
}
```

### 5.5 SendMessage 名称路由（P1）

```go
// 建议修改: internal/tools/agent/sendmessage.go

type SendMessageInput struct {
    To      string `json:"to"`              // 支持: agent_id UUID, name, "*"
    Content string `json:"content"`
    Summary string `json:"summary,omitempty"` // 5-10字预览
}

// 建议修改: internal/coordinator/coordinator.go — 增加名称索引
type Coordinator struct {
    agents    map[string]*agentEntry    // by agent_id
    nameIndex map[string]string         // name → agent_id
    // ...
}

func (c *Coordinator) SendMessageTo(ctx context.Context, to string, message string) error {
    c.mu.RLock()
    defer c.mu.RUnlock()

    if to == "*" {
        // 广播
        for id := range c.agents {
            _ = c.sendToAgent(ctx, id, message)
        }
        return nil
    }

    // 先查 name index，再查 agent_id
    if id, ok := c.nameIndex[to]; ok {
        return c.sendToAgent(ctx, id, message)
    }
    return c.sendToAgent(ctx, to, message)
}
```

---

## 六、代码质量问题

### 6.1 AgentTool.Call 缺少输入验证

```go
// internal/tools/agent/agent.go
func (t *agentTool) Call(input tools.Input, ctx *tools.UseContext, ...) (*tools.Result, error) {
    // ❌ 没有对 in.MaxTurns 的范围校验（可以传入负数或超大值）
    // ❌ 没有对 in.AllowedTools 的有效性校验
    // ❌ 没有对 in.SystemPrompt 长度的限制
    maxTurns := 0
    if in.MaxTurns != nil {
        maxTurns = *in.MaxTurns  // ← 直接使用，无范围校验
    }
}
```

**建议**：增加 `validateAgentInput(in AgentInput) error` 函数，校验各字段合法性。

### 6.2 TaskCreateTool 的 "do not create again" 提示低效

```go
// internal/tools/tasks/tasks.go:125-131
result := fmt.Sprintf(
    "%s\n\nTask %s is now running in the background. "+
    "Do NOT create another task with the same description. "+
    "Use TaskGet with id=%q to check its status ...",
    string(out), agentID, agentID,
)
```

**问题**：通过自然语言提示控制 LLM 行为，容易被忽略；应通过工具设计（幂等性检查）而非提示词来防止重复创建。

**建议**：在 `Coordinator.SpawnAgent` 中基于描述哈希做幂等检查，重复调用时返回已有 agentID。

### 6.3 Coordinator.deduplication 基于 Description 字段不可靠

```go
// internal/coordinator/coordinator.go (推测实现)
// 通过 Description + Running 状态去重
// ❌ Description 是用户输入文本，完全相同的描述很罕见
// ❌ 不同任务可能有相同描述
```

**建议**：使用 `task_id`（由调用方生成并传入）作为幂等键，而非 Description。

### 6.4 WaitForAgent 缺少超时控制

```go
// internal/tools/agent/agent.go
result, err := ctx.Coordinator.WaitForAgent(ctx.Ctx, agentID)
// ❌ ctx.Ctx 的 deadline 可能被主循环的上下文控制
// ❌ 没有针对子Agent的独立超时配置
// ❌ 子Agent永远不完成时，主循环会永久阻塞
```

**建议**：在 `AgentInput` 中增加 `TimeoutSeconds *int` 字段，`WaitForAgent` 使用 `context.WithTimeout` 封装。

### 6.5 SendMessageTool 缺少 "agent 不存在" 的友好错误

```go
// internal/tools/agent/sendmessage.go
// 当 agent_id 对应的Agent不存在时，错误信息不够友好
// 原版会给出"可用Agent列表"帮助 LLM 纠错
```

**建议**：Send 失败时，在错误消息中附加当前可用的 Agent 列表（调用 `Coordinator.ListAgents()`）。

---

## 依赖关系图

```
P0（前置）
  ├── 子Agent工具过滤
  │     └── 无前置依赖，独立实现
  └── 异步后台Agent模式
        └── 无前置依赖，在现有 Coordinator 基础上扩展

P1（核心）
  ├── 内置Agent类型（5种）
  │     ├── 依赖: P0 异步后台Agent（用于 background 类型的Agent）
  │     └── 依赖: P0 工具过滤（每种Agent有独立工具集）
  ├── SendMessage名称路由
  │     └── 无前置依赖
  ├── AgentID上下文传播
  │     └── 无前置依赖
  └── Coordinator UI面板（基础版）
        └── 依赖: P0 异步后台Agent

P2（体验）
  ├── 自定义Agent定义
  │     └── 依赖: P1 内置Agent类型系统（共享类型体系）
  ├── CacheSafeParams
  │     └── 依赖: 引擎层 + StopHooks（独立基础设施）
  ├── Agent摘要生成
  │     └── 依赖: P0 异步后台Agent（异步生成）
  ├── 模型选择策略
  │     └── 依赖: P1 内置Agent类型
  └── SendMessage广播
        └── 依赖: P1 SendMessage名称路由

P3（高级）
  ├── 文件邮箱系统
  │     └── 无前置依赖（独立基础设施）
  ├── Swarm/团队模式
  │     ├── 依赖: P3 文件邮箱系统
  │     ├── 依赖: P1 SendMessage路由
  │     └── 依赖: P0 所有前置
  ├── 执行后端（tmux/iTerm2）
  │     └── 依赖: P3 Swarm架构
  ├── 权限同步
  │     ├── 依赖: P3 文件邮箱系统
  │     └── 依赖: P3 Swarm Leader/Worker角色
  ├── Agent记忆（按类型）
  │     └── 依赖: P1 内置Agent类型
  └── BackgroundTasksDialog / TeamsDialog
        ├── 依赖: P0 异步后台Agent
        └── 依赖: P3 Swarm模式
```

---

## 相关资料

- [多Agent系统原版设计文档](./origin/multi-agent-system-design.md) — TypeScript 原版深度分析（1200行）
- [架构设计文档](../project/architecture.md) — Go版本整体架构
- `internal/coordinator/coordinator.go` — Coordinator 核心实现
- `internal/tools/agent/agent.go` — AgentTool 实现
- `internal/tools/tasks/tasks.go` — Task工具集实现
- `internal/engine/orchestration.go` — 工具调用批处理（IsConcurrencySafe）
