# Claude Code Go — 总体架构设计文档

> 版本：v2.0
> 日期：2026-04-02
> 状态：已归档，所有 Agent 启动前必读

---

## 目录

1. [项目概述](#1-项目概述)
2. [整体架构](#2-整体架构)
3. [模块定义](#3-模块定义)
4. [核心技术决策](#4-核心技术决策)
5. [关键数据流](#5-关键数据流)
6. [外部依赖清单](#6-外部依赖清单)
7. [模块负责 Agent 索引](#7-模块负责-agent-索引)

---

## 1. 项目概述

### 重写目标

将 Claude Code（TypeScript / Bun 运行时）**全量重写**为 Go 语言版本。
**目标**：保持与原版本完全相同的功能语义和用户体验，按 Go 语言特性做惯用化实现，不裁剪任何功能。

重写范围涵盖：完整 CLI 入口、核心 LLM 查询引擎、全部内置工具（~40 个）、TUI 界面、MCP 客户端、权限系统、配置系统、多 Agent 协调、会话管理等。

### 与原版本的关系

原版本是事实参考标准。各模块开发 Agent 需深入阅读原始 TS 代码，理解其行为语义后，以 Go 惯用方式实现——**不要照搬文件结构，要复刻功能语义**。

---

## 2. 整体架构

### 2.1 分层架构

```
┌─────────────────────────────────────────┐
│              入口层 (Entry)              │
│          CLI 命令解析与程序启动            │
└──────────────────┬──────────────────────┘
                   │
┌──────────────────▼──────────────────────┐
│              TUI 层 (UI)                │
│     终端用户界面，渲染与用户交互           │
└──────────────────┬──────────────────────┘
                   │
┌──────────────────▼──────────────────────┐
│             核心层 (Core)               │
│  LLM 查询引擎 | 工具编排 | 权限检查        │
│  应用状态管理 | 上下文压缩                 │
└────┬──────────┬──────────┬──────────────┘
     │          │          │
┌────▼──┐  ┌───▼───┐  ┌───▼────────────┐
│工具层  │  │服务层  │  │  基础设施层     │
│Tools  │  │Svc    │  │  Config/State  │
└───────┘  └───────┘  └────────────────┘
```

### 2.2 模块依赖方向（严格单向，禁止循环）

```
入口层
  └─► TUI 层
        └─► 核心层
              ├─► 工具层
              ├─► 服务层
              └─► 基础设施层
                    └─► 公共类型包（pkg/types）

规则：
- 下层模块禁止依赖上层模块
- 同层平行模块原则上禁止互相依赖
- pkg/types 是唯一零依赖的公共基础，所有层均可依赖
```

---

## 3. 模块定义

### 3.1 模块总览

| 模块 | 职责概述 | 层次 |
|------|---------|------|
| **CLI 入口** | 命令行解析、程序启动、模式分发 | 入口层 |
| **TUI** | 终端界面渲染、用户输入处理、交互状态管理 | TUI 层 |
| **查询引擎** | LLM 会话管理、主循环、工具编排、消息历史 | 核心层 |
| **工具系统** | Tool 接口规范、所有内置工具的实现 | 工具层 |
| **权限系统** | 工具调用权限检查、规则匹配、用户确认 | 核心层 |
| **上下文压缩** | 对话历史自动压缩，保证 token 窗口不溢出 | 核心层 |
| **多 Agent 协调** | 子 Agent 调度、Swarm 通信、任务协调 | 核心层 |
| **Slash 命令** | /斜杠命令注册与执行 | 核心层 |
| **Hooks 系统** | Pre/Post tool hooks、session hooks、sampling hooks | 核心层 |
| **API 客户端** | Anthropic API SSE 流式调用、重试、用量统计 | 服务层 |
| **MCP 客户端** | MCP 协议连接管理、远程工具/资源调用 | 服务层 |
| **OAuth** | OAuth2 认证流程、Token 管理 | 服务层 |
| **配置系统** | 全局/项目/本地三级配置文件读写 | 基础设施层 |
| **应用状态** | 运行时全局状态并发安全存储与订阅 | 基础设施层 |
| **会话存储** | 对话 transcript 持久化、会话恢复 | 基础设施层 |
| **公共类型** | 跨模块共享的数据类型定义（零依赖） | 公共包 |

---

### 3.2 各模块详细定义

#### CLI 入口模块

**职责**：程序的唯一入口点。负责解析命令行参数、初始化运行环境、根据模式（交互式 REPL / 非交互式 print / SDK 服务 / 诊断等）分发到对应的处理路径。

**边界**：
- 只做启动引导和参数路由，不包含业务逻辑
- 负责初始化各模块（依赖注入），但不管理运行时状态

**关键功能**：交互式 REPL 模式、非交互式（`-p`）模式、`claude api` SDK 模式、`claude mcp` MCP 管理、`claude doctor` 诊断、`claude config` 配置管理、会话恢复（`--resume`）、自动更新

---

#### TUI 模块

**职责**：基于 BubbleTea（Elm 架构：Model-Update-View）实现终端用户界面。负责所有 UI 渲染、键盘输入处理、用户交互状态管理，以及将用户操作转化为引擎指令。

**边界**：
- 只负责界面呈现与用户交互，不执行任何业务逻辑
- 从查询引擎接收消息流，异步渲染到终端
- 向查询引擎提交用户输入

**关键功能**：
- REPL 主界面（消息历史、输入框、状态栏）
- 流式输出渲染（助手回复、工具调用/结果）
- 权限请求对话框（工具调用需要用户确认时弹出）
- 加载动画、进度展示
- 主题与样式系统（对应 Lip Gloss）

---

#### 查询引擎模块

**职责**：整个系统的核心驱动力。管理单次 Claude 会话的完整生命周期：维护消息历史、驱动 LLM 请求-响应主循环、编排工具调用、管理 token 预算、触发上下文压缩。

**边界**：
- 是 TUI 层与底层服务/工具之间的中枢
- 不直接渲染 UI，通过 channel 向 TUI 推送消息流
- 通过接口调用工具，不感知具体工具实现

**关键功能**：
- 会话生命周期管理（消息历史、cost 累计、turn 计数）
- LLM 主循环：发送请求 → 接收流式响应 → 检测 tool_use → 执行工具 → 追加结果 → 继续循环
- 工具并发编排：将工具调用划分为可并发批次（只读）和串行批次（写操作）
- 系统提示构建（内存文件注入、coordinator 上下文注入）
- Token 预算管理（大结果截断/替换）

---

#### 工具系统模块

**职责**：定义统一的 Tool 接口规范，并实现全部内置工具（~40 个）。每个工具封装一种外部能力（文件操作、Shell 执行、网络请求、Agent 调度等）。

**工具接口**：所有工具实现同一接口，包含：工具名称/描述/输入 Schema、权限类型声明、执行逻辑（返回结构化结果）。

**内置工具分类**：

| 类别 | 工具 |
|------|------|
| 文件操作 | 文件读取、文件写入、文件编辑、Glob 搜索、Grep 搜索、Notebook 编辑 |
| Shell 执行 | Bash 工具（含沙箱判断、危险命令检测） |
| Agent 调度 | AgentTool（子 Agent 派发）、SendMessage（Swarm 通信） |
| MCP | MCPTool（调用 MCP 服务工具）、MCP 资源读取 |
| 网络 | WebFetch、WebSearch |
| 任务管理 | TaskCreate/Get/List/Update/Stop/Output |
| 用户交互 | AskUserQuestion、TodoWrite |
| 模式控制 | EnterPlanMode/ExitPlanMode、EnterWorktree/ExitWorktree |
| 其他 | SkillTool、CronTool、SleepTool、REPLTool、BriefTool、SyntheticOutput 等 |

**边界**：
- 工具之间禁止互相依赖
- 工具不感知 TUI 层（不直接渲染）
- 权限检查通过注入的权限系统接口完成

---

#### 权限系统模块

**职责**：在工具执行前检查调用权限。根据配置的规则（allow/deny/ask）和工具的权限类型，给出 allow / deny / ask-user 三种决策。ask-user 时向 TUI 发起确认请求，等待用户响应。

**边界**：
- 只做权限决策，不执行工具
- 不依赖查询引擎、TUI 或具体工具实现

**关键功能**：
- 多级权限规则匹配（全局 / 项目 / 本地配置叠加）
- 文件路径权限校验（允许/拒绝访问特定路径）
- Shell 命令规则匹配（精确匹配 + 通配符）
- 拒绝原因跟踪（记录权限拒绝历史，用于系统提示注入）

---

#### 上下文压缩模块

**职责**：当对话历史的 token 数接近模型上下文窗口上限时，自动触发压缩策略，保证对话可以持续进行。

**边界**：
- 由查询引擎在每轮循环中调用，不主动触发
- 压缩策略需保留关键上下文，不能破坏对话连贯性

**关键功能**：
- Auto-compact：超过 token 阈值时自动调用 LLM 总结历史
- Micro-compact：针对超长单条消息的局部压缩
- Snip-compact：截断并标记被移除的历史片段

---

#### 多 Agent 协调模块

**职责**：支持 Claude Code 的 multi-agent 运行模式。协调多个并发 Agent 实例，管理 Agent 间消息传递（Swarm），注入协调上下文到系统提示中。

**边界**：
- 与 AgentTool（在工具系统中）协作，但属于不同层：协调模块负责运行时编排，AgentTool 负责单次子 Agent 调用
- 不直接管理 TUI，但可通过 channel 与主进程通信

**关键功能**：
- Coordinator 模式上下文注入（告知 Claude 当前处于协调模式）
- 子 Agent fork 与生命周期管理
- Agent 间消息路由（SendMessage）

---

#### Slash 命令模块

**职责**：实现 `/` 前缀的命令系统（如 `/clear`、`/compact`、`/config`、`/help` 等）。负责命令注册、解析和执行。

**边界**：
- 命令在 TUI 层的输入阶段被识别和截获，交由本模块执行
- 命令可以修改应用状态、触发引擎操作，但通过接口调用，不直接耦合

---

#### Hooks 系统模块

**职责**：提供工具调用前后的钩子执行能力。允许用户在 settings 中配置 shell 命令或自定义处理器，在工具使用前/后、采样前/后、会话开始/结束时自动执行。

**边界**：
- 由查询引擎在工具执行前后调用
- 钩子执行结果可阻断工具调用（PreToolUse hook 返回 block）

---

#### API 客户端模块

**职责**：封装与 Anthropic API 的所有 HTTP 通信。负责构建请求、处理 SSE 流式响应、错误重试、用量统计。

**边界**：
- 只负责网络通信，不了解 Claude 的业务逻辑
- 不依赖任何 internal 业务包

**关键功能**：
- SSE 流式响应解析（content_block_delta / tool_use 事件）
- 指数退避重试（速率限制、临时错误）
- Token 用量累计与成本计算
- Fallback 模型切换（主模型失败时切换备用模型）

---

#### MCP 客户端模块

**职责**：实现 MCP（Model Context Protocol）客户端，连接并管理外部 MCP 服务器，暴露其工具和资源供 Claude 调用。

**边界**：
- 管理 MCP 连接的生命周期（连接、重连、断开）
- 将 MCP 工具适配为系统内统一的 Tool 接口

**关键功能**：
- 多 Transport 支持（stdio、SSE、HTTP）
- MCP 连接池管理
- 工具名规范化
- MCP OAuth 认证

---

#### OAuth 模块

**职责**：管理 Anthropic 平台的 OAuth2 认证流程，包括授权码流程、Token 存储与刷新、系统 Keychain 安全存储。

---

#### 配置系统模块

**职责**：管理三级配置文件的读写：全局配置（`~/.claude/`）、项目配置（`.claude/`）、本地覆盖配置（`.claude.local`）。支持配置合并与优先级覆盖。

**边界**：
- 只做配置文件 I/O，不包含业务判断
- 不依赖任何 internal 业务模块

---

#### 应用状态模块

**职责**：提供运行时全局应用状态的并发安全存储。支持状态订阅（状态变更时通知监听者）。对应原版的 AppState + Zustand store。

**边界**：
- 不包含业务逻辑，只做状态存取
- 不依赖查询引擎、TUI、工具等上层模块（防止循环依赖）

---

#### 会话存储模块

**职责**：对话 transcript 的持久化存储与读取。支持 `--resume` 功能（从上次对话断点恢复）。

---

#### 公共类型包

**职责**：定义所有跨模块共享的核心数据类型（Message、PermissionMode、ToolResult 等）。**零外部依赖**，所有模块均可安全引用。

---

## 4. 核心技术决策

### 4.1 TUI 框架：BubbleTea + Lip Gloss

原版使用 React + Ink（React 渲染到终端），Go 版本使用 BubbleTea（Elm 架构：Model-Update-View）。

**关键映射**：
- React 组件 → `tea.Model` 接口
- `useState` / `setState` → Model struct 字段 + `Update()` 返回新 Model
- `useEffect` + 异步 → `tea.Cmd`（副作用，返回 `tea.Msg`）
- 外部事件推送 → `tea.Program.Send(msg)`
- Ink 样式 → Lip Gloss style

### 4.2 并发模型：goroutine + channel

原版使用 `async/await` + `AsyncGenerator`，Go 版本使用 goroutine + channel。

**核心规则**：
- LLM 主循环在独立 goroutine 运行，通过 channel 向 TUI 推送消息流
- 可并发的工具调用（只读工具）使用 `errgroup` 并发执行
- 写操作工具必须串行执行，避免文件系统竞争
- 全局状态使用 `sync.RWMutex` 保护，读多写少场景优先读锁
- 上下文取消统一使用 `context.Context` 传递

### 4.3 错误处理策略

- 业务错误：自定义错误类型（实现 `error` 接口），携带结构化信息
- 用户中止：统一使用 `AbortError` 类型，可被上层识别并优雅退出
- API 错误：区分可重试（速率限制、临时故障）与不可重试（认证失败、输入错误）
- 工具错误：工具失败不终止主循环，返回错误结果给 LLM 继续处理

### 4.4 配置优先级

```
本地配置（.claude/settings.local.json）
    覆盖 ↓
项目配置（.claude/settings.json）
    覆盖 ↓
全局配置（~/.claude/settings.json）
    覆盖 ↓
默认值
```

### 4.5 权限决策流程

```
工具调用请求
    ↓
查 配置规则（allow/deny 列表）→ 匹配 → allow / deny
    ↓ 无匹配
查 工具默认权限类型
    ↓ 需要确认
向 TUI 发送 PermissionRequest → 等待用户响应 → allow / deny
```

---

## 5. 关键数据流

### 5.1 LLM 查询主循环

```
用户输入
    ↓
[TUI] 接收输入 → 提交给查询引擎
    ↓
[查询引擎] 处理输入（注入内存/系统提示、应用 token 预算）
    ↓
[查询引擎] 调用 API 客户端，获取 SSE 流
    ↓
[查询引擎] 解析流事件：
    - 文字内容 → 推送到 TUI 渲染
    - tool_use → 进入工具编排
    ↓
[查询引擎] 工具编排：
    - 权限检查 → ask-user 时暂停等待 TUI 用户确认
    - 并发/串行执行工具
    - 收集 tool_result
    ↓
[查询引擎] 追加 tool_result，继续下一轮循环
    ↓
直到 stop_reason == "end_turn" 或达到最大轮数
    ↓
[TUI] 渲染最终结果，恢复输入
```

### 5.2 工具权限检查流程

```
工具执行请求
    ↓
[权限系统] 检查规则
    ├── allow → 直接执行工具
    ├── deny  → 返回拒绝结果，不执行
    └── ask   → 向 TUI 发送权限确认请求
                    ↓
               [TUI] 展示确认对话框
                    ↓
               用户确认 → allow → 执行工具
               用户拒绝 → deny  → 返回拒绝结果
```

### 5.3 子 Agent 调度流程

```
LLM 调用 AgentTool（含任务描述）
    ↓
[AgentTool] 创建独立 QueryEngine 实例
    ↓
[子 Agent QueryEngine] 运行独立会话
    - 拥有独立消息历史
    - 可访问部分工具（由调用方授权）
    ↓
子 Agent 完成 → 返回结果给父 Agent
    ↓
父 Agent 将结果作为 tool_result 继续
```

---

## 6. 外部依赖清单

| 依赖 | 用途 | 对应原版 |
|------|------|---------|
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API 官方 Go SDK | `@anthropic-ai/sdk` |
| `github.com/charmbracelet/bubbletea` | TUI 框架 | React + Ink |
| `github.com/charmbracelet/lipgloss` | TUI 样式 | Ink 样式系统 |
| `github.com/charmbracelet/bubbles` | TUI 通用组件（文本输入等） | Ink 组件 |
| `github.com/spf13/cobra` | CLI 命令框架 | Commander.js |
| `github.com/mark3labs/mcp-go` | MCP 协议客户端 | `@modelcontextprotocol/sdk` |
| `golang.org/x/oauth2` | OAuth2 认证 | 自研 OAuth 流程 |
| `github.com/golang-jwt/jwt` | JWT 处理 | `jsonwebtoken` |
| `go.opentelemetry.io/otel` | 链路追踪（可选） | OpenTelemetry |
| `google.golang.org/protobuf` | Protobuf 序列化 | `protobufjs` |
| `github.com/go-playground/validator` | 输入验证 | Zod |

---

## 7. 模块负责 Agent 索引

> 各 Agent 的详细设计见 `docs/project/agents/` 目录

| 模块 | 负责 Agent | 主要依赖 |
|------|-----------|---------|
| 公共类型包 | Agent-Infra | 无 |
| 配置系统、应用状态、会话存储 | Agent-Infra | 公共类型包 |
| API 客户端 | Agent-Services | 公共类型包、配置 |
| MCP 客户端 | Agent-Services | 公共类型包、配置 |
| OAuth | Agent-Services | 配置 |
| 权限系统 | Agent-Core | 公共类型包、配置、应用状态 |
| 上下文压缩 | Agent-Core | API 客户端、公共类型包 |
| 查询引擎 | Agent-Core | API 客户端、工具接口、权限系统、压缩、应用状态 |
| 工具系统（接口 + 全部内置工具） | Agent-Tools | 工具接口、权限系统、配置 |
| 多 Agent 协调 | Agent-Coordinator | 查询引擎、工具系统 |
| Hooks 系统 | Agent-Core | 配置、公共类型包 |
| Slash 命令 | Agent-Commands | 查询引擎、应用状态 |
| TUI | Agent-TUI | 查询引擎、应用状态、权限系统 |
| CLI 入口 | Agent-CLI | 所有模块（组装层） |
