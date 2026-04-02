# Claude Code Go — 总体架构设计文档

> 版本：v1.0
> 日期：2026-04-02
> 基于原始 TypeScript 代码深度分析输出

---

## 目录

1. [项目概述](#1-项目概述)
2. [整体架构图](#2-整体架构图)
3. [Go 项目包结构设计](#3-go-项目包结构设计)
4. [核心技术选型与设计决策](#4-核心技术选型与设计决策)
5. [关键数据流详解](#5-关键数据流详解)
6. [外部依赖清单](#6-外部依赖清单)
7. [模块划分与开发 Agent 索引](#7-模块划分与开发-agent-索引)
8. [关键挑战与解决方案](#8-关键挑战与解决方案)

---

## 1. 项目概述

### 重写目标与范围

将 Claude Code（TypeScript / Bun 运行时）**全量重写**为 Go 语言版本。目标是保持与原版本**完全相同的架构设计、模块分层、功能语义**，仅根据 Go 语言特性做惯用化实现（不裁剪功能）。

重写范围包括：
- 完整 CLI 入口（`main.tsx` → `cmd/`）
- 核心查询引擎与 LLM 调用循环（`QueryEngine` + `query.ts`）
- 全部 ~40 个内置 Tool（`tools/`）
- TUI 界面（React/Ink REPL → BubbleTea）
- MCP 客户端集成（`services/mcp/`）
- 权限系统（`utils/permissions/`）
- 配置系统（`utils/config.ts`）
- 多 Agent 协调（`coordinator/`, `tools/AgentTool/`）
- 服务层（API 客户端、自动压缩、Analytics 等）

**不在范围内**：纯 Ant 内部功能（VOICE_MODE, KAIROS 等特性门控模块）在 Go 版本中暂时以空实现或注释保留接口。

### 与原 TS 版本的对应关系

| TS 层/模块 | 对应 Go 包 | 说明 |
|-----------|-----------|------|
| `main.tsx` | `cmd/root.go` | CLI 入口，Cobra 根命令 |
| `QueryEngine.ts` | `internal/engine/queryengine.go` | 会话级查询引擎 |
| `query.ts` | `internal/engine/query.go` | 主循环 async generator |
| `Tool.ts` | `internal/tool/tool.go` | Tool 接口定义 |
| `tools/BashTool` | `internal/tools/bash/` | Bash 工具实现 |
| `services/api/claude.ts` | `internal/api/client.go` | Anthropic API 客户端 |
| `services/mcp/` | `internal/mcp/` | MCP 客户端 |
| `screens/REPL.tsx` | `internal/tui/repl/` | BubbleTea REPL 模型 |
| `utils/config.ts` | `internal/config/` | 配置管理 |
| `utils/permissions/` | `internal/permissions/` | 权限系统 |
| `state/AppState.ts` | `internal/state/appstate.go` | 应用状态 |
| `coordinator/` | `internal/coordinator/` | 多 Agent 协调 |

### Go 版本的核心优势

1. **单一原生二进制**：无需 Bun 运行时，部署简单
2. **更低启动延迟**：无 JS 模块评估开销（原版 main.tsx 入口有 `profileCheckpoint('main_tsx_entry')` 埋点监控启动耗时）
3. **goroutine 并发**：原版 TS 使用 `Promise.all` 并发执行只读工具，Go 用 goroutine/channel 天然表达，更高效
4. **内存控制**：Go GC 比 V8 更可预测，适合长对话 session
5. **跨平台交叉编译**：`GOOS/GOARCH` 轻松产出 Linux/macOS/Windows 二进制

---

## 2. 整体架构图

### 2.1 分层架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         入口层 (Entry Layer)                         │
│  cmd/root.go  cmd/chat.go  cmd/api.go  cmd/mcp.go  cmd/doctor.go   │
│                        (Cobra CLI Commands)                          │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────────┐
│                        TUI 层 (UI Layer)                             │
│    internal/tui/repl/     internal/tui/components/                   │
│    (BubbleTea Model)      (Lip Gloss 样式组件)                        │
│    internal/tui/screens/  internal/tui/keybindings/                  │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────────┐
│                       核心层 (Core Layer)                             │
│  ┌─────────────────────┐   ┌──────────────────────┐                 │
│  │  internal/engine/   │   │  internal/state/      │                 │
│  │  QueryEngine        │◄──│  AppState + Store     │                 │
│  │  query loop         │   │  (RWMutex 并发安全)   │                 │
│  └──────────┬──────────┘   └──────────────────────┘                 │
│             │                                                         │
│  ┌──────────▼──────────┐   ┌──────────────────────┐                 │
│  │  internal/tool/     │   │  internal/permissions/│                 │
│  │  Tool 接口           │   │  权限检查引擎          │                 │
│  │  buildTool 工厂      │   │  规则匹配              │                 │
│  └──────────┬──────────┘   └──────────────────────┘                 │
└─────────────┼───────────────────────────────────────────────────────┘
              │
┌─────────────▼───────────────────────────────────────────────────────┐
│                       工具层 (Tools Layer)                            │
│  internal/tools/                                                      │
│  ┌──────┐ ┌───────┐ ┌──────┐ ┌──────┐ ┌───────┐ ┌───────┐         │
│  │ bash │ │feedit │ │fread │ │fwrite│ │ glob  │ │ grep  │         │
│  └──────┘ └───────┘ └──────┘ └──────┘ └───────┘ └───────┘         │
│  ┌───────┐ ┌──────┐ ┌───────┐ ┌──────┐ ┌──────┐ ┌───────┐         │
│  │ agent │ │ mcp  │ │search │ │ todo │ │tasks │ │skills │         │
│  └───────┘ └──────┘ └───────┘ └──────┘ └──────┘ └───────┘         │
└─────────────────────────────┬───────────────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────────────┐
│                       服务层 (Services Layer)                         │
│  internal/api/       internal/mcp/       internal/compact/           │
│  (Anthropic HTTP)    (MCP Client)        (Auto-compact)              │
│  internal/analytics/ internal/oauth/     internal/config/            │
│  (事件日志)           (OAuth2流)          (配置读写)                   │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 模块依赖关系图

```
cmd/
  └── 依赖 → internal/tui/, internal/engine/, internal/config/,
              internal/state/, internal/mcp/, internal/tools/

internal/tui/
  └── 依赖 → internal/engine/, internal/state/, internal/tool/,
              internal/permissions/, internal/config/

internal/engine/
  └── 依赖 → internal/api/, internal/tool/, internal/tools/,
              internal/permissions/, internal/state/, internal/mcp/,
              internal/compact/, internal/config/

internal/tools/ (各 tool)
  └── 依赖 → internal/tool/ (接口), internal/permissions/,
              internal/config/, internal/api/ (AgentTool)
              ⚠️ 禁止: tools 包之间互相依赖
              ⚠️ 禁止: tools 包依赖 tui 包

internal/permissions/
  └── 依赖 → internal/state/, internal/config/, internal/tool/
              ⚠️ 禁止: 依赖 tui、engine、tools

internal/api/
  └── 依赖 → internal/config/, internal/tool/ (类型定义)
              ⚠️ 禁止: 依赖 engine、tui、tools

internal/mcp/
  └── 依赖 → internal/tool/, internal/config/
              ⚠️ 禁止: 依赖 engine、tui

internal/state/
  └── 依赖 → internal/config/, internal/tool/ (仅接口类型)
              ⚠️ 禁止: 依赖 engine、tui、tools（防止循环）

internal/config/
  └── 依赖 → pkg/types/ (纯类型，零依赖)
              ⚠️ 禁止: 依赖任何 internal 包

pkg/types/
  └── 零依赖（纯数据类型定义）

依赖方向规则（严格单向）：
cmd → tui → engine → (api, mcp, tools, permissions)
                          ↓
                   (state, config, pkg/types)
```

### 2.3 一次完整 LLM 请求的数据流图

```
用户输入 (stdin / TUI PromptInput)
    │
    ▼
[TUI: REPL Model] ──── Msg{UserInput} ────► Update()
    │
    ▼ tea.Cmd
[engine.QueryEngine.SubmitMessage(prompt)]
    │
    ▼
[processUserInput]  ← 处理 /slash 命令、附件、内存注入
    │
    ▼
[query.QueryLoop(params)]  ← 主循环，chan SDKMessage 输出
    │
    ├── 1. 应用 ToolResultBudget（大结果截断/替换）
    ├── 2. SnipCompact（历史压缩）
    ├── 3. MicroCompact（微压缩）
    ├── 4. AutoCompact（自动对话压缩，超 token 阈值时触发）
    │
    ├── 5. fetchSystemPromptParts → 构建 SystemPrompt
    │
    ▼
[api.CallModel(messages, systemPrompt, tools, ...)]  ← SSE streaming
    │
    │  for event := range stream {
    │    switch event.Type {
    │    case "content_block_delta": 流式文字/思考块
    │    case "tool_use":           收集 ToolUseBlock
    │    case "message_stop":       结束
    │    }
    │  }
    │
    ▼
[AssistantMessage 构建完毕]
    │
    ├── stop_reason == "end_turn"? → yield terminal{reason: "end_turn"} → 结束循环
    │
    └── 有 tool_use blocks? → 进入工具执行阶段
            │
            ▼
    [toolOrchestration.RunTools(toolUseBlocks)]
            │
            ├── partitionToolCalls: 划分可并发(只读) / 必须串行(写操作) 批次
            │
            ├── 并发批次: goroutine + errgroup 并发执行
            │   └── [tool.Call(input, ctx, canUseTool)] ← 先调 permissions.CanUseTool
            │
            └── 串行批次: 顺序执行
                └── [tool.Call(input, ctx, canUseTool)]
                        │
                        ├── PermissionCheck: permissions.HasPermissionsToUseTool()
                        │     ├── allow → 直接执行
                        │     ├── ask   → 发送 PermissionRequest 到 TUI channel → 等待用户确认
                        │     └── deny  → 返回 PermissionDenial 结果
                        │
                        └── 执行工具，返回 ToolResult{data, newMessages}
            │
            ▼
    构建 tool_result UserMessage，追加到 messages
            │
            ▼
    继续下一轮 while(true) 循环 ← QueryLoop 不退出
            │
    ... (直到 stop_reason == "end_turn" 或 maxTurns 耗尽)
            │
            ▼
[QueryEngine] 收集所有 SDKMessage，累计 Usage/Cost
    │
    ▼
[TUI: REPL Model] 接收 chan Message，渲染到终端
```

---

## 3. Go 项目包结构设计

### 3.1 完整目录结构

```
claude-code-go/
├── cmd/                            # CLI 入口层（Cobra）
│   ├── main.go                     # 程序入口，Cobra root 初始化
│   ├── root.go                     # 根命令：chat 模式 (对应 main.tsx launchRepl)
│   ├── chat.go                     # `claude` 交互式 REPL
│   ├── print.go                    # `claude -p` 非交互模式（headless/SDK）
│   ├── api.go                      # `claude api` SDK 服务模式
│   ├── mcp.go                      # `claude mcp` MCP 管理命令
│   ├── doctor.go                   # `claude doctor` 诊断命令
│   ├── config.go                   # `claude config` 配置命令
│   ├── resume.go                   # `claude --resume` 恢复会话
│   └── update.go                   # `claude update` 自动更新
│
├── internal/
│   ├── engine/                     # 核心查询引擎（对应 QueryEngine.ts + query.ts）
│   │   ├── queryengine.go          # QueryEngine 结构体：会话级状态管理
│   │   ├── query.go                # queryLoop：主循环 goroutine + channel
│   │   ├── query_config.go         # buildQueryConfig：配置快照（对应 query/config.ts）
│   │   ├── token_budget.go         # Token 预算跟踪（对应 query/tokenBudget.ts）
│   │   ├── stop_hooks.go           # Stop hook 执行（对应 query/stopHooks.ts）
│   │   └── transitions.go          # Terminal/Continue 类型（对应 query/transitions.ts）
│   │
│   ├── tool/                       # Tool 接口定义（对应 Tool.ts）
│   │   ├── tool.go                 # Tool 接口、ToolUseContext、ToolResult 类型
│   │   ├── builder.go              # buildTool 工厂函数（填充 TOOL_DEFAULTS）
│   │   ├── types.go                # ToolProgress、ValidationResult 等公共类型
│   │   └── registry.go             # 全局 tool 注册表（替代 tools.ts getTools()）
│   │
│   ├── tools/                      # 各 Tool 实现（对应 src/tools/*）
│   │   ├── bash/                   # BashTool
│   │   │   ├── bash.go             # Tool 实现
│   │   │   ├── permissions.go      # bash 权限检查（bashPermissions.ts）
│   │   │   ├── security.go         # 危险命令检测（bashSecurity.ts）
│   │   │   ├── sandbox.go          # 沙箱判断（shouldUseSandbox.ts）
│   │   │   └── prompt.go           # Tool 描述/系统提示
│   │   │
│   │   ├── fileedit/               # FileEditTool
│   │   │   ├── fileedit.go
│   │   │   ├── utils.go
│   │   │   └── prompt.go
│   │   │
│   │   ├── fileread/               # FileReadTool
│   │   │   ├── fileread.go
│   │   │   ├── limits.go           # 文件大小/token 限制
│   │   │   └── prompt.go
│   │   │
│   │   ├── filewrite/              # FileWriteTool
│   │   │   └── filewrite.go
│   │   │
│   │   ├── glob/                   # GlobTool
│   │   │   └── glob.go
│   │   │
│   │   ├── grep/                   # GrepTool
│   │   │   └── grep.go
│   │   │
│   │   ├── agent/                  # AgentTool（子 Agent 调度）
│   │   │   ├── agent.go            # Tool 实现
│   │   │   ├── runagent.go         # runAgent 核心逻辑（runAgent.ts）
│   │   │   ├── forksubagent.go     # fork 子 agent（forkSubagent.ts）
│   │   │   ├── loadadents.go       # agent 定义加载（loadAgentsDir.ts）
│   │   │   ├── agentmemory.go      # agent 内存快照
│   │   │   └── prompt.go
│   │   │
│   │   ├── mcp/                    # MCPTool（调用 MCP 服务端工具）
│   │   │   ├── mcptool.go
│   │   │   └── mcpauth.go          # McpAuthTool
│   │   │
│   │   ├── websearch/              # WebSearchTool
│   │   │   └── websearch.go
│   │   │
│   │   ├── webfetch/               # WebFetchTool
│   │   │   └── webfetch.go
│   │   │
│   │   ├── todo/                   # TodoWriteTool
│   │   │   └── todo.go
│   │   │
│   │   ├── tasks/                  # Task 相关工具
│   │   │   ├── create.go           # TaskCreateTool
│   │   │   ├── get.go              # TaskGetTool
│   │   │   ├── list.go             # TaskListTool
│   │   │   ├── update.go           # TaskUpdateTool
│   │   │   ├── stop.go             # TaskStopTool
│   │   │   └── output.go           # TaskOutputTool
│   │   │
│   │   ├── skill/                  # SkillTool
│   │   │   └── skill.go
│   │   │
│   │   ├── toolsearch/             # ToolSearchTool
│   │   │   └── toolsearch.go
│   │   │
│   │   ├── notebook/               # NotebookEditTool
│   │   │   └── notebook.go
│   │   │
│   │   ├── lsp/                    # LSPTool
│   │   │   └── lsp.go
│   │   │
│   │   ├── planmode/               # EnterPlanModeTool / ExitPlanModeTool
│   │   │   ├── enter.go
│   │   │   └── exit.go
│   │   │
│   │   ├── worktree/               # EnterWorktreeTool / ExitWorktreeTool
│   │   │   ├── enter.go
│   │   │   └── exit.go
│   │   │
│   │   ├── sendmessage/            # SendMessageTool（swarm 内部通信）
│   │   │   └── sendmessage.go
│   │   │
│   │   ├── team/                   # TeamCreateTool / TeamDeleteTool
│   │   │   ├── create.go
│   │   │   └── delete.go
│   │   │
│   │   ├── askuser/                # AskUserQuestionTool
│   │   │   └── askuser.go
│   │   │
│   │   ├── sleep/                  # SleepTool
│   │   │   └── sleep.go
│   │   │
│   │   ├── repl/                   # REPLTool
│   │   │   └── repltool.go
│   │   │
│   │   ├── brief/                  # BriefTool
│   │   │   └── brief.go
│   │   │
│   │   ├── cron/                   # ScheduleCronTool
│   │   │   └── cron.go
│   │   │
│   │   ├── mcpresources/           # ListMcpResourcesTool / ReadMcpResourceTool
│   │   │   ├── list.go
│   │   │   └── read.go
│   │   │
│   │   └── syntheticoutput/        # SyntheticOutputTool（结构化输出）
│   │       └── syntheticoutput.go
│   │
│   ├── api/                        # Anthropic API 客户端（对应 services/api/）
│   │   ├── client.go               # HTTP 客户端，SSE 流式调用
│   │   ├── claude.go               # callModel：构建请求、处理响应流
│   │   ├── errors.go               # API 错误类型、重试逻辑
│   │   ├── retry.go                # withRetry、FallbackTriggeredError
│   │   ├── logging.go              # 请求/响应日志，Usage 统计
│   │   ├── usage.go                # accumulateUsage, updateUsage
│   │   └── streaming.go            # SSE 流解析，StreamEvent 类型
│   │
│   ├── mcp/                        # MCP 客户端（对应 services/mcp/）
│   │   ├── client.go               # MCP 客户端连接管理（connectToServer, fetchToolsForClient）
│   │   ├── types.go                # McpServerConfig, MCPServerConnection, Transport 类型
│   │   ├── config.go               # MCP 配置读取（getMcpConfigByName）
│   │   ├── normalization.go        # 工具名规范化
│   │   ├── auth.go                 # OAuth 认证处理（elicitationHandler.ts）
│   │   └── manager.go              # MCPConnectionManager：连接池管理
│   │
│   ├── permissions/                # 权限系统（对应 utils/permissions/）
│   │   ├── permissions.go          # hasPermissionsToUseTool 主逻辑
│   │   ├── rules.go                # PermissionRule 解析与匹配
│   │   ├── mode.go                 # PermissionMode 枚举与转换
│   │   ├── result.go               # PermissionResult、PermissionDecision 类型
│   │   ├── filesystem.go           # 文件路径权限校验
│   │   ├── shell.go                # shell 规则匹配（shellRuleMatching.ts）
│   │   ├── denial.go               # 拒绝跟踪（denialTracking.ts）
│   │   ├── patterns.go             # 危险模式检测
│   │   └── setup.go                # 初始权限上下文构建
│   │
│   ├── state/                      # 应用状态（对应 state/AppState.ts）
│   │   ├── appstate.go             # AppState 结构体定义
│   │   ├── store.go                # Store：sync.RWMutex + 订阅广播
│   │   ├── defaults.go             # getDefaultAppState()
│   │   └── selectors.go            # 状态选择器函数
│   │
│   ├── config/                     # 配置系统（对应 utils/config.ts）
│   │   ├── global.go               # GlobalConfig 读写（~/.claude/settings.json）
│   │   ├── project.go              # ProjectConfig 读写（.claude/settings.json）
│   │   ├── local.go                # LocalConfig（.claude/settings.local.json）
│   │   ├── types.go                # GlobalConfig, ProjectConfig 类型定义
│   │   ├── lockfile.go             # 文件锁（lockfile.ts）
│   │   └── env.go                  # 环境变量展开（envUtils.ts）
│   │
│   ├── compact/                    # 上下文压缩（对应 services/compact/）
│   │   ├── autocompact.go          # 自动压缩触发逻辑
│   │   ├── compact.go              # buildPostCompactMessages
│   │   ├── microcompact.go         # 微压缩（apiMicrocompact.ts）
│   │   └── snip.go                 # Snip 压缩（snipCompact.ts）
│   │
│   ├── coordinator/                # 多 Agent 协调（对应 coordinator/）
│   │   ├── coordinator.go          # coordinator mode 判断与上下文注入
│   │   └── context.go              # getCoordinatorUserContext
│   │
│   ├── tui/                        # TUI 层（对应 screens/ + components/，用 BubbleTea 实现）
│   │   ├── repl/
│   │   │   ├── model.go            # REPL BubbleTea Model 结构体
│   │   │   ├── update.go           # REPL Update(msg) 消息处理
│   │   │   ├── view.go             # REPL View() 渲染
│   │   │   └── init.go             # REPL Init() 初始化 Cmd
│   │   │
│   │   ├── components/
│   │   │   ├── message/            # 消息渲染
│   │   │   │   ├── assistant.go    # 助手消息渲染
│   │   │   │   ├── user.go         # 用户消息渲染
│   │   │   │   └── tooluse.go      # ToolUse / ToolResult 渲染
│   │   │   ├── prompt/             # PromptInput 组件
│   │   │   │   ├── input.go
│   │   │   │   └── queue.go        # 排队命令显示
│   │   │   ├── spinner/            # 加载动画（SpinnerWithVerb）
│   │   │   │   └── spinner.go
│   │   │   ├── permission/         # 权限请求对话框
│   │   │   │   └── dialog.go
│   │   │   └── statusbar/          # 底部状态栏
│   │   │       └── statusbar.go
│   │   │
│   │   └── styles/                 # Lip Gloss 样式定义
│   │       ├── theme.go            # 主题颜色
│   │       └── layout.go           # 布局常量
│   │
│   ├── commands/                   # /slash 命令系统（对应 commands/ + commands.ts）
│   │   ├── registry.go             # 命令注册与查找
│   │   ├── types.go                # Command 接口定义（对应 types/command.ts）
│   │   ├── handlers/               # 各命令实现
│   │   │   ├── clear.go
│   │   │   ├── compact.go
│   │   │   ├── config.go
│   │   │   ├── doctor.go
│   │   │   └── ...
│   │   └── slash.go                # /slash 命令解析
│   │
│   ├── analytics/                  # 事件分析（对应 services/analytics/）
│   │   ├── client.go               # 事件上报客户端
│   │   ├── growthbook.go           # GrowthBook 功能开关
│   │   └── events.go               # logEvent 实现
│   │
│   ├── oauth/                      # OAuth2 认证（对应 services/oauth/）
│   │   ├── flow.go                 # OAuth2 授权流程
│   │   ├── tokens.go               # Token 存储与刷新
│   │   └── keychain.go             # Keychain / 安全存储
│   │
│   ├── session/                    # 会话持久化（对应 utils/sessionStorage.ts）
│   │   ├── storage.go              # recordTranscript, flushSessionStorage
│   │   ├── resume.go               # --resume 会话恢复
│   │   └── transcript.go           # transcript 读写
│   │
│   ├── memdir/                     # 内存目录/CLAUDE.md（对应 memdir/）
│   │   ├── memdir.go               # loadMemoryPrompt
│   │   └── paths.go                # 内存路径解析
│   │
│   ├── hooks/                      # 钩子系统（对应 utils/hooks/）
│   │   ├── pretool.go              # PreToolUse hooks
│   │   ├── posttool.go             # PostToolUse hooks
│   │   ├── postsampling.go         # PostSampling hooks（executePostSamplingHooks）
│   │   ├── session.go              # Session start/stop hooks
│   │   └── types.go                # Hook 相关类型
│   │
│   └── bootstrap/                  # 全局会话状态（对应 bootstrap/state.ts）
│       ├── state.go                # getSessionId, setCwd, getOriginalCwd 等
│       └── init.go                 # 进程启动初始化
│
├── pkg/                            # 可被外部复用的纯工具包
│   ├── types/                      # 核心数据类型（对应 types/）
│   │   ├── message.go              # Message, AssistantMessage, UserMessage 等
│   │   ├── permissions.go          # PermissionMode, PermissionRule 等
│   │   ├── tools.go                # ToolProgressData 等工具进度类型
│   │   ├── ids.go                  # AgentId, SessionId 类型包装
│   │   └── hooks.go                # HookProgress, PromptRequest/Response
│   │
│   └── utils/                      # 纯工具函数（无外部依赖）
│       ├── format.go               # formatTokens, truncateToWidth
│       ├── json.go                 # safeParseJSON, jsonStringify
│       ├── path.go                 # normalizePathForConfigKey
│       ├── errors.go               # AbortError, ConfigParseError
│       └── messages.go             # createUserMessage, normalizeMessagesForAPI
│
├── go.mod
├── go.sum
└── docs/
```

### 3.2 每个包的核心职责

| 包路径 | 核心职责 | 关键类型/函数 |
|-------|---------|-------------|
| `cmd/` | Cobra CLI 命令定义，参数解析，启动入口 | `rootCmd`, `Execute()` |
| `internal/engine` | 会话生命周期管理，主循环驱动 | `QueryEngine`, `QueryLoop()` |
| `internal/tool` | Tool 接口规范，构建工厂 | `Tool` interface, `buildTool()` |
| `internal/tools/*` | 各 Tool 的具体实现 | 各 `Tool` struct 实现 |
| `internal/api` | Anthropic API SSE 流式调用，重试 | `CallModel()`, `StreamEvent` |
| `internal/mcp` | MCP 协议客户端，工具/资源管理 | `MCPClient`, `connectToServer()` |
| `internal/permissions` | 权限规则检查，ask/allow/deny 决策 | `HasPermissionsToUseTool()` |
| `internal/state` | AppState 并发安全存储与订阅 | `Store`, `GetState()`, `SetState()` |
| `internal/config` | 多级配置文件读写（全局/项目/本地） | `GetGlobalConfig()`, `SaveGlobalConfig()` |
| `internal/compact` | 对话历史压缩策略 | `AutoCompact()`, `MicroCompact()` |
| `internal/coordinator` | 多 Agent 协调上下文注入 | `GetCoordinatorUserContext()` |
| `internal/tui` | BubbleTea TUI 模型，渲染逻辑 | `ReplModel`, `Update()`, `View()` |
| `internal/commands` | /slash 命令注册与执行 | `Command` interface, `Registry` |
| `internal/hooks` | Pre/Post tool/sampling hooks 执行 | `ExecutePreToolHooks()` |
| `internal/session` | 对话 transcript 持久化，--resume | `RecordTranscript()` |
| `internal/memdir` | CLAUDE.md 内存文件加载 | `LoadMemoryPrompt()` |
| `internal/bootstrap` | 进程级全局状态（sessionId, cwd） | `GetSessionId()`, `SetCwd()` |
| `pkg/types` | 共享数据类型定义（零依赖） | `Message`, `PermissionMode` 等 |
| `pkg/utils` | 纯工具函数（无外部状态） | 格式化、JSON、路径等 |

### 3.3 依赖规则（禁止循环依赖）

```
允许的依赖方向（单向）：
cmd → internal/* → pkg/*

internal 内部规则：
- engine 可依赖：api, mcp, tools, permissions, state, config, compact, coordinator, hooks, session, memdir, bootstrap
- tools/* 可依赖：tool(接口), permissions, config, api(仅 AgentTool), bootstrap
- tools/* 之间：禁止互相依赖
- tui 可依赖：engine, state, tool, permissions, config, commands
- tui 禁止被：engine, tools, permissions, config 依赖
- permissions 可依赖：state, config, tool(类型)
- permissions 禁止依赖：engine, tui, tools, api
- api 可依赖：config, pkg/types
- api 禁止依赖：engine, tui, tools, permissions, state
- config 可依赖：pkg/types, pkg/utils
- config 禁止依赖：所有 internal 包
- state 可依赖：config, pkg/types
- state 禁止依赖：engine, tui, tools, api, permissions
```

---

## 4. 核心技术选型与设计决策

### 4.1 TUI 框架：BubbleTea + Lip Gloss

**原版**：React + Ink（React 组件渲染到终端）

**Go 版本**：BubbleTea（Elm 架构：Model-Update-View）

**映射关系**：

| React/Ink 概念 | BubbleTea 对应 |
|---------------|----------------|
| `React.Component` | `tea.Model` interface（`Init()/Update()/View()`）|
| `useState(init)` | Model struct 中的字段 |
| `useEffect` | `tea.Cmd`（副作用返回 `tea.Msg`）|
| `setState(fn)` | `Update(msg)` 返回新 Model |
| Props | Model 字段直接访问（无 prop-drilling）|
| `React.Context` | Model 顶层字段下传，或共享 `*state.Store` 指针 |
| `useSyncExternalStore(store)` | `tea.Program.Send(msg)` 从外部推送消息 |
| `render()` | `View() string`（Lip Gloss 构建字符串）|
| `ink.Box` / `ink.Text` | `lipgloss.NewStyle()` + 字符串拼接 |
| `useInput(handler)` | BubbleTea 自动路由 `tea.KeyMsg` 到 `Update()` |

**REPL Model 核心结构**：
```go
// internal/tui/repl/model.go
type Model struct {
    // 对应 AppState（共享指针，只读访问）
    state       *state.Store
    // 对话消息列表
    messages    []types.Message
    // 当前输入框内容
    input       string
    // 权限确认队列（对应 toolUseConfirmQueue）
    confirmQueue []permissions.ConfirmRequest
    // 进行中的 toolUse ID 集合
    inProgressToolUseIDs map[string]bool
    // 加载动画状态
    spinner     spinner.Model
    // viewport 滚动（对应 VirtualMessageList）
    viewport    viewport.Model
    // 终端尺寸
    width, height int
}

type Msg interface{ isTuiMsg() }

// 来自 QueryEngine 的新消息
type NewSDKMessageMsg struct { Msg types.SDKMessage }
// 权限请求
type PermissionRequestMsg struct { Req permissions.ConfirmRequest }
// 用户输入提交
type SubmitInputMsg struct { Text string }
```

### 4.2 CLI 框架：Cobra

**原版**：`@commander-js/extra-typings`（Commander.js）

**Go 版本**：`github.com/spf13/cobra`

```go
// cmd/root.go
var rootCmd = &cobra.Command{
    Use:   "claude",
    Short: "Claude Code - AI-powered coding assistant",
    RunE:  runInteractive,  // 默认运行交互式 REPL
}

func init() {
    rootCmd.PersistentFlags().StringP("model", "m", "", "Specify model")
    rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
    rootCmd.PersistentFlags().StringP("permission-mode", "", "default", "Permission mode")
    rootCmd.PersistentFlags().IntP("max-turns", "", 0, "Max turns (0=unlimited)")
    rootCmd.AddCommand(printCmd, apiCmd, mcpCmd, doctorCmd, configCmd)
}
```

### 4.3 并发模型：goroutine + channel

**原版**：TypeScript `async function*`（AsyncGenerator）+ `await`

**Go 版本**：goroutine + channel（模拟 AsyncGenerator 语义）

**核心模式**：

```go
// AsyncGenerator<SDKMessage> → chan types.SDKMessage
// internal/engine/query.go

func queryLoop(ctx context.Context, params QueryParams) <-chan QueryYield {
    ch := make(chan QueryYield, 16)
    go func() {
        defer close(ch)
        // ... 循环逻辑
        for {
            // 调用 API
            for event := range api.CallModel(ctx, apiParams) {
                ch <- QueryYield{Event: event}
            }
            // 执行工具
            for update := range toolOrchestration.RunTools(ctx, toolUseBlocks, ...) {
                ch <- QueryYield{ToolResult: update}
            }
            // 退出判断
            if terminal {
                ch <- QueryYield{Terminal: &terminalResult}
                return
            }
        }
    }()
    return ch
}

// 工具并发执行（对应 runToolsConcurrently）
func runToolsConcurrently(ctx context.Context, tools []ToolUseBlock, ...) <-chan MessageUpdate {
    ch := make(chan MessageUpdate, len(tools))
    var wg sync.WaitGroup
    for _, toolUse := range tools {
        wg.Add(1)
        go func(t ToolUseBlock) {
            defer wg.Done()
            for update := range runToolUse(ctx, t, ...) {
                ch <- update
            }
        }(toolUse)
    }
    go func() { wg.Wait(); close(ch) }()
    return ch
}
```

**AbortController 对应**：
```go
// TS: abortController.signal → Go: context.Context
// TS: abortController.abort() → Go: cancel()
ctx, cancel := context.WithCancel(parentCtx)
defer cancel()
// 传递给所有下游调用
```

### 4.4 错误处理策略

**原版**：TypeScript `try/catch` + 自定义 Error 类型（`AbortError`, `ConfigParseError`, `FallbackTriggeredError`）

**Go 版本**：惯用 `error` 接口 + sentinel errors + `errors.As`

```go
// pkg/utils/errors.go
type AbortError struct{ Msg string }
func (e *AbortError) Error() string { return e.Msg }

type ConfigParseError struct{ Path string; Cause error }
func (e *ConfigParseError) Error() string { return fmt.Sprintf("config parse error at %s: %v", e.Path, e.Cause) }
func (e *ConfigParseError) Unwrap() error { return e.Cause }

// 使用
var abortErr *AbortError
if errors.As(err, &abortErr) {
    // 用户取消，不报错
}
```

**查询循环错误处理**：原版在 `query.ts` 有多个 `try/catch` 块处理不同类型错误（prompt_too_long, max_output_tokens, abort, retryable API errors）。Go 版本用显式 `switch err := ...; err != nil {}` + 哨兵错误类型替代。

### 4.5 配置系统设计

原版 `utils/config.ts` 实现了多级配置合并（全局/项目/本地），并有文件锁和 watch 机制。

**Go 版本设计**：

```go
// internal/config/types.go
type GlobalConfig struct {
    UserID               string            `json:"userID,omitempty"`
    Theme                string            `json:"theme"`
    Verbose              bool              `json:"verbose"`
    AutoCompactEnabled   bool              `json:"autoCompactEnabled"`
    NumStartups          int               `json:"numStartups"`
    McpServers           map[string]McpServerConfig `json:"mcpServers,omitempty"`
    PrimaryApiKey        string            `json:"primaryApiKey,omitempty"`
    // ... 完整映射 config.ts GlobalConfig
}

type ProjectConfig struct {
    AllowedTools         []string          `json:"allowedTools"`
    McpServers           map[string]McpServerConfig `json:"mcpServers,omitempty"`
    HasTrustDialogAccepted bool            `json:"hasTrustDialogAccepted,omitempty"`
    // ...
}

// internal/config/global.go
type ConfigManager struct {
    mu         sync.RWMutex
    globalPath string
    cache      *GlobalConfig
    // 文件监听（对应 watchFile/unwatchFile）
    watcher    *fsnotify.Watcher
}

func (m *ConfigManager) Get() (*GlobalConfig, error)
func (m *ConfigManager) Save(cfg *GlobalConfig) error  // 使用文件锁
```

**配置层级**（与 TS 版本完全一致）：
1. `~/.claude/settings.json` - 用户全局配置
2. `<project>/.claude/settings.json` - 项目配置
3. `<project>/.claude/settings.local.json` - 本地配置（不提交 git）
4. `~/.claude/policy.json` - 企业策略配置
5. 环境变量覆盖（`ANTHROPIC_API_KEY`, `CLAUDE_*` 等）

### 4.6 权限系统设计

原版权限系统（`utils/permissions/permissions.ts`）核心是 `hasPermissionsToUseTool`，支持 ask/allow/deny 规则，以及 `PermissionMode`（default, acceptEdits, bypassPermissions, dontAsk, plan, auto）。

**Go 版本设计**：

```go
// pkg/types/permissions.go
type PermissionMode string
const (
    PermissionModeDefault           PermissionMode = "default"
    PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
    PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
    PermissionModeDontAsk           PermissionMode = "dontAsk"
    PermissionModePlan              PermissionMode = "plan"
    PermissionModeAuto              PermissionMode = "auto"
)

type PermissionBehavior string
const (
    BehaviorAllow PermissionBehavior = "allow"
    BehaviorDeny  PermissionBehavior = "deny"
    BehaviorAsk   PermissionBehavior = "ask"
)

// internal/permissions/permissions.go
type PermissionResult struct {
    Behavior     PermissionBehavior
    UpdatedInput map[string]any
    DecisionReason *DecisionReason
}

// 主权限检查函数（对应 hasPermissionsToUseTool）
func HasPermissionsToUseTool(
    tool tool.Tool,
    input map[string]any,
    ctx *ToolUseContext,
    assistantMsg *types.AssistantMessage,
    toolUseID string,
) (PermissionResult, error)
```

**权限检查顺序**（与原版一致）：
1. 工具的 `checkPermissions(input, ctx)` 方法（工具特定逻辑）
2. `alwaysDenyRules` 检查
3. `alwaysAllowRules` 检查
4. `alwaysAskRules` 检查
5. 模式决定：`bypassPermissions` → 全允许；`plan` → 只允许读操作；`acceptEdits` → 允许文件编辑；`default` → 写操作 ask
6. PreToolUse hooks（外部 hook 脚本）

### 4.7 状态管理设计

原版使用 Zustand-like `createStore`（`state/store.ts`），底层是 `useSyncExternalStore`，AppState 是 `DeepImmutable` 的纯值对象，通过 `setState(fn)` 函数式更新。

**Go 版本**：

```go
// internal/state/store.go
type Store struct {
    mu       sync.RWMutex
    state    AppState
    subs     []chan struct{} // 订阅者通知 channel
    onChange func(oldState, newState AppState)
}

func NewStore(initial AppState, onChange func(old, new AppState)) *Store

// 对应 TS: getState()
func (s *Store) GetState() AppState {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.state
}

// 对应 TS: setState(fn: (prev) => next)
func (s *Store) SetState(fn func(AppState) AppState) {
    s.mu.Lock()
    old := s.state
    s.state = fn(s.state)
    new := s.state
    s.mu.Unlock()
    if old != new { // 浅比较（struct 值比较）
        s.onChange(old, new)
        s.notifySubscribers()
    }
}

// BubbleTea 外部事件注入（对应 useSyncExternalStore）
func (s *Store) Subscribe() (<-chan struct{}, func()) {
    ch := make(chan struct{}, 1)
    s.mu.Lock()
    s.subs = append(s.subs, ch)
    s.mu.Unlock()
    return ch, func() { s.unsubscribe(ch) }
}
```

**AppState 结构体**（Go 中使用值类型，SetState 产生副本，天然不可变）：
```go
// internal/state/appstate.go
type AppState struct {
    Settings            config.SettingsJson
    Verbose             bool
    MainLoopModel       ModelSetting
    ToolPermissionContext ToolPermissionContext
    Mcp                 McpState
    Tasks               map[string]TaskState  // 用 sync.Map 或 map+lock
    Messages            []types.Message       // 对话消息（REPL 持有）
    // ... 完整映射 AppState 所有字段
}
```

---

## 5. 关键数据流详解

### 5.1 LLM Query Loop 流程

```
QueryEngine.SubmitMessage(prompt)
│
├─ 1. processUserInput(input, context)
│     ├─ 识别 /slash 命令 → 直接执行（不发 API）
│     ├─ 处理文件附件、内存注入
│     └─ 返回 {messages, shouldQuery, model}
│
├─ 2. 构建 SystemPrompt
│     └─ fetchSystemPromptParts(tools, model, mcpClients)
│           ├─ 工具 prompt: tool.Prompt(options)
│           ├─ 用户上下文: getUserContext()
│           └─ 系统上下文: getSystemContext()
│
└─ 3. query.QueryLoop(params) → <-chan QueryYield
      │
      while true:
      │  ├─ applyToolResultBudget     // 大结果替换
      │  ├─ snipCompact               // 历史截断
      │  ├─ microCompact              // 微压缩
      │  ├─ autoCompact               // 超 token 时触发完整压缩
      │  │
      │  ├─ api.CallModel(...)  ← SSE 流
      │  │    ├─ yield: text_delta, thinking_delta
      │  │    ├─ yield: tool_use 块（开始流式收集）
      │  │    └─ yield: message_stop（stop_reason）
      │  │
      │  ├─ 构建 AssistantMessage（含所有 ContentBlock）
      │  │
      │  ├─ stop_reason == "end_turn" → return Terminal{reason}
      │  │
      │  └─ 有 tool_use →
      │         toolOrchestration.RunTools(toolUseBlocks)
      │              ├─ partitionToolCalls → 批次划分
      │              ├─ 并发批次: goroutine pool
      │              └─ 串行批次: 顺序执行
      │                   └─ 每个 tool:
      │                        ├─ canUseTool(tool, input, ...) → 权限检查
      │                        ├─ tool.Call(input, ctx, canUseTool)
      │                        └─ yield: UserMessage(tool_result)
      │         继续下一轮 while true
```

### 5.2 Tool 执行流程

```
toolOrchestration.RunTools(blocks, assistantMsgs, canUseTool, ctx)
│
├─ partitionToolCalls(blocks, ctx)
│     每个 block:
│       tool = findToolByName(ctx.options.tools, block.name)
│       input = tool.inputSchema.Parse(block.input)
│       isSafe = tool.IsConcurrencySafe(input)
│     → []Batch{{isConcurrencySafe, blocks}}
│
├─ for batch in batches:
│     if batch.isConcurrencySafe:
│       runToolsConcurrently(batch.blocks, ...) // goroutine fan-out
│     else:
│       runToolsSerially(batch.blocks, ...)
│
└─ runToolUse(toolUse, assistantMsg, canUseTool, ctx):
      ├─ 1. 找到 tool
      ├─ 2. tool.ValidateInput(input, ctx) → ValidationResult
      │      失败 → yield UserMessage(tool_result: error)
      ├─ 3. canUseTool(tool, input, ctx, assistantMsg, toolUseID)
      │      ├─ hasPermissionsToUseTool() → allow/deny/ask
      │      ├─ ask → 发送 PermissionRequest 到 TUI
      │      │        等待用户 allow/deny/always-allow
      │      └─ deny → yield UserMessage(tool_result: denied)
      ├─ 4. tool.Call(input, ctx, canUseTool, parentMsg, onProgress)
      │      ├─ 执行实际操作
      │      ├─ onProgress(progress) → yield ProgressMessage（UI 动画）
      │      └─ 返回 ToolResult{data, newMessages, contextModifier}
      └─ 5. 构建 UserMessage(tool_result_block)
             yield MessageUpdate{msg}
```

### 5.3 权限检查流程

```
canUseTool(tool, input, ctx, assistantMsg, toolUseID, forceDecision?)
│
├─ forceDecision != nil → 直接使用（speculative execution）
│
├─ hasPermissionsToUseTool(tool, input, ctx, ...)
│     ├─ 1. tool.CheckPermissions(input, ctx) → PermissionResult
│     │      （工具特定逻辑，如 BashTool 的只读命令检测）
│     ├─ 2. 检查 alwaysDenyRules → {behavior: "deny"}
│     ├─ 3. 检查 alwaysAllowRules → {behavior: "allow"}
│     ├─ 4. 检查 alwaysAskRules → {behavior: "ask"}
│     └─ 5. 根据 PermissionMode 决定
│            - bypassPermissions → allow
│            - plan → isReadOnly? allow : ask
│            - acceptEdits → isFileEdit? allow : ask
│            - dontAsk → allow
│            - auto → classifierCheck → allow/deny/ask
│            - default → isReadOnly? allow : ask
│
├─ result.behavior == "allow" → resolve(allow)
├─ result.behavior == "deny"  → resolve(deny)
└─ result.behavior == "ask"   →
      │  非交互模式: resolve(deny)
      │  交互模式:
      │    发送 PermissionRequestMsg 到 TUI channel
      │    等待用户确认（TUI 弹出 PermissionRequest 对话框）
      └─ 用户 allow/always-allow/deny → resolve(decision)
               如果 always-allow → 更新 alwaysAllowRules
               如果 always-deny  → 更新 alwaysDenyRules
```

### 5.4 多 Agent 协调流程

```
AgentTool.Call(input{prompt, tools, model, ...}, ctx)
│
├─ 确定子 Agent 工具集（过滤 ASYNC_AGENT_ALLOWED_TOOLS）
│
├─ createSubagentContext(ctx, agentId, ...)
│     ├─ 隔离 AppState（子 Agent 的 setAppState 写到独立 store）
│     ├─ 继承父 Agent 的 readFileState（缓存复用）
│     └─ agentId = newAgentId()
│
├─ runAgent(agentId, prompt, tools, mcpClients, ctx)
│     ├─ setAgentTranscriptSubdir(agentId) → 独立 transcript 目录
│     ├─ 构建子 Agent QueryEngine（同父，但独立配置）
│     ├─ engine.SubmitMessage(prompt)
│     │     └─ query.QueryLoop → <-chan QueryYield
│     │           ├─ 流式处理：yield SDKMessage 给 AgentTool
│     │           └─ 工具调用：递归调用子工具
│     │
│     └─ 将 AgentTool 输出（progress + final result）
│         发送给父 Agent 的 onProgress callback
│
└─ 返回 ToolResult{data: agentOutput}
   父 Agent 将结果追加为 tool_result UserMessage
   继续父循环

Coordinator 模式（CLAUDE_CODE_COORDINATOR_MODE=1）：
  主 Agent 通过 AgentTool 调度多个 Worker Agent
  Worker Agent 通过 SendMessageTool 向协调器报告结果
  协调器通过 TeamCreateTool/TeamDeleteTool 管理 Worker 生命周期
```

---

## 6. 外部依赖清单

| Go 库 | 推荐版本 | 对应原 TS 依赖 | 用途 |
|------|---------|-------------|------|
| `github.com/spf13/cobra` | v1.9.x | `@commander-js/extra-typings` | CLI 框架 |
| `github.com/charmbracelet/bubbletea` | v1.3.x | `react` + `ink` | TUI 框架（Elm 架构）|
| `github.com/charmbracelet/lipgloss` | v1.1.x | `ink` 样式 | 终端样式/颜色 |
| `github.com/charmbracelet/bubbles` | v0.20.x | ink 内置组件 | viewport, spinner, textinput 组件 |
| `github.com/anthropics/anthropic-sdk-go` | v0.x | `@anthropic-ai/sdk` | Anthropic API 客户端（官方 SDK）|
| `github.com/mark3labs/mcp-go` | latest | `@modelcontextprotocol/sdk` | MCP 协议客户端（stdio/SSE/HTTP）|
| `golang.org/x/oauth2` | latest | `各 oauth 包` | OAuth2 授权流程 |
| `github.com/golang-jwt/jwt/v5` | v5.x | `jsonwebtoken` | JWT 解析 |
| `github.com/fsnotify/fsnotify` | v1.8.x | `fs.watchFile` | 配置文件热重载 |
| `github.com/BurntSushi/toml` | — | — | 配置（若需要 TOML 格式）|
| `encoding/json` | stdlib | `JSON.parse/stringify` | JSON 序列化（标准库）|
| `go.opentelemetry.io/otel` | v1.x | `opentelemetry-sdk` | 链路追踪（Perfetto）|
| `google.golang.org/protobuf` | v1.x | `protobufjs` | Protobuf 序列化（事件日志）|
| `github.com/go-playground/validator/v10` | v10.x | `zod/v4` | 输入校验（替代 Zod 运行时校验）|
| `github.com/google/uuid` | v1.x | `crypto.randomUUID` | UUID 生成 |
| `github.com/atotto/clipboard` | v0.1.x | — | 剪贴板访问 |
| `github.com/zalando/go-keyring` | v0.2.x | macOS Keychain API | 安全密钥存储 |
| `golang.org/x/sys` | latest | Node `os` 模块 | 跨平台系统调用 |
| `github.com/mattn/go-isatty` | v0.0.x | `process.stdout.isTTY` | TTY 检测 |
| `github.com/creack/pty` | v1.1.x | `node-pty` | 伪终端（Bash 交互）|
| `mvdan.cc/sh/v3` | v3.x | `shell-quote` | Shell 命令解析 |
| `github.com/sabhiram/go-gitignore` | latest | — | .gitignore 解析 |
| `golang.org/x/net` | latest | — | HTTP/2, WebSocket |
| `nhooyr.io/websocket` | v1.8.x | `ws` | WebSocket 客户端 |
| `github.com/tidwall/gjson` | v1.x | `lodash-es` | 快速 JSON 路径查询 |
| `github.com/stretchr/testify` | v1.x | `bun:test` | 测试断言 |
| `go.uber.org/zap` | v1.x | 自定义 log.ts | 结构化日志 |

---

## 7. 模块划分与开发 Agent 索引

根据代码规模、依赖关系和功能内聚性，划分为以下开发 Agent：

### Agent 0：架构基础层（Tech Lead 先行）
**职责**：基础类型定义、接口规范、项目骨架
**负责包**：`pkg/types/`, `pkg/utils/`, `internal/tool/`（接口），`go.mod`
**关键文件**：
- `pkg/types/message.go` — Message 类型体系（约 400 行）
- `pkg/types/permissions.go` — 权限类型（约 200 行）
- `internal/tool/tool.go` — Tool 接口（约 350 行，完整映射 TS Tool 接口）
- `internal/tool/builder.go` — buildTool 工厂（约 100 行）
**估算代码量**：~1500 行 | 复杂度：⭐⭐⭐（奠定基础，需精确）

### Agent 1：配置与状态系统
**职责**：多级配置、AppState Store、Bootstrap 全局状态
**负责包**：`internal/config/`, `internal/state/`, `internal/bootstrap/`
**关键文件**：
- `internal/config/global.go` — GlobalConfig 读写（约 400 行）
- `internal/config/project.go` — ProjectConfig（约 300 行）
- `internal/state/store.go` — 并发安全 Store（约 200 行）
- `internal/state/appstate.go` — AppState 定义（约 300 行）
- `internal/bootstrap/state.go` — 进程级状态（约 200 行）
**估算代码量**：~2000 行 | 复杂度：⭐⭐⭐ (配置层级复杂)
**依赖**：Agent 0（pkg/types）

### Agent 2：API 客户端与压缩服务
**职责**：Anthropic API SSE 流调用、重试、压缩策略
**负责包**：`internal/api/`, `internal/compact/`
**关键文件**：
- `internal/api/claude.go` — callModel，请求构建，响应流处理（约 600 行）
- `internal/api/retry.go` — 指数退避重试，FallbackTriggeredError（约 200 行）
- `internal/api/streaming.go` — SSE 事件解析（约 300 行）
- `internal/compact/autocompact.go` — 自动压缩触发（约 300 行）
- `internal/compact/microcompact.go` — 微压缩（约 200 行）
**估算代码量**：~2500 行 | 复杂度：⭐⭐⭐⭐ (SSE 流复杂，压缩策略多)
**依赖**：Agent 0, Agent 1

### Agent 3：MCP 客户端
**职责**：MCP 协议实现，连接管理，工具/资源发现
**负责包**：`internal/mcp/`
**关键文件**：
- `internal/mcp/client.go` — connectToServer（stdio/SSE/HTTP），fetchToolsForClient（约 500 行）
- `internal/mcp/manager.go` — MCPConnectionManager（约 300 行）
- `internal/mcp/types.go` — McpServerConfig 完整类型（约 200 行）
- `internal/mcp/auth.go` — OAuth2 + elicitation 处理（约 300 行）
**估算代码量**：~2000 行 | 复杂度：⭐⭐⭐⭐ (多传输协议、OAuth)
**依赖**：Agent 0, Agent 1

### Agent 4：权限系统
**职责**：完整权限规则引擎，canUseTool 实现
**负责包**：`internal/permissions/`
**关键文件**：
- `internal/permissions/permissions.go` — hasPermissionsToUseTool 主逻辑（约 400 行）
- `internal/permissions/rules.go` — 规则解析与 glob 匹配（约 300 行）
- `internal/permissions/shell.go` — shell 规则匹配（约 200 行）
- `internal/permissions/filesystem.go` — 路径权限（约 150 行）
- `internal/permissions/denial.go` — 拒绝跟踪（约 100 行）
**估算代码量**：~1800 行 | 复杂度：⭐⭐⭐⭐ (规则匹配逻辑复杂)
**依赖**：Agent 0, Agent 1

### Agent 5：核心查询引擎
**职责**：QueryEngine、queryLoop、toolOrchestration
**负责包**：`internal/engine/`
**关键文件**：
- `internal/engine/queryengine.go` — QueryEngine（约 500 行）
- `internal/engine/query.go` — queryLoop（约 700 行，最复杂）
- `internal/engine/tool_orchestration.go` — runTools 并发/串行（约 300 行）
- `internal/engine/tool_execution.go` — runToolUse（约 400 行）
**估算代码量**：~2500 行 | 复杂度：⭐⭐⭐⭐⭐ (核心最复杂)
**依赖**：Agent 0-4（几乎全部）

### Agent 6：核心工具实现（文件系统工具）
**职责**：Bash, FileEdit, FileRead, FileWrite, Glob, Grep
**负责包**：`internal/tools/bash/`, `fileedit/`, `fileread/`, `filewrite/`, `glob/`, `grep/`
**估算代码量**：~3000 行 | 复杂度：⭐⭐⭐⭐ (Bash 权限检查复杂)
**依赖**：Agent 0, Agent 4, Agent 5（Tool 接口）

### Agent 7：Agent 工具与协调器
**职责**：AgentTool（子 Agent 调度），coordinator 模式
**负责包**：`internal/tools/agent/`, `internal/coordinator/`
**估算代码量**：~2000 行 | 复杂度：⭐⭐⭐⭐⭐ (递归调用，状态隔离)
**依赖**：Agent 0-5

### Agent 8：其余内置工具
**职责**：WebSearch, WebFetch, Todo, Tasks, Skill, ToolSearch, Notebook, LSP, PlanMode, Worktree, SendMessage, Team, AskUser, Sleep, Cron, MCPResources, SyntheticOutput
**负责包**：`internal/tools/` 下其余子包
**估算代码量**：~4000 行 | 复杂度：⭐⭐⭐
**依赖**：Agent 0, Agent 5

### Agent 9：TUI 层
**职责**：BubbleTea REPL，所有 TUI 组件，样式
**负责包**：`internal/tui/`
**关键文件**：
- `internal/tui/repl/model.go` — REPL Model（约 400 行）
- `internal/tui/repl/update.go` — Update 消息处理（约 600 行）
- `internal/tui/repl/view.go` — View 渲染（约 500 行）
- `internal/tui/components/` — 各组件（约 2000 行合计）
**估算代码量**：~4000 行 | 复杂度：⭐⭐⭐⭐ (React→BubbleTea 思维转换)
**依赖**：Agent 0-5（通过 channel 接收事件）

### Agent 10：CLI 入口、Hooks、Session、Commands
**职责**：cmd/ 层 Cobra 命令，/slash 命令系统，hooks，session 持久化
**负责包**：`cmd/`, `internal/commands/`, `internal/hooks/`, `internal/session/`
**估算代码量**：~3000 行 | 复杂度：⭐⭐⭐
**依赖**：Agent 0-9（集成层）

### 开发顺序建议（依赖优先）

```
第 1 阶段（并行）：Agent 0（基础）
第 2 阶段（并行）：Agent 1（配置/状态）、Agent 2（API）、Agent 3（MCP）
第 3 阶段（并行）：Agent 4（权限）
第 4 阶段：Agent 5（查询引擎）← 依赖全部
第 5 阶段（并行）：Agent 6（文件工具）、Agent 7（Agent 工具）、Agent 8（其余工具）
第 6 阶段（并行）：Agent 9（TUI）、Agent 10（CLI + Hooks + Session）
```

---

## 8. 关键挑战与解决方案

### 挑战 1：AsyncGenerator → goroutine + channel 的语义映射

**问题**：TypeScript `async function*` 是惰性拉取（lazy pull），调用方 `for await (const msg of gen) {}` 控制消费速度。Go channel 是推送模型（eager push），生产者不等消费者确认就继续推。

**原版依赖**：`query.ts` 的 `queryLoop` 是一个超过 1000 行的 AsyncGenerator，中间穿插大量 `yield`，每个 `yield` 点都可能被调用方暂停。

**解决方案**：
- Go 中使用带缓冲 channel（`make(chan QueryYield, 16)`）近似惰性语义
- 关键控制点（如 PermissionRequest）使用无缓冲 channel 或 `sync.WaitGroup` 等待用户响应
- 将 `for await (const x of gen)` 统一替换为 `for x := range ch`
- 对于 `return terminal`（生成器 return value），使用 `QueryYield{Terminal: &result}` 作为最后一个值发送，或单独用 `chan Terminal` 处理终止信号

### 挑战 2：Tool 接口的 React 渲染方法

**问题**：`Tool.ts` 的 `Tool` 接口包含大量 React 渲染方法（`renderToolResultMessage`, `renderToolUseMessage`, `renderGroupedToolUse` 等），返回 `React.ReactNode`。这些方法深度依赖 React 组件树。

**解决方案**：
- 将渲染方法从 `Tool` 接口中**拆分**到独立的 `ToolRenderer` 接口
- `Tool` 接口只保留 `Call`, `Description`, `CheckPermissions`, `ValidateInput` 等**逻辑方法**
- `ToolRenderer` 接口保留渲染方法，返回 `string`（Lip Gloss 构建的终端字符串），而非 React 节点
- TUI 层通过 `ToolRendererRegistry` 按 Tool Name 查找对应 Renderer

```go
// internal/tool/tool.go — 精简版接口
type Tool interface {
    Name() string
    Call(input map[string]any, ctx *ToolUseContext) (ToolResult, error)
    Description(input map[string]any, opts DescriptionOptions) (string, error)
    CheckPermissions(input map[string]any, ctx *ToolUseContext) (PermissionResult, error)
    ValidateInput(input map[string]any, ctx *ToolUseContext) ValidationResult
    IsConcurrencySafe(input map[string]any) bool
    IsReadOnly(input map[string]any) bool
    Prompt(opts PromptOptions) (string, error)
    // ... 其他逻辑方法
}

// internal/tui/components/message/renderer.go — TUI 渲染接口
type ToolRenderer interface {
    RenderToolUse(input map[string]any, opts RenderOptions) string
    RenderToolResult(output any, progress []ProgressMessage, opts RenderOptions) string
    RenderToolUseProgress(progress []ProgressMessage, opts RenderOptions) string
}
```

### 挑战 3：DeepImmutable AppState 的 Go 实现

**问题**：TypeScript 的 `DeepImmutable<AppState>` 利用类型系统在编译期阻止直接 mutation，用 `setState(fn)` 进行函数式更新。Go 没有原生的不可变类型。

**解决方案**：
- AppState 定义为**值类型**（struct，不含指针字段，或指针字段指向不可变数据）
- `Store.SetState(fn func(AppState) AppState)` 接受纯函数，内部 `newState = fn(oldState)` 产生副本
- 对于 `tasks map[string]TaskState`（含函数类型，原版用 `& {}`不包含在 DeepImmutable 中）使用 `sync.Map` 或单独的并发安全结构
- 通过 `go vet` + 代码规范禁止直接修改 AppState 字段（约定：所有写操作必须通过 `store.SetState`）

### 挑战 4：Feature Flags（bun:bundle feature()）

**问题**：原版大量使用 `feature('FEATURE_NAME')` 进行**编译时**死代码消除（bun:bundle 在打包时根据 feature flags 剔除代码）。这不是运行时 if，而是编译期裁剪。

**解决方案**：
- Go 版本使用 **build tags**（`//go:build` 约束）实现编译期特性开关
- 同时提供运行时版本：`var FeatureFlags = map[string]bool{...}` + 运行时检查
- 对于 Go 版本初期，将所有 feature 默认为 true（包含全部功能），后续按需用 build tags 裁剪

```go
// internal/bootstrap/features.go
const (
    FeatureCoordinatorMode = true   // COORDINATOR_MODE
    FeatureHistorySnip     = true   // HISTORY_SNIP
    // ...
)
```

### 挑战 5：PermissionRequest 的 TUI 交互等待

**问题**：原版的权限 ask 流程：工具执行 goroutine 需要**暂停等待**用户在 TUI 中做出选择，然后继续执行。这跨越了 goroutine 边界。

**解决方案**：
```go
// internal/permissions/interactive.go

// 权限请求：工具 goroutine 发送请求，阻塞等待响应
type PermissionRequest struct {
    Tool    tool.Tool
    Input   map[string]any
    RespCh  chan PermissionDecision  // 工具 goroutine 在此等待
}

// TUI 中的权限队列（对应 toolUseConfirmQueue）
type PermissionQueue struct {
    mu    sync.Mutex
    queue []PermissionRequest
    // 通知 TUI 有新请求
    notify chan struct{}
}

// 工具 goroutine 调用（阻塞）：
func (q *PermissionQueue) Request(ctx context.Context, req PermissionRequest) (PermissionDecision, error) {
    respCh := make(chan PermissionDecision, 1)
    req.RespCh = respCh
    q.mu.Lock()
    q.queue = append(q.queue, req)
    q.mu.Unlock()
    select { case <-q.notify: default: }
    // 等待 TUI 响应
    select {
    case decision := <-respCh:
        return decision, nil
    case <-ctx.Done():
        return PermissionDecision{}, ctx.Err()
    }
}

// TUI Update() 处理：显示对话框，用户点击后发送到 respCh
```

### 挑战 6：会话 Transcript 持久化（recordTranscript）

**问题**：原版 `recordTranscript` 使用 `flushSessionStorage`，底层可能是 SQLite 或文件系统异步写入，有 `fire-and-forget` 和等待两种模式（bare mode）。

**解决方案**：
- Go 使用文件系统 JSON 写入（与原版保持一致）
- `RecordTranscript` 接受 `fireAndForget bool` 参数
- 使用带大小限制的写缓冲区 + goroutine 异步刷盘
- 文件锁避免并发写入冲突（与 config 系统共用 lockfile 机制）

### 挑战 7：Ink VirtualMessageList → BubbleTea viewport 分页

**问题**：原版 REPL 使用 Ink 的 VirtualMessageList 实现了虚拟滚动（仅渲染可见区域的消息），以处理超长对话。BubbleTea 内置的 `viewport.Model` 是全量渲染后滚动，对于超长对话有性能问题。

**解决方案**：
- 初期使用 BubbleTea 的 `viewport.Model`（简单可靠）
- 优化阶段实现分页渲染：维护渲染窗口 `[startIdx, endIdx]`，只渲染窗口内消息
- 消息高度缓存：每条渲染后的消息记录行数，滚动时快速计算 offset

```go
// internal/tui/repl/view.go
func (m Model) renderVisibleMessages() string {
    // 计算可见窗口
    visibleHeight := m.height - headerHeight - footerHeight
    msgs := m.getVisibleMessages(visibleHeight)
    var sb strings.Builder
    for _, msg := range msgs {
        sb.WriteString(m.renderMessage(msg))
    }
    return sb.String()
}
```

---

*文档基于对原始 TypeScript 代码库（`src/QueryEngine.ts`, `src/query.ts`, `src/Tool.ts`, `src/tools/`, `src/services/api/`, `src/services/mcp/`, `src/screens/REPL.tsx`, `src/coordinator/`, `src/utils/config.ts`, `src/types/`, `src/state/AppState.ts`, `src/main.tsx`）的深度阅读分析生成。*
