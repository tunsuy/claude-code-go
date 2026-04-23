# Claude Code 多Agent系统设计深度分析

> 基于 claude-code-main 源码的全面逆向工程分析
> 分析日期：2026-04-23

---

## 目录

1. [宏观设计理念](#1-宏观设计理念)
2. [整体架构总览](#2-整体架构总览)
3. [核心概念模型](#3-核心概念模型)
4. [Agent 类型体系](#4-agent-类型体系)
5. [Agent 生命周期管理](#5-agent-生命周期管理)
6. [数据交互与通信机制](#6-数据交互与通信机制)
7. [执行后端架构](#7-执行后端架构)
8. [权限同步与安全模型](#8-权限同步与安全模型)
9. [Agent 记忆与状态持久化](#9-agent-记忆与状态持久化)
10. [工作流集成场景](#10-工作流集成场景)
11. [UI 呈现架构](#11-ui-呈现架构)
12. [关键技术点总结](#12-关键技术点总结)
13. [核心文件索引](#13-核心文件索引)

---

## 1. 宏观设计理念

### 1.1 "Tool-as-Agent" 范式

Claude Code 的多Agent系统最核心的设计理念是 **"Tool-as-Agent"**——Agent 本身是一个 Tool。主 Agent（LLM）通过调用名为 `Task`（代码中为 `AgentTool`）的工具来"生成"子 Agent。这意味着：

- **Agent 的创建是 LLM 自主决策的结果**，不是预设的静态编排
- LLM 根据任务复杂度和类型，自主判断何时需要 spawn 子 Agent
- 每个子 Agent 拥有独立的对话上下文、系统提示词和工具集
- 这种设计使得 Agent 协作是**涌现的（emergent）**而非硬编码的

### 1.2 分层递进的多Agent模式

系统设计了三层递进的多Agent协作模式：

```
┌─────────────────────────────────────────────────────────┐
│  Layer 3: Swarm/Team Mode (团队模式)                     │
│  ├── 完全独立的 Agent 进程/上下文                          │
│  ├── 基于文件的邮箱通信                                    │
│  ├── 团队级共享记忆                                       │
│  └── 支持 tmux/iTerm2/进程内 三种执行后端                   │
├─────────────────────────────────────────────────────────┤
│  Layer 2: Background Agent (后台Agent)                   │
│  ├── 异步执行，不阻塞主对话                                │
│  ├── 通过 Task 系统管理生命周期                             │
│  └── 进度追踪和结果聚合                                    │
├─────────────────────────────────────────────────────────┤
│  Layer 1: Subagent (同步子Agent)                         │
│  ├── 阻塞式执行（fork 主上下文）                            │
│  ├── 共享父Agent的 prompt cache                           │
│  └── 适用于快速委托任务                                    │
└─────────────────────────────────────────────────────────┘
```

### 1.3 "Prompt Cache 共享"优先

一个精妙的设计考量是 **prompt cache 命中率优化**。子 Agent 的 `CacheSafeParams` 设计确保了 fork 出的子 Agent 与父 Agent 共享 Anthropic API 的 prompt cache：

```typescript
// src/utils/forkedAgent.ts
export type CacheSafeParams = {
  systemPrompt: SystemPrompt          // 必须与父Agent一致
  userContext: { [k: string]: string } // 前缀消息影响缓存
  systemContext: { [k: string]: string } // 追加到system prompt
  toolUseContext: ToolUseContext        // 工具集、模型等
  forkContextMessages: Message[]       // 父上下文消息用于缓存共享
}
```

这意味着子 Agent 并非从零开始——它继承了父 Agent 的上下文前缀，避免了昂贵的重复 prompt 处理。

### 1.4 "隔离但可协作"原则

每个 Agent 运行在隔离的上下文中，但通过明确的通信通道协作：

- **上下文隔离**：通过 `AsyncLocalStorage` 实现进程内的 Agent 间上下文隔离
- **通信显式**：Agent 间通信必须通过 `SendMessage` 工具，不存在隐式的状态共享
- **权限隔离**：每个 Agent 有独立的权限模式，但可以通过 Permission Sync 机制同步

---

## 2. 整体架构总览

### 2.1 模块依赖关系

```
                           ┌──────────────┐
                           │   main.tsx   │  CLI 入口
                           └──────┬───────┘
                                  │
                    ┌─────────────┼─────────────┐
                    │             │             │
              ┌─────▼─────┐ ┌────▼────┐ ┌─────▼──────┐
              │ tools.ts  │ │query.ts │ │commands.ts │
              │ (工具注册) │ │(查询引擎)│ │ (命令系统) │
              └─────┬─────┘ └────┬────┘ └────────────┘
                    │            │
        ┌───────────┼────────────┤
        │           │            │
  ┌─────▼─────┐ ┌──▼──────┐ ┌──▼─────────────┐
  │ AgentTool │ │ Other   │ │ TeamCreateTool │
  │ (核心)    │ │ Tools   │ │ TeamDeleteTool │
  └─────┬─────┘ └─────────┘ │ SendMessageTool│
        │                    │ TaskXxxTool    │
        │                    └──────┬─────────┘
        │                           │
  ┌─────▼──────────────────────────▼──────────────┐
  │              utils/swarm/                       │
  │  ├── inProcessRunner.ts   (进程内Agent运行器)    │
  │  ├── spawnInProcess.ts    (进程内Agent生成)      │
  │  ├── permissionSync.ts    (权限同步)             │
  │  ├── teamHelpers.ts       (团队文件管理)          │
  │  └── backends/            (执行后端)             │
  │      ├── TmuxBackend.ts                         │
  │      ├── ITermBackend.ts                        │
  │      ├── InProcessBackend.ts                    │
  │      └── registry.ts      (后端注册与检测)        │
  └───────────────────────────────────────────────-─┘
        │
  ┌─────▼──────────────────────────────────┐
  │        tasks/ (任务运行时)               │
  │  ├── LocalAgentTask/     (本地Agent任务)  │
  │  ├── RemoteAgentTask/    (远程Agent任务)  │
  │  ├── InProcessTeammateTask/ (进程内队友)  │
  │  ├── LocalShellTask/     (Shell任务)     │
  │  └── DreamTask/          (后台梦境任务)   │
  └────────────────────────────────────────┘
```

### 2.2 核心文件职责

| 文件 | 职责 | 大小 |
|------|------|------|
| `tools/AgentTool/AgentTool.tsx` | Agent工具主逻辑，决定同步/异步执行 | 228KB |
| `tools/AgentTool/runAgent.ts` | Agent查询循环（单次对话turn） | 35KB |
| `tools/AgentTool/forkSubagent.ts` | Fork子Agent机制 | 8.5KB |
| `tools/shared/spawnMultiAgent.ts` | tmux多Agent生成共享模块 | 35KB |
| `utils/swarm/inProcessRunner.ts` | 进程内队友运行器 | 52KB |
| `utils/swarm/spawnInProcess.ts` | 进程内队友生成 | 10KB |
| `utils/forkedAgent.ts` | Fork Agent执行器 | 24KB |
| `utils/teammateMailbox.ts` | 队友邮箱（消息系统） | 33KB |
| `utils/swarm/permissionSync.ts` | 跨Agent权限同步 | 26KB |
| `coordinator/coordinatorMode.ts` | 协调器模式 | 19KB |
| `Task.ts` | 任务类型定义 | 3KB |

---

## 3. 核心概念模型

### 3.1 任务类型体系

```typescript
// src/Task.ts
export type TaskType =
  | 'local_bash'           // 本地Shell命令
  | 'local_agent'          // 本地后台Agent
  | 'remote_agent'         // 远程Agent（CCR环境）
  | 'in_process_teammate'  // 进程内队友（Swarm）
  | 'local_workflow'       // 本地工作流
  | 'monitor_mcp'          // MCP监控
  | 'dream'                // 后台"梦境"任务

export type TaskStatus =
  | 'pending'    // 等待启动
  | 'running'    // 运行中
  | 'completed'  // 完成
  | 'failed'     // 失败
  | 'killed'     // 被终止
```

### 3.2 Agent 上下文隔离模型

```typescript
// src/utils/agentContext.ts

// 子Agent上下文
type SubagentContext = {
  agentId: string          // UUID
  parentSessionId?: string // 父会话ID
  agentType: 'subagent'
  subagentName?: string    // 如 "Explore", "Plan"
  isBuiltIn?: boolean
  invokingRequestId?: string
}

// 队友Agent上下文
type TeammateAgentContext = {
  agentId: string          // "name@team"
  agentName: string        // 显示名
  teamName: string
  agentColor?: string      // UI颜色
  planModeRequired: boolean
  parentSessionId: string
  isTeamLead: boolean
  agentType: 'teammate'
}
```

关键设计：使用 `AsyncLocalStorage` 而非全局状态。原因是当多个 Agent 被后台运行（Ctrl+B），它们会并发执行，`AsyncLocalStorage` 保证每个异步执行链的上下文不会互相干扰。

### 3.3 Agent 定义模式

```typescript
// src/tools/AgentTool/loadAgentsDir.ts
// Agent 可以通过 Markdown 或 JSON 文件定义

type AgentDefinition = {
  agentType: string        // Agent类型名，如 "Explore"
  description?: string     // 描述
  whenToUse?: string       // 使用时机提示
  prompt?: string          // 系统提示词
  tools?: string[]         // 可用工具列表（'*' 表示全部）
  disallowedTools?: string[]  // 禁用工具
  model?: string           // 模型选择（'inherit' | 'sonnet' | 'haiku' | 'opus'）
  permissionMode?: PermissionMode
  mcpServers?: AgentMcpServerSpec[]  // MCP服务器配置
  hooks?: HooksSettings    // 钩子配置
  maxTurns?: number        // 最大对话轮数
  skills?: string[]        // 技能
  memory?: 'user' | 'project' | 'local'  // 记忆范围
  background?: boolean     // 是否后台执行
  isolation?: 'worktree' | 'remote'  // 隔离级别
  source: SettingSource | 'built-in'  // 来源
}
```

---

## 4. Agent 类型体系

### 4.1 内置Agent（Built-in Agents）

系统内置了5种专门化Agent，每种都有精心设计的系统提示词和工具约束：

#### 4.1.1 Explore Agent（代码探索Agent）

```
文件：src/tools/AgentTool/built-in/exploreAgent.ts
职责：快速搜索和分析代码库
模型：外部用户 → Haiku（追求速度），内部用户 → inherit（继承父Agent）
特点：
  - 严格只读模式（READ-ONLY）
  - 禁止创建/修改/删除任何文件
  - 可用工具：Glob, Grep, FileRead, Bash(只读)
  - 省略 CLAUDE.md 以节省 token
  - 支持并行工具调用以加速搜索
```

#### 4.1.2 Plan Agent（方案规划Agent）

```
文件：src/tools/AgentTool/built-in/planAgent.ts
职责：探索代码库并设计实现方案
模型：inherit（继承父Agent，在plan模式下可获得Opus）
特点：
  - 严格只读模式
  - 输出结构化的实施计划
  - 包含"关键实施文件"清单
  - 复用 Explore Agent 的工具配置
```

#### 4.1.3 Verification Agent（验证Agent）

```
文件：src/tools/AgentTool/built-in/verificationAgent.ts
职责：尝试"打破"实现——而非确认它能工作
模型：inherit
特点：
  - 禁止修改项目目录中的文件
  - 可以在 /tmp 目录写测试脚本
  - 包含详细的验证策略（前端/后端/CLI/基建/数据库等）
  - 内置"反省机制"防止验证偷懒
  - 要求至少一个对抗性探测（并发/边界/幂等/孤儿操作）
```

#### 4.1.4 General Purpose Agent（通用Agent）

```
文件：src/tools/AgentTool/built-in/generalPurposeAgent.ts
职责：通用任务执行
模型：使用 getDefaultSubagentModel()（默认 inherit）
特点：
  - 拥有全部工具（tools: ['*']）
  - 适用于搜索代码、分析架构、执行多步任务
  - 禁止主动创建文档文件
```

#### 4.1.5 Claude Code Guide Agent（使用引导Agent）

```
文件：src/tools/AgentTool/built-in/claudeCodeGuideAgent.ts
职责：帮助用户理解和使用 Claude Code
特点：
  - 擅长三个领域：Claude Code CLI、Agent SDK、Claude API
  - 可以从官方文档站点抓取信息
  - WebFetch + WebSearch 能力
```

### 4.2 自定义Agent

用户可以在 `.claude/agents/` 目录下通过 Markdown 或 JSON 定义自定义 Agent：

```markdown
---
description: "My custom code reviewer"
model: sonnet
tools:
  - FileRead
  - Grep
  - Glob
permissionMode: default
---

You are a code review specialist...
```

Agent 加载优先级（从高到低）：
1. **User agents** (`~/.claude/agents/`)
2. **Project agents** (`.claude/agents/`)
3. **Local agents** (本地配置)
4. **Managed agents** (策略配置)
5. **Plugin agents** (插件提供)
6. **CLI arg agents** (命令行参数)
7. **Built-in agents** (内置)

### 4.3 模型选择策略

```typescript
// src/utils/model/agent.ts
export function getAgentModel(
  agentModel: string | undefined,
  parentModel: string,
  toolSpecifiedModel?: ModelAlias,
  permissionMode?: PermissionMode,
): string {
  // 1. 环境变量覆盖
  if (process.env.CLAUDE_CODE_SUBAGENT_MODEL) {
    return parseUserSpecifiedModel(process.env.CLAUDE_CODE_SUBAGENT_MODEL)
  }
  // 2. 工具指定的模型
  if (toolSpecifiedModel) { ... }
  // 3. Agent定义中的模型
  const agentModelWithExp = agentModel ?? getDefaultSubagentModel()
  // 4. 'inherit' → 继承父Agent模型
  if (agentModelWithExp === 'inherit') {
    return getRuntimeMainLoopModel({ ... })
  }
  // 5. 别名匹配父模型层级（防止意外降级）
  if (aliasMatchesParentTier(agentModelWithExp, parentModel)) {
    return parentModel
  }
  // 6. 解析为具体模型
  return parseUserSpecifiedModel(agentModelWithExp)
}
```

关键设计：当子 Agent 使用 `opus` 别名但父 Agent 已经在使用 Opus 4.6 时，子 Agent 会直接继承父 Agent 的确切模型字符串（而不是解析别名到默认版本），防止意外降级。

---

## 5. Agent 生命周期管理

### 5.1 同步子Agent生命周期

```
用户消息 → 主Agent LLM推理
    │
    ▼ LLM决定调用AgentTool
    │
AgentTool.execute()
    │
    ├── 解析Agent类型和参数
    ├── filterToolsForAgent() → 构建工具集
    ├── buildEffectiveSystemPrompt() → 构建系统提示
    │
    ▼ 同步模式（阻塞主循环）
    │
runAgent() → query() 循环
    │
    ├── 子Agent与LLM多轮对话
    ├── 每轮执行工具调用
    ├── recordSidechainTranscript() → 记录对话历史
    │
    ▼ 子Agent完成
    │
返回结果到主Agent
    │
    ▼ 主Agent继续推理
```

### 5.2 异步后台Agent生命周期

```
用户消息 → 主Agent LLM推理
    │
    ▼ LLM决定创建后台Agent
    │
registerAsyncAgent() → 注册到AppState.tasks
    │
    ├── generateTaskId('local_agent')
    ├── createAbortController() → 可取消
    ├── initTaskOutputAsSymlink() → 输出文件
    │
    ▼ 启动后台执行
    │
runAsyncAgentLifecycle()  ← 不阻塞主循环
    │
    ├── runWithAgentContext() → 设置AsyncLocalStorage
    ├── runAgent() → 独立的查询循环
    ├── updateAsyncAgentProgress() → 更新进度
    │
    ├── 成功 → completeAsyncAgent()
    │         ├── 更新状态为 'completed'
    │         ├── 发送通知
    │         └── 启动摘要生成
    │
    └── 失败 → failAsyncAgent()
              ├── 更新状态为 'failed'
              └── 记录错误
```

### 5.3 Swarm队友Agent生命周期

```
主Agent → TeamCreateTool
    │
    ▼ 创建团队
    │
writeTeamFile() → 写入 ~/.claude/teams/{team_name}/team.json
    │
    ├── leadAgentId: 主Agent ID
    ├── members: []
    └── sessionId: 当前会话ID
    │
    ▼ 主Agent调用AgentTool（带team_name参数）
    │
detectAndGetBackend() → 选择执行后端
    │
    ├─── tmux → spawnMultiAgent()
    │    └── 在tmux pane中启动新Claude进程
    │
    ├─── iTerm2 → ITermBackend.spawn()
    │    └── 在iTerm2分割面板中启动
    │
    └─── in-process → spawnInProcessTeammate()
         └── 同进程内通过AsyncLocalStorage隔离
    │
    ▼ 队友启动
    │
teammateInit() → initializeTeammateHooks()
    │
    ├── 读取团队文件
    ├── 注册Stop钩子（空闲时通知leader）
    ├── 应用团队级权限路径
    │
    ▼ 队友运行
    │
inProcessRunner.startInProcessTeammate() 或 独立进程
    │
    ├── 执行任务
    ├── 通过邮箱与leader/其他队友通信
    │
    ▼ 队友完成/空闲
    │
发送 idle notification 到 leader 邮箱
    │
    ▼ Leader 可以：
    │
    ├── 发新任务 → SendMessage → 队友恢复执行
    ├── 请求关闭 → shutdown_request → 队友确认后退出
    └── 删除团队 → TeamDeleteTool → 清理所有资源
```

### 5.4 Fork子Agent机制

`forkSubagent.ts` 实现了一种特殊的子Agent模式，专门为最大化 prompt cache 命中率设计：

```typescript
// Fork子Agent与父Agent共享：
// 1. 系统提示词
// 2. 用户上下文
// 3. 工具集
// 4. 模型
// 5. 上下文消息前缀

// Fork子Agent独立的：
// 1. 新的对话消息
// 2. 可变状态（fileStateCache等）
// 3. 独立的usage跟踪
// 4. 独立的AbortController
```

---

## 6. 数据交互与通信机制

### 6.1 邮箱系统（Teammate Mailbox）

这是 Swarm 模式下 Agent 间通信的核心基础设施：

```
存储路径：~/.claude/teams/{team_name}/inboxes/{agent_name}.json

消息格式：
{
  from: string       // 发送者名称
  text: string       // 消息内容（可以是JSON结构）
  timestamp: string  // ISO时间戳
  read: boolean      // 是否已读
  color?: string     // 发送者颜色
  summary?: string   // 5-10字预览摘要
}
```

通信流程：
```
Agent A                     文件系统                    Agent B
   │                           │                          │
   │── writeToMailbox(B) ─────►│                          │
   │                           │◄── pollInbox(B) ────────│
   │                           │── messages ──────────────►│
   │                           │                          │
   │◄── writeToMailbox(A) ────│◄── writeToMailbox(A) ────│
   │                           │                          │
```

关键实现细节：
- 使用 **文件锁**（`lockfile`）防止并发写入冲突
- 重试策略：10次重试，5-100ms指数退避
- 消息类型包括：普通消息、空闲通知、权限请求/响应、关闭请求/响应、计划审批

### 6.2 SendMessage 工具

```typescript
// src/tools/SendMessageTool/prompt.ts
// 支持的发送目标：
{
  to: "researcher"      // 按名称发送给队友
  to: "*"               // 广播给所有队友（慎用）
  to: "uds:/path.sock"  // 本地UDS socket跨会话通信
  to: "bridge:session_" // 远程Bridge跨机器通信
}
```

### 6.3 协议消息类型

```
普通消息：     { to, message, summary }
关闭请求：     { type: "shutdown_request", request_id }
关闭响应：     { type: "shutdown_response", request_id, approve }
计划审批请求：  { type: "plan_approval_request", request_id }
计划审批响应：  { type: "plan_approval_response", request_id, approve, feedback }
权限请求：     { type: "permission_request", id, toolName, input }
权限响应：     { type: "permission_response", id, decision }
空闲通知：     { type: "idle_notification", agentName, reason, summary }
```

### 6.4 Agent摘要生成

```typescript
// src/services/AgentSummary/agentSummary.ts
// Agent完成后自动生成摘要，用于：
// 1. 向父Agent/Leader报告完成情况
// 2. 在UI中显示Agent工作概要
// 3. 在SDK事件中传递进度信息
```

---

## 7. 执行后端架构

### 7.1 后端类型

```typescript
// src/utils/swarm/backends/types.ts
type BackendType = 'tmux' | 'iterm2' | 'in-process'

interface PaneBackend {
  type: BackendType
  displayName: string
  supportsHideShow: boolean
  isAvailable(): Promise<boolean>
  isRunningInside(): Promise<boolean>
  createTeammatePaneInSwarmView(name, color): Promise<CreatePaneResult>
  sendCommand(paneId, command): Promise<void>
  killPane(paneId): Promise<void>
  // ...
}

interface TeammateExecutor {
  spawn(config): Promise<TeammateSpawnResult>
  sendMessage(agentId, message): Promise<void>
  requestShutdown(agentId): Promise<void>
  kill(agentId): Promise<void>
}
```

### 7.2 后端检测与选择

```typescript
// src/utils/swarm/backends/registry.ts
// 检测优先级：
// 1. 如果用户已通过UI选择过，使用保存的偏好
// 2. 非交互模式 → in-process
// 3. 在tmux内 → tmux
// 4. 在iTerm2内且it2 CLI可用 → iTerm2
// 5. tmux已安装 → tmux（外部会话）
// 6. 降级到 in-process
```

### 7.3 Tmux 后端

```
┌──────────────────────────────────────────────┐
│ tmux session: claude-swarm-{pid}              │
│                                               │
│  ┌─────────────┐  ┌─────────────┐            │
│  │  team-lead  │  │ researcher  │            │
│  │  (主Agent)  │  │  (队友1)    │            │
│  │             │  │             │            │
│  └─────────────┘  └─────────────┘            │
│  ┌─────────────┐  ┌─────────────┐            │
│  │  coder      │  │  reviewer   │            │
│  │  (队友2)    │  │  (队友3)    │            │
│  │             │  │             │            │
│  └─────────────┘  └─────────────┘            │
└──────────────────────────────────────────────┘
```

每个 tmux pane 运行一个独立的 Claude Code 进程，通过 CLI 参数传递队友身份：

```bash
claude --agent-id "researcher@team-1" \
       --team-name "team-1" \
       --agent-color "blue" \
       --parent-session-id "sess_xxx" \
       --dangerously-skip-permissions  # 继承父Agent权限模式
```

### 7.4 In-Process 后端

进程内后端是最轻量的选择，不需要外部依赖：

```typescript
// src/utils/swarm/spawnInProcess.ts
// 特点：
// 1. 不启动新进程，在同一Node.js进程内运行
// 2. 使用 AsyncLocalStorage 实现上下文隔离
// 3. 共享API客户端、MCP连接等资源
// 4. 通过 AbortController 终止（不是 kill-pane）
// 5. 通信仍然使用文件邮箱（保持一致性）
```

```typescript
// 上下文隔离的关键：
export function runWithTeammateContext<T>(
  context: TeammateContext,
  fn: () => T,
): T {
  return teammateContextStorage.run(context, fn)
}
```

---

## 8. 权限同步与安全模型

### 8.1 权限同步机制

当 Worker Agent 遇到需要用户审批的操作时：

```
Worker Agent                   Leader Agent                  User
     │                              │                          │
     │ 检测到需要权限                 │                          │
     │                              │                          │
     │── permission_request ────────►│                          │
     │   {toolName, input, id}      │── 展示权限提示 ──────────►│
     │                              │                          │
     │                              │◄── approve/deny ─────────│
     │                              │                          │
     │◄── permission_response ──────│                          │
     │   {id, decision}             │                          │
     │                              │                          │
     │ 继续执行/拒绝                  │                          │
```

### 8.2 权限传播

```typescript
// 创建队友时继承权限模式
function buildInheritedCliFlags(options) {
  if (planModeRequired) {
    // Plan mode 优先于 bypass，安全考量
  } else if (permissionMode === 'bypassPermissions') {
    flags.push('--dangerously-skip-permissions')
  } else if (permissionMode === 'acceptEdits') {
    flags.push('--permission-mode acceptEdits')
  } else if (permissionMode === 'auto') {
    flags.push('--permission-mode auto')
  }
}
```

### 8.3 工具过滤

```typescript
// src/tools/AgentTool/agentToolUtils.ts
// 不同类型的Agent有不同的工具限制：

// 所有Agent都禁止的工具：
const ALL_AGENT_DISALLOWED_TOOLS = [
  'TeamCreate', 'TeamDelete', 'SendMessage',
  'TaskCreate', 'TaskGet', 'TaskList', 'TaskUpdate', 'TaskStop',
  'TaskOutput', 'EnterPlanMode', 'ExitPlanMode', ...
]

// 异步Agent允许的工具（白名单）：
const ASYNC_AGENT_ALLOWED_TOOLS = [
  'Bash', 'FileRead', 'FileEdit', 'FileWrite',
  'Glob', 'Grep', 'Agent', 'WebFetch', 'WebSearch',
  'Notebook', 'LSP', 'TodoWrite', ...
]

// 进程内队友允许的额外工具：
const IN_PROCESS_TEAMMATE_ALLOWED_TOOLS = [
  ...ASYNC_AGENT_ALLOWED_TOOLS,
  'SendMessage', 'TaskCreate', 'TaskGet',
  'TaskList', 'TaskUpdate', ...
]
```

---

## 9. Agent 记忆与状态持久化

### 9.1 Agent 记忆范围

```typescript
// src/tools/AgentTool/agentMemory.ts
type AgentMemoryScope = 'user' | 'project' | 'local'

// 'user'    → ~/.claude/agent-memory/{agentType}/
// 'project' → {cwd}/.claude/agent-memory/{agentType}/
// 'local'   → {cwd}/.claude/agent-memory-local/{agentType}/
```

记忆以 Markdown 文件存储在对应目录中，Agent 每次启动时自动加载相关记忆作为上下文的一部分。

### 9.2 记忆快照同步

```typescript
// src/tools/AgentTool/agentMemorySnapshot.ts
// Agent 可以保存/恢复记忆快照
// 用于跨会话的 Agent 记忆持续性
checkAgentMemorySnapshot()    // 检查是否有可恢复的快照
initializeFromSnapshot()       // 从快照恢复
```

### 9.3 团队共享记忆

```typescript
// src/utils/teamMemoryOps.ts
// 团队成员可以共享记忆文件
// 路径格式：.claude/teams/{team}/shared-memory/
// 支持读/写/搜索操作
// 由 teamMemorySync 服务负责同步
```

### 9.4 对话历史持久化

```
每个Agent的对话历史保存在：
~/.claude/sessions/{sessionId}/agents/{agentId}/transcript.json

包含完整的消息列表，可用于：
- Agent恢复（resumeAgent）
- 会话回顾
- 调试和分析
```

---

## 10. 工作流集成场景

### 10.1 何时会触发多Agent

#### 场景1：LLM 自主决策 spawn 子Agent

最常见的场景。主 Agent 在处理用户请求时，根据内置 Agent 的 `whenToUse` 描述自主判断：

```
用户："帮我找到所有使用了 deprecated API 的文件"
主Agent推理 → 这是一个搜索密集型任务
         → Explore Agent 的描述匹配
         → 调用 AgentTool(agent_type="Explore", prompt="...")
```

#### 场景2：Coordinator 模式

当启用 Coordinator 模式时，主 Agent 变成纯粹的"调度器"：

```typescript
// src/coordinator/coordinatorMode.ts
// Coordinator 模式下：
// - 主Agent不直接执行任何文件操作
// - 所有实际工作都委派给后台 Worker Agent
// - 主Agent专注于任务分解和进度监控
// - Worker 有独立的工具集（Bash, FileRead, FileEdit等）
```

#### 场景3：UltraPlan（超级规划）

```
用户请求复杂特性 → /ultraplan 命令
  → 生成多个 Plan Agent（不同视角）
  → 并行探索代码库
  → 汇总为统一实施计划
  → 可选：生成 Worker Agent 执行计划
```

#### 场景4：团队协作（Swarm）

```
用户："创建一个团队来重构这个模块"
  → TeamCreateTool → 创建团队配置
  → AgentTool(name="frontend-dev", prompt="...", team_name="refactor-team")
  → AgentTool(name="backend-dev", prompt="...", team_name="refactor-team")
  → AgentTool(name="test-writer", prompt="...", team_name="refactor-team")
  → 三个队友并行工作
  → 通过 SendMessage 协调
  → Leader 监控进度并汇总
```

#### 场景5：后台验证

```
主Agent完成代码修改后
  → 自动 spawn Verification Agent（后台）
  → Verification Agent 运行测试、检查边界情况
  → 完成后通知主Agent
  → 主Agent向用户报告验证结果
```

#### 场景6：远程Agent（Background PR）

```
用户："帮我修复这个PR上的所有review comments"
  → 创建 RemoteAgentTask
  → 在 CCR（Claude Code Remote）环境中执行
  → 远程Agent有完整的开发环境
  → 通过轮询获取进度和结果
```

### 10.2 Agent间的数据流

```
                    ┌─────────────────────┐
                    │    User Message     │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │    Main Agent       │
                    │    (Team Lead)      │
                    └──────────┬──────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
     ┌────────▼────────┐ ┌────▼────┐ ┌────────▼────────┐
     │  Explore Agent  │ │  Plan   │ │  Worker Agent   │
     │  (搜索结果)      │ │  Agent  │ │  (代码修改)      │
     └────────┬────────┘ │ (方案)  │ └────────┬────────┘
              │          └────┬────┘          │
              │               │               │
              └───────────────┼───────────────┘
                              │
                   ┌──────────▼──────────┐
                   │  聚合到主Agent      │
                   │  → 综合报告给用户    │
                   └─────────────────────┘

数据流向：
  同步子Agent: 返回值直接合并到父Agent消息流
  异步Agent:   通过 TaskOutput 文件传递结果
  Swarm队友:   通过 Mailbox 消息传递
  远程Agent:   通过 HTTP API 轮询事件流
```

### 10.3 Task 管理工具集

LLM 通过以下工具管理后台任务：

| 工具 | 用途 |
|------|------|
| `TaskCreateTool` | 创建新的后台任务 |
| `TaskGetTool` | 获取任务详情 |
| `TaskListTool` | 列出所有任务 |
| `TaskUpdateTool` | 更新任务状态/描述 |
| `TaskStopTool` | 停止/终止任务 |
| `TaskOutputTool` | 读取任务输出 |
| `TeamCreateTool` | 创建Agent团队 |
| `TeamDeleteTool` | 删除Agent团队 |
| `SendMessageTool` | Agent间消息传递 |

---

## 11. UI 呈现架构

### 11.1 Agent进度展示

```typescript
// src/components/AgentProgressLine.tsx
// 在对话流中内联展示Agent进度：
// ┌─────────────────────────────────────────┐
// │ 🔵 Explore Agent                        │
// │ ├── Reading src/utils/agent.ts           │
// │ ├── Searching for "spawn"               │
// │ └── 3 tools, 12.5k tokens              │
// └─────────────────────────────────────────┘
```

### 11.2 Coordinator 面板

```typescript
// src/components/CoordinatorAgentStatus.tsx (CoordinatorTaskPanel)
// 在Coordinator模式下展示所有后台Worker：
// ┌─────────────────────────────────────────┐
// │ Workers:                                 │
// │  [1] 🟢 frontend-dev  Working on UI     │
// │  [2] 🟡 backend-dev   Running tests     │
// │  [3] ✅ test-writer   Completed (2m)    │
// │                                          │
// │ Enter: view  x: dismiss  ↑↓: navigate   │
// └─────────────────────────────────────────┘
```

### 11.3 团队状态面板

```typescript
// src/components/teams/TeamsDialog.tsx
// 展示团队成员状态、消息历史：
// ┌─────────────────────────────────────────┐
// │ Team: refactor-team                      │
// │                                          │
// │ Members:                                 │
// │  🔴 researcher  - Running               │
// │  🔵 coder       - Idle                  │
// │  🟢 reviewer    - Running               │
// │                                          │
// │ Recent Messages:                         │
// │  researcher → coder: "Found the entry..."│
// │  coder → team-lead: "Done with task #1" │
// └─────────────────────────────────────────┘
```

### 11.4 后台任务对话框

```typescript
// src/components/tasks/BackgroundTasksDialog.tsx (113KB!)
// 最大的UI组件之一，功能包括：
// - 任务列表（本地+远程）
// - 任务详情查看
// - 实时输出流
// - 任务控制（暂停/恢复/终止）
// - Todo列表展示
```

### 11.5 队友视图

```typescript
// src/components/TeammateViewHeader.tsx
// 可以"进入"某个队友的视角查看其对话历史：
// ┌─────────────────────────────────────────┐
// │ 🔵 Viewing: researcher@team-1           │
// │ [Esc to return to main]                 │
// │                                          │
// │ Human: Search for all API endpoints...   │
// │ Assistant: I'll start by searching...    │
// │ [Tool: Grep] pattern="@app.route"        │
// │ ...                                      │
// └─────────────────────────────────────────┘
```

### 11.6 Spinner 集成

```typescript
// src/components/Spinner/TeammateSpinnerLine.tsx
// src/components/Spinner/TeammateSpinnerTree.tsx
// 在等待Agent响应时展示的动画Spinner：
// - 树形展示队友工作状态
// - 颜色编码匹配队友颜色
// - 实时更新工具使用进度
```

---

## 12. 关键技术点总结

### 12.1 AsyncLocalStorage 的创造性使用

这是整个多Agent系统最核心的技术基础。Node.js 的 `AsyncLocalStorage` 被用来在同一进程内实现多个 Agent 的上下文隔离，避免了多进程的开销：

```typescript
const agentContextStorage = new AsyncLocalStorage<AgentContext>()

// 每个Agent在自己的上下文中运行
function runWithAgentContext<T>(context: AgentContext, fn: () => T): T {
  return agentContextStorage.run(context, fn)
}

// 在任何异步操作中都能获取当前Agent的上下文
function getAgentContext(): AgentContext | undefined {
  return agentContextStorage.getStore()
}
```

### 12.2 文件系统作为消息总线

没有使用任何外部消息队列或IPC机制，而是直接使用文件系统：
- **优点**：零外部依赖、天然持久化、跨进程/跨机器兼容
- **挑战**：并发控制通过 `lockfile` 实现
- **路径约定**：`~/.claude/teams/{team}/inboxes/{agent}.json`

### 12.3 Prompt Cache 感知的 Agent 设计

子Agent的创建被精心设计以最大化 Anthropic API 的 prompt cache 命中率：
- `CacheSafeParams` 类型确保关键参数一致
- Fork 子Agent 继承父 Agent 的消息前缀
- 避免不必要的 `maxOutputTokens` 变更（会导致 `budget_tokens` 变化从而破坏缓存）

### 12.4 多后端适配器模式

执行后端通过统一的 `TeammateExecutor` 接口抽象，支持：
- **tmux**：最完整的体验，独立进程，独立终端面板
- **iTerm2**：macOS原生分屏，用户体验最好
- **in-process**：最轻量，无外部依赖，适合IDE集成

后端选择是自动检测的，且会缓存结果。

### 12.5 优雅的权限代理

Swarm 中的权限审批不是每个 Worker 独立弹窗，而是集中到 Leader 处理：
- Worker 发送权限请求到 Leader 邮箱
- Leader 在其UI中展示（可能还有用户交互）
- 响应通过邮箱返回
- Worker 轮询邮箱获取决策

### 12.6 渐进式降级策略

整个系统在各个层面都有降级策略：
- 没有 tmux/iTerm2 → 降级到 in-process
- 没有团队功能 → 降级到独立子Agent
- 远程Agent不可用 → 降级到本地Agent
- Agent记忆不可用 → 无记忆启动

---

## 13. 核心文件索引

### Agent 核心

| 路径 | 描述 |
|------|------|
| `src/tools/AgentTool/AgentTool.tsx` | Agent工具主逻辑（同步/异步决策） |
| `src/tools/AgentTool/runAgent.ts` | Agent查询循环 |
| `src/tools/AgentTool/forkSubagent.ts` | Fork子Agent机制 |
| `src/tools/AgentTool/builtInAgents.ts` | 内置Agent注册 |
| `src/tools/AgentTool/loadAgentsDir.ts` | Agent定义加载（MD/JSON） |
| `src/tools/AgentTool/resumeAgent.ts` | Agent恢复/续接 |
| `src/tools/AgentTool/agentToolUtils.ts` | Agent工具过滤和辅助函数 |
| `src/tools/AgentTool/agentMemory.ts` | Agent持久化记忆 |
| `src/tools/AgentTool/agentMemorySnapshot.ts` | 记忆快照同步 |
| `src/tools/AgentTool/agentColorManager.ts` | Agent颜色分配 |

### 内置Agent定义

| 路径 | Agent类型 |
|------|----------|
| `src/tools/AgentTool/built-in/generalPurposeAgent.ts` | 通用Agent |
| `src/tools/AgentTool/built-in/exploreAgent.ts` | 代码探索Agent |
| `src/tools/AgentTool/built-in/planAgent.ts` | 方案规划Agent |
| `src/tools/AgentTool/built-in/verificationAgent.ts` | 验证Agent |
| `src/tools/AgentTool/built-in/claudeCodeGuideAgent.ts` | 使用引导Agent |

### Swarm基础设施

| 路径 | 描述 |
|------|------|
| `src/utils/swarm/inProcessRunner.ts` | 进程内队友运行器 |
| `src/utils/swarm/spawnInProcess.ts` | 进程内队友生成 |
| `src/utils/swarm/permissionSync.ts` | 跨Agent权限同步 |
| `src/utils/swarm/teamHelpers.ts` | 团队文件读写 |
| `src/utils/swarm/teammateInit.ts` | 队友初始化钩子 |
| `src/utils/swarm/teammateLayoutManager.ts` | 终端布局管理 |
| `src/utils/swarm/backends/registry.ts` | 后端注册中心 |
| `src/utils/swarm/backends/TmuxBackend.ts` | tmux后端 |
| `src/utils/swarm/backends/ITermBackend.ts` | iTerm2后端 |
| `src/utils/swarm/backends/InProcessBackend.ts` | 进程内后端 |

### 通信与协作

| 路径 | 描述 |
|------|------|
| `src/utils/teammateMailbox.ts` | 队友邮箱（文件消息系统） |
| `src/tools/SendMessageTool/SendMessageTool.ts` | Agent间消息发送 |
| `src/tools/TeamCreateTool/TeamCreateTool.ts` | 团队创建 |
| `src/tools/TeamDeleteTool/TeamDeleteTool.ts` | 团队删除 |
| `src/utils/teamDiscovery.ts` | 团队发现 |
| `src/utils/teamMemoryOps.ts` | 团队共享记忆 |

### 任务管理

| 路径 | 描述 |
|------|------|
| `src/Task.ts` | 任务类型定义 |
| `src/tasks/LocalAgentTask/LocalAgentTask.tsx` | 本地Agent任务 |
| `src/tasks/RemoteAgentTask/RemoteAgentTask.tsx` | 远程Agent任务 |
| `src/tasks/InProcessTeammateTask/InProcessTeammateTask.tsx` | 进程内队友任务 |
| `src/tools/TaskCreateTool/TaskCreateTool.ts` | 任务创建工具 |
| `src/tools/TaskOutputTool/TaskOutputTool.tsx` | 任务输出读取 |

### 上下文与隔离

| 路径 | 描述 |
|------|------|
| `src/utils/agentContext.ts` | Agent上下文（AsyncLocalStorage） |
| `src/utils/teammateContext.ts` | 队友上下文 |
| `src/utils/forkedAgent.ts` | Fork Agent执行器 |
| `src/utils/teammate.ts` | 队友身份工具函数 |

### Coordinator模式

| 路径 | 描述 |
|------|------|
| `src/coordinator/coordinatorMode.ts` | 协调器模式开关与上下文 |
| `src/components/CoordinatorAgentStatus.tsx` | 协调器UI面板 |

### UI组件

| 路径 | 描述 |
|------|------|
| `src/components/AgentProgressLine.tsx` | Agent进度行 |
| `src/components/tasks/BackgroundTasksDialog.tsx` | 后台任务对话框 |
| `src/components/tasks/BackgroundTaskStatus.tsx` | 后台任务状态 |
| `src/components/teams/TeamsDialog.tsx` | 团队对话框 |
| `src/components/teams/TeamStatus.tsx` | 团队状态 |
| `src/components/TeammateViewHeader.tsx` | 队友视图头部 |
| `src/components/Spinner/TeammateSpinnerLine.tsx` | 队友Spinner |
| `src/components/agents/AgentsList.tsx` | Agent列表 |
| `src/components/agents/AgentsMenu.tsx` | Agent管理菜单 |

---

## 附录：架构总结图

```
┌─────────────────────────────────────────────────────────────────┐
│                        Claude Code                               │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │                     UI Layer (Ink/React)                   │   │
│  │  AgentProgressLine │ CoordinatorPanel │ TeamsDialog │ ...  │   │
│  └───────────────────────────────┬───────────────────────────┘   │
│                                  │                                │
│  ┌───────────────────────────────▼───────────────────────────┐   │
│  │                   Tool Layer                               │   │
│  │  AgentTool │ SendMessage │ TeamCreate │ TaskXxx │ ...      │   │
│  └───────────────────────────────┬───────────────────────────┘   │
│                                  │                                │
│  ┌───────────────────────────────▼───────────────────────────┐   │
│  │                   Agent Runtime                            │   │
│  │  runAgent │ forkSubagent │ inProcessRunner │ forkedAgent   │   │
│  └───────────────────────────────┬───────────────────────────┘   │
│                                  │                                │
│  ┌───────────────────────────────▼───────────────────────────┐   │
│  │                   Task Management                          │   │
│  │  LocalAgentTask │ RemoteAgentTask │ InProcessTeammateTask  │   │
│  └───────────────────────────────┬───────────────────────────┘   │
│                                  │                                │
│  ┌───────────────────────────────▼───────────────────────────┐   │
│  │                   Communication Layer                      │   │
│  │  teammateMailbox │ permissionSync │ teamMemorySync         │   │
│  └───────────────────────────────┬───────────────────────────┘   │
│                                  │                                │
│  ┌───────────────────────────────▼───────────────────────────┐   │
│  │                   Execution Backends                       │   │
│  │  TmuxBackend │ ITermBackend │ InProcessBackend │ Remote    │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │                   Context Isolation                        │   │
│  │  AsyncLocalStorage │ AgentContext │ TeammateContext         │   │
│  └───────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

*本文档基于 claude-code-main 源码的静态分析生成，反映了截至分析时的代码状态。*
