# 工具层详细设计

> 负责 Agent：Agent-Tools
> 状态：设计中
> 日期：2026-04-02

---

## 目录

1. [Tool 接口依赖声明](#1-tool-接口依赖声明)
2. [目录结构设计](#2-目录结构设计)
3. [完整工具注册表](#3-完整工具注册表)
4. [各类工具实现详解](#4-各类工具实现详解)
5. [工具注册机制](#5-工具注册机制)
6. [并发安全分类](#6-并发安全分类)
7. [设计决策](#7-设计决策)

---

## 1. Tool 接口依赖声明

```go
// TODO(dep): 等待 Agent-Core #6 完成 Tool 接口定义后，补全此处并开始编码
// 预期接口位于 internal/tool/tool.go
//
// 基于对 src/Tool.ts 的分析，预期接口形态如下（仅为设计参考，以 Agent-Core 最终产出为准）：
//
// package tool
//
// import "context"
//
// // Tool 是所有内置工具的统一接口
// type Tool interface {
//     // Name 返回工具的唯一标识名称（如 "Bash", "Read", "Edit"）
//     Name() string
//
//     // Description 返回工具的功能描述（注入到 system prompt）
//     Description(ctx context.Context) (string, error)
//
//     // InputSchema 返回 JSON Schema（用于 API 的 tools 字段）
//     InputSchema() map[string]any
//
//     // Call 执行工具调用
//     Call(ctx context.Context, input json.RawMessage, opts CallOptions) (ToolResult, error)
//
//     // IsConcurrencySafe 是否可与其他工具并发执行（只读工具返回 true）
//     IsConcurrencySafe(input json.RawMessage) bool
//
//     // IsReadOnly 是否只读（决定权限提示级别）
//     IsReadOnly(input json.RawMessage) bool
//
//     // CheckPermissions 工具特定权限检查（通用权限由框架处理）
//     CheckPermissions(ctx context.Context, input json.RawMessage) (PermissionResult, error)
//
//     // IsEnabled 当前环境下该工具是否启用
//     IsEnabled() bool
// }
//
// type ToolResult struct {
//     Content     any    // 结构化结果数据
//     IsError     bool
// }
//
// type CallOptions struct {
//     AbortCh     <-chan struct{}     // 取消信号
//     OnProgress  func(data any)     // 进度回调（用于 AgentTool 等长时操作）
//     Context     *ToolUseContext    // 完整上下文（权限、状态等）
// }
```

---

## 2. 目录结构设计

### 推荐方案：**按类别分组**

```
internal/
└── tools/
    ├── registry.go           # 工具注册表（Registry + 全局注册函数）
    ├── fileops/              # 文件操作类
    │   ├── fileread.go
    │   ├── fileread_test.go
    │   ├── filewrite.go
    │   ├── filewrite_test.go
    │   ├── fileedit.go
    │   ├── fileedit_test.go
    │   ├── glob.go
    │   ├── glob_test.go
    │   ├── grep.go
    │   ├── grep_test.go
    │   ├── notebookedit.go
    │   └── notebookedit_test.go
    ├── shell/                # Shell 执行类
    │   ├── bash.go
    │   ├── bash_test.go
    │   ├── bash_security.go  # 危险命令检测
    │   ├── bash_sandbox.go   # 沙箱判断逻辑
    │   └── bash_timeout.go   # 超时控制
    ├── agent/                # Agent 调度类
    │   ├── agentool.go       # AgentTool（子 Agent 派发）
    │   ├── agentool_test.go
    │   ├── sendmessage.go    # Swarm 通信
    │   └── sendmessage_test.go
    ├── mcp/                  # MCP 工具类
    │   ├── mcptool.go        # MCPTool（调用 MCP 工具）
    │   ├── mcptool_test.go
    │   ├── listresources.go  # ListMcpResources
    │   └── readresource.go   # ReadMcpResource
    ├── web/                  # 网络类
    │   ├── webfetch.go
    │   ├── webfetch_test.go
    │   ├── websearch.go
    │   └── websearch_test.go
    ├── tasks/                # 任务管理类
    │   ├── taskcreate.go
    │   ├── taskget.go
    │   ├── tasklist.go
    │   ├── taskupdate.go
    │   ├── taskstop.go
    │   ├── taskoutput.go
    │   └── tasks_test.go
    ├── interact/             # 用户交互 & 模式控制类
    │   ├── askuser.go        # AskUserQuestion
    │   ├── todowrite.go      # TodoWrite
    │   ├── enterplanmode.go  # EnterPlanMode
    │   ├── exitplanmode.go   # ExitPlanMode
    │   ├── enterworktree.go  # EnterWorktree
    │   └── exitworktree.go   # ExitWorktree
    └── misc/                 # 其他工具
        ├── skill.go          # SkillTool
        ├── sleep.go          # SleepTool
        ├── brief.go          # BriefTool
        ├── toolsearch.go     # ToolSearch
        ├── syntheticoutput.go
        └── lsp.go            # LSPTool（可选）
```

### 方案选择理由

| 方案 | 优点 | 缺点 |
|------|------|------|
| **按类别分组（推荐）** | 相关工具共享辅助函数（如 shell/ 内的安全检测），目录数量可控（~8 个），便于导航 | 同类工具必须避免命名冲突 |
| 每工具独立子目录 | 隔离彻底，与 TS 源码结构对应 | 40 个子目录过于零散，公用代码难共享，Go 包粒度太细 |

**关键原则**：同一 package 内的工具**不得互相调用**（调用需经过 Core 层编排），共享的辅助函数放在同 package 的 `*_helper.go` 文件中。

---

## 3. 完整工具注册表

### 3.1 核心工具（稳定，全平台可用）

| 工具名 | Go 包路径 | 权限类型 | 并发安全 | 功能描述 |
|--------|-----------|----------|----------|----------|
| `Read` | `internal/tools/fileops` | ReadOnly | ✅ 是 | 读取文件内容（文本、图片、PDF、Notebook），支持行范围和分页 |
| `Write` | `internal/tools/fileops` | Write（需确认） | ❌ 否 | 创建或覆盖写入文件 |
| `Edit` | `internal/tools/fileops` | Write（需确认） | ❌ 否 | 精确字符串替换编辑文件（old_string → new_string） |
| `Glob` | `internal/tools/fileops` | ReadOnly | ✅ 是 | 按 glob 模式搜索文件路径 |
| `Grep` | `internal/tools/fileops` | ReadOnly | ✅ 是 | 正则搜索文件内容（ripgrep 语义） |
| `NotebookEdit` | `internal/tools/fileops` | Write（需确认） | ❌ 否 | 编辑 Jupyter Notebook 特定 Cell |
| `Bash` | `internal/tools/shell` | Write（需确认） | ❌ 否 | 执行 Shell 命令，含沙箱判断、超时、危险命令检测 |
| `Agent` | `internal/tools/agent` | Special | ❌ 否 | 派发子 Agent 执行子任务，管理子 Agent 生命周期 |
| `SendMessage` | `internal/tools/agent` | Special | ❌ 否 | Swarm 模式下向指定 Peer Agent 发送消息 |
| `WebFetch` | `internal/tools/web` | ReadOnly | ✅ 是 | 抓取 URL 内容（HTML 转 Markdown），支持提示词处理 |
| `WebSearch` | `internal/tools/web` | ReadOnly | ✅ 是 | 调用 Brave 搜索 API 返回搜索结果 |
| `TodoWrite` | `internal/tools/interact` | Write | ❌ 否 | 写入/更新 TODO 列表（结构化任务状态管理） |
| `AskUserQuestion` | `internal/tools/interact` | Special | ❌ 否 | 向用户提问并等待回答（阻塞式交互） |
| `EnterPlanMode` | `internal/tools/interact` | Special | ❌ 否 | 进入计划模式（只读规划，不执行操作） |
| `ExitPlanMode` | `internal/tools/interact` | Special | ❌ 否 | 退出计划模式，恢复正常执行权限 |
| `EnterWorktree` | `internal/tools/interact` | Special | ❌ 否 | 创建并切换到 Git Worktree（隔离工作目录） |
| `ExitWorktree` | `internal/tools/interact` | Special | ❌ 否 | 退出 Worktree，清理或保留分支 |
| `Skill` | `internal/tools/misc` | Special | ❌ 否 | 调用预定义的 Skill（内置斜杠命令封装） |
| `ToolSearch` | `internal/tools/misc` | ReadOnly | ✅ 是 | 按关键词搜索可用工具（Tool 延迟加载时的探索入口） |
| `Brief` | `internal/tools/misc` | ReadOnly | ✅ 是 | 返回当前会话摘要信息 |
| `ListMcpResources` | `internal/tools/mcp` | ReadOnly | ✅ 是 | 列出 MCP 服务器上的所有资源 |
| `ReadMcpResource` | `internal/tools/mcp` | ReadOnly | ✅ 是 | 读取指定 MCP 资源内容 |
| `MCPTool` | `internal/tools/mcp` | Special | — | MCP 动态工具包装器（运行时从 MCP 服务器发现并注册） |

### 3.2 任务管理工具（TodoV2 功能标志控制）

| 工具名 | Go 包路径 | 权限类型 | 并发安全 | 功能描述 |
|--------|-----------|----------|----------|----------|
| `TaskCreate` | `internal/tools/tasks` | Write | ❌ 否 | 创建异步后台任务（子 Agent 实例） |
| `TaskGet` | `internal/tools/tasks` | ReadOnly | ✅ 是 | 查询单个任务的状态和元数据 |
| `TaskList` | `internal/tools/tasks` | ReadOnly | ✅ 是 | 列出所有任务及其状态 |
| `TaskUpdate` | `internal/tools/tasks` | Write | ❌ 否 | 更新任务的优先级或元数据 |
| `TaskStop` | `internal/tools/tasks` | Write | ❌ 否 | 停止并清理指定后台任务 |
| `TaskOutput` | `internal/tools/tasks` | ReadOnly | ✅ 是 | 读取任务的流式输出（进度/结果） |

### 3.3 条件启用工具（特性标志或平台限制）

| 工具名 | Go 包路径 | 启用条件 | 功能描述 |
|--------|-----------|----------|----------|
| `REPL` | `internal/tools/misc` | `USER_TYPE=ant` | 代码 REPL 执行环境（Bun VM） |
| `PowerShell` | `internal/tools/shell` | Windows 平台 | PowerShell 命令执行 |
| `Sleep` | `internal/tools/misc` | `PROACTIVE` 特性 | 等待指定毫秒数（Agent 主动轮询用） |
| `LSP` | `internal/tools/misc` | `ENABLE_LSP_TOOL=true` | LSP 诊断与代码智能 |
| `ScheduleCron` | `internal/tools/misc` | `AGENT_TRIGGERS` 特性 | 定时任务调度 |
| `TeamCreate` | `internal/tools/agent` | `AGENT_SWARMS` 特性 | 创建 Agent Swarm 团队 |
| `TeamDelete` | `internal/tools/agent` | `AGENT_SWARMS` 特性 | 删除 Agent Swarm 团队 |
| `SyntheticOutput` | `internal/tools/misc` | SDK 模式 | 生成合成输出（用于 SDK streaming） |

---

## 4. 各类工具实现详解

### 4.1 文件操作类

#### 4.1.1 实现模式（以 FileRead 为例展开完整代码骨架）

```go
// internal/tools/fileops/fileread.go
package fileops

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"

    // TODO(dep): 等待 Agent-Core 完成接口定义
    // tool "github.com/your-org/claude-code-go/internal/tool"
)

// FileReadInput 对应 TS inputSchema（与原版保持完全一致）
type FileReadInput struct {
    FilePath string  `json:"file_path"`          // 必填，绝对路径
    Offset   *int    `json:"offset,omitempty"`   // 起始行（1-indexed），默认 1
    Limit    *int    `json:"limit,omitempty"`    // 读取行数，默认全部
    Pages    *string `json:"pages,omitempty"`    // PDF 页范围，如 "1-5"
}

// FileReadOutput 结构化输出（discriminated union 用 Type 字段区分）
type FileReadOutput struct {
    Type string          `json:"type"` // "text" | "image" | "notebook" | "pdf" | "file_unchanged"
    File FileReadContent `json:"file"`
}

type FileReadContent struct {
    FilePath   string `json:"file_path,omitempty"`
    Content    string `json:"content,omitempty"`
    NumLines   int    `json:"num_lines,omitempty"`
    StartLine  int    `json:"start_line,omitempty"`
    TotalLines int    `json:"total_lines,omitempty"`
    // 图片专用字段
    Base64      string `json:"base64,omitempty"`
    MediaType   string `json:"type,omitempty"`
    OriginalSize int64 `json:"original_size,omitempty"`
}

// fileReadTool 实现 tool.Tool 接口
// TODO(dep): 完整实现需等待 internal/tool 接口就绪
type fileReadTool struct{}

// FileReadTool 是导出的单例实例（对应 TS 的 buildTool({...}) 导出）
var FileReadTool = &fileReadTool{}

func (t *fileReadTool) Name() string { return "Read" }

func (t *fileReadTool) Description(_ context.Context) (string, error) {
    return `Reads a file from the local filesystem. You can access any file directly by using this tool.
Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid.
...`, nil
}

func (t *fileReadTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "file_path": map[string]any{
                "type":        "string",
                "description": "The absolute path to the file to read",
            },
            "offset": map[string]any{
                "type":        "integer",
                "description": "The line number to start reading from. Only provide if the file is too large to read at once",
            },
            "limit": map[string]any{
                "type":        "integer",
                "description": "The number of lines to read. Only provide if the file is too large to read at once.",
            },
            "pages": map[string]any{
                "type":        "string",
                "description": `Page range for PDF files (e.g., "1-5", "3", "10-20"). Only applicable to PDF files. Maximum 20 pages per request.`,
            },
        },
        "required": []string{"file_path"},
    }
}

func (t *fileReadTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *fileReadTool) IsReadOnly(_ json.RawMessage) bool         { return true }
func (t *fileReadTool) IsEnabled() bool                           { return true }

// Call 是核心执行逻辑
func (t *fileReadTool) Call(ctx context.Context, rawInput json.RawMessage, opts CallOptions) (ToolResult, error) {
    var input FileReadInput
    if err := json.Unmarshal(rawInput, &input); err != nil {
        return ToolResult{IsError: true}, fmt.Errorf("invalid input: %w", err)
    }

    // 1. 路径规范化（expandPath：处理 ~、相对路径、Windows 路径分隔符）
    fullPath := expandPath(input.FilePath)

    // 2. 安全检查（设备文件阻断：/dev/zero, /dev/random 等）
    if isBlockedDevicePath(fullPath) {
        return ToolResult{IsError: true}, fmt.Errorf("cannot read device file: %s", fullPath)
    }

    // 3. 权限检查（通过 opts.Context 获取权限上下文）
    // TODO(dep): if err := opts.Context.Permissions.CheckRead(fullPath); err != nil { ... }

    // 4. 按文件类型分支处理
    ext := strings.ToLower(filepath.Ext(fullPath))
    switch {
    case isImageExtension(ext):
        return t.readImage(ctx, fullPath, opts)
    case ext == ".ipynb":
        return t.readNotebook(ctx, fullPath, opts)
    case ext == ".pdf":
        return t.readPDF(ctx, input, fullPath, opts)
    default:
        return t.readText(ctx, input, fullPath, opts)
    }
}

// readText 读取普通文本文件（含行号、token 限制检查、去重缓存）
func (t *fileReadTool) readText(ctx context.Context, input FileReadInput, fullPath string, opts CallOptions) (ToolResult, error) {
    offset := 0
    if input.Offset != nil {
        offset = *input.Offset - 1 // 转换为 0-indexed
    }

    content, lineCount, totalLines, err := readFileInRange(fullPath, offset, input.Limit, opts.AbortCh)
    if err != nil {
        if os.IsNotExist(err) {
            return ToolResult{IsError: true}, fmt.Errorf("file does not exist: %s", fullPath)
        }
        return ToolResult{IsError: true}, err
    }

    // Token 数量检查（防止超大文件撑爆上下文）
    if err := validateContentTokens(content, ext, opts.Context.FileReadingLimits); err != nil {
        return ToolResult{IsError: true}, err
    }

    output := FileReadOutput{
        Type: "text",
        File: FileReadContent{
            FilePath:   input.FilePath,
            Content:    content,
            NumLines:   lineCount,
            StartLine:  offset + 1,
            TotalLines: totalLines,
        },
    }
    return ToolResult{Content: output}, nil
}
```

#### 4.1.2 文件操作类权限设计

| 工具 | 默认权限要求 | 可配置绕过 | 说明 |
|------|------------|-----------|------|
| FileRead | 无需确认（ReadOnly） | alwaysDeny 规则 | 路径受 deny 规则过滤 |
| FileWrite | 需要用户确认 | alwaysAllow 规则 | 创建/覆盖写入属于破坏性操作 |
| FileEdit | 需要用户确认 | alwaysAllow 规则 | old_string 不匹配时自动失败，不会误覆盖 |
| Glob | 无需确认（ReadOnly） | — | 纯路径搜索 |
| Grep | 无需确认（ReadOnly） | — | 纯内容搜索 |
| NotebookEdit | 需要用户确认 | alwaysAllow 规则 | 修改 .ipynb 特定 cell |

**阻断设备文件列表**（对应 TS `BLOCKED_DEVICE_PATHS`）：
```go
var blockedDevicePaths = map[string]bool{
    "/dev/zero": true, "/dev/random": true, "/dev/urandom": true,
    "/dev/full": true, "/dev/stdin": true, "/dev/tty": true,
    "/dev/console": true, "/dev/stdout": true, "/dev/stderr": true,
    "/dev/fd/0": true, "/dev/fd/1": true, "/dev/fd/2": true,
}
```

---

### 4.2 Shell 执行类（Bash）

#### 4.2.1 输入 Schema

```go
type BashInput struct {
    Command                string  `json:"command"`
    Timeout                *int    `json:"timeout,omitempty"`               // 毫秒，默认 120000（2 分钟），最大 600000
    DangerouslyDisableSandbox bool `json:"dangerously_disable_sandbox,omitempty"`
}
```

#### 4.2.2 危险命令检测逻辑

对应 TS `bashSecurity.ts` + `bashCommandHelpers.ts`，分三层：

```
Layer 1: 用户配置排除命令（settings.sandbox.excludedCommands）
    → 从 settings 读取 excludedCommands 列表
    → 支持精确匹配、前缀匹配、通配符（"git *"）
    → 拆分复合命令（&&、||、;、|）逐段检查
    → 注意：这是用户体验功能，不是安全边界

Layer 2: 安全规则匹配（bashPermissions）
    → cd + git 跨管道段攻击检测
    → 环境变量劫持检测（BINARY_HIJACK_VARS）
    → 安全包装器剥离（timeout, nice, env 等）后再匹配

Layer 3: 权限系统决策
    → 匹配 alwaysAllow / alwaysDeny / alwaysAsk 规则
    → 无匹配时根据工具默认策略（ask）展示用户确认
```

关键实现：

```go
// internal/tools/shell/bash_security.go

// BashPermissionRule 表示一条权限规则
type BashPermissionRule struct {
    Type    string // "prefix" | "exact" | "wildcard"
    Prefix  string
    Command string
    Pattern string
}

// ParseBashPermissionRule 解析 "git *" 或 "npm install" 等规则字符串
func ParseBashPermissionRule(pattern string) BashPermissionRule { ... }

// ContainsExcludedCommand 检查命令是否被用户配置的排除列表命中
// 注意：不是安全边界，不要作为安全防护使用
func ContainsExcludedCommand(command string, excludedPatterns []string) bool {
    // 1. 拆分复合命令（&&, ||, ;, |）
    subcommands := splitCompoundCommand(command)
    for _, sub := range subcommands {
        // 2. 固定点迭代：剥离环境变量前缀和安全包装器
        candidates := expandCandidates(sub)
        for _, cand := range candidates {
            for _, pattern := range excludedPatterns {
                rule := ParseBashPermissionRule(pattern)
                if rule.Matches(cand) {
                    return true
                }
            }
        }
    }
    return false
}
```

#### 4.2.3 沙箱判断

对应 TS `shouldUseSandbox.ts`：

```go
// internal/tools/shell/bash_sandbox.go

// ShouldUseSandbox 决定是否在沙箱中执行命令
// 逻辑：沙箱总开关 AND NOT（用户明确禁用 AND 策略允许禁用）AND NOT 排除命令
func ShouldUseSandbox(cmd string, disableSandbox bool, settings *SandboxSettings) bool {
    if !SandboxManager.IsEnabled() {
        return false
    }
    if disableSandbox && SandboxManager.AreUnsandboxedCommandsAllowed() {
        return false
    }
    if cmd == "" {
        return false
    }
    if ContainsExcludedCommand(cmd, settings.ExcludedCommands) {
        return false
    }
    return true
}
```

#### 4.2.4 超时控制

```go
// internal/tools/shell/bash_timeout.go

const (
    DefaultBashTimeoutMs = 120_000  // 2 分钟
    MaxBashTimeoutMs     = 600_000  // 10 分钟
)

func executeBashWithTimeout(ctx context.Context, cmd string, timeoutMs int) (stdout, stderr string, err error) {
    if timeoutMs <= 0 {
        timeoutMs = DefaultBashTimeoutMs
    }
    if timeoutMs > MaxBashTimeoutMs {
        timeoutMs = MaxBashTimeoutMs
    }

    timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
    defer cancel()

    // 使用 os/exec 执行，继承当前工作目录
    c := exec.CommandContext(timeoutCtx, "bash", "-c", cmd)
    // ...
}
```

---

### 4.3 Agent 调度类

#### 4.3.1 AgentTool：子 Agent 创建与生命周期

对应 TS `AgentTool/runAgent.ts`，核心设计：

```go
// internal/tools/agent/agentool.go

type AgentInput struct {
    Prompt      string   `json:"prompt"`                   // 子任务描述
    SystemPrompt *string `json:"system_prompt,omitempty"`  // 自定义系统提示（覆盖默认）
    AllowedTools []string `json:"allowed_tools,omitempty"` // 限定子 Agent 可用工具
    MaxTurns     *int    `json:"max_turns,omitempty"`      // 最大对话轮数
}

// AgentTool 调用流程（每次调用是一个独立子会话）：
//
// 1. 创建独立 QueryEngine 实例（cloneFileStateCache，复制父 context）
// 2. 构建子 Agent system prompt（注入 coordinator 上下文）
// 3. 过滤工具列表（AllowedTools 限制 + AGENT_DISALLOWED_TOOLS 排除）
// 4. 运行子 Agent 主循环（使用 errgroup 追踪）
// 5. 监听取消信号（父 Agent 中断时级联取消子 Agent）
// 6. 收集并返回子 Agent 的最终消息

func (t *agentTool) Call(ctx context.Context, rawInput json.RawMessage, opts CallOptions) (ToolResult, error) {
    var input AgentInput
    if err := json.Unmarshal(rawInput, &input); err != nil {
        return ToolResult{IsError: true}, err
    }

    // TODO(dep): 子 Agent 创建依赖 Agent-Core 的 QueryEngine 接口
    // subEngine := opts.Context.Engine.Fork(ForkOptions{
    //     SystemPrompt: input.SystemPrompt,
    //     AllowedTools: input.AllowedTools,
    //     MaxTurns:     input.MaxTurns,
    // })
    //
    // result, err := subEngine.Run(ctx, input.Prompt, opts.OnProgress)
    // ...

    return ToolResult{Content: "TODO: sub-agent result"}, nil
}
```

**子 Agent 禁用工具列表**（对应 TS `ALL_AGENT_DISALLOWED_TOOLS`）：
- `AskUserQuestion`（子 Agent 不能向用户提问，会阻塞父流程）
- `EnterPlanMode` / `ExitPlanMode`（子 Agent 模式由父 Agent 控制）
- `EnterWorktree` / `ExitWorktree`（Worktree 作用域由父 Agent 管理）

#### 4.3.2 SendMessage：Swarm 通信

```go
// internal/tools/agent/sendmessage.go

type SendMessageInput struct {
    AgentID string `json:"agent_id"` // 目标 Agent 的 UUID
    Content string `json:"content"`  // 消息内容
}

// SendMessageTool 通过内存 channel 路由将消息发送到指定 Agent 的收件箱
// 设计：使用全局 AgentRouter（注册表模式），按 AgentID 查找对应的 inbox channel
//
// TODO(dep): 依赖 Agent-Core 的 AgentRouter 接口
```

---

### 4.4 MCP 类

```go
// TODO(dep): 依赖 Agent-Services MCP 客户端接口（internal/mcp）
//
// MCPTool 是 MCP 远程工具的动态包装器：
// - 运行时从 MCP 服务器发现工具 schema
// - 调用 mcp.Client.CallTool(serverName, toolName, input)
// - 将结果转换为 ToolResult
//
// ListMcpResources / ReadMcpResource：
// - 调用 mcp.Client.ListResources(serverName) / ReadResource(serverName, uri)
// - 只读，并发安全
```

**接口预期**（等待 Agent-Services 完成后回填）：

```go
type MCPClient interface {
    CallTool(ctx context.Context, serverName, toolName string, input json.RawMessage) (MCPToolResult, error)
    ListResources(ctx context.Context, serverName string) ([]MCPResource, error)
    ReadResource(ctx context.Context, serverName, uri string) (MCPResourceContent, error)
    ListTools(ctx context.Context, serverName string) ([]MCPToolDef, error)
}
```

---

### 4.5 网络类

#### 4.5.1 WebFetch

```go
type WebFetchInput struct {
    URL    string `json:"url"`    // 必须是 HTTPS（HTTP 自动升级）
    Prompt string `json:"prompt"` // 对页面内容运行的处理提示词
}

// 实现要点：
// 1. HTTP → HTTPS 自动升级
// 2. 用 goquery 或 html2text 将 HTML 转换为 Markdown
// 3. 内容过长时用小模型按 Prompt 提炼摘要
// 4. 15 分钟 LRU 缓存（相同 URL 缓存结果）
// 5. 重定向跟踪（最多 5 跳，遇跨主机重定向时告知用户）
// 6. 并发安全（只读操作）
```

#### 4.5.2 WebSearch

```go
type WebSearchInput struct {
    Query          string   `json:"query"`
    AllowedDomains []string `json:"allowed_domains,omitempty"`
    BlockedDomains []string `json:"blocked_domains,omitempty"`
}

// 实现要点：
// 1. 调用 Brave Search API（或配置的搜索服务）
// 2. 返回格式化的搜索结果 Markdown（标题 + 摘要 + URL）
// 3. 支持域名过滤（allowed/blocked）
// 4. 需要 BRAVE_API_KEY 或等效配置
// 5. 并发安全（只读操作）
```

---

### 4.6 任务管理类

对应 TS 中 TodoV2 功能的 TaskCreate/Get/List/Update/Stop/Output 系列：

```go
// 任务状态机
type TaskStatus string
const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusStopped   TaskStatus = "stopped"
)

// TaskCreateInput 创建异步后台任务
type TaskCreateInput struct {
    Description string   `json:"description"` // 任务描述（子 Agent prompt）
    Tools       []string `json:"tools,omitempty"` // 限定工具集
    Priority    *int     `json:"priority,omitempty"`
}

// TaskOutputInput 读取任务流式输出
type TaskOutputInput struct {
    TaskID string `json:"task_id"`
    Since  *int   `json:"since,omitempty"` // 从第几条输出开始读（用于增量拉取）
}

// 实现要点：
// - 所有 Task 工具共享一个 TaskRegistry（全局任务注册表）
// - TaskRegistry 存储在 AppState 中（需要 Agent-Infra 支持）
// - TaskCreate 启动 goroutine 运行子 Agent，注册到 TaskRegistry
// - TaskStop 向目标 goroutine 发送取消信号（context.CancelFunc）
// - TaskOutput 从 channel 读取进度消息（非阻塞，返回当前已有输出）
// - 任务 ID 使用 UUID v4
```

---

### 4.7 其他工具

#### AskUserQuestion

```go
// 核心机制：通过 opts.Context 持有的 channel 向 TUI 发出问询请求，阻塞等待用户回答
// 工具本身不直接与 TUI 耦合，通过接口注入的 RequestPrompt 函数间接触发

type AskUserQuestionInput struct {
    Question string `json:"question"`
}

// TODO(dep): 依赖 Agent-Core 注入的 RequestPrompt 接口
// response, err := opts.Context.RequestPrompt(input.Question)
```

#### TodoWrite

```go
// 管理结构化 TODO 列表（与 TUI 的 Todo Panel 同步）
// 注意：TodoWrite 工具不在消息流中渲染（renderToolResultMessage 返回 null）
// 而是更新 AppState 中的 todoItems，由 TUI 的 Todo Panel 订阅并展示

type TodoItem struct {
    ID       string `json:"id"`
    Content  string `json:"content"`
    Status   string `json:"status"` // "pending" | "in_progress" | "completed"
    Priority string `json:"priority"` // "high" | "medium" | "low"
}

type TodoWriteInput struct {
    Todos []TodoItem `json:"todos"`
}
```

#### EnterPlanMode / ExitPlanMode

```go
// 通过修改 ToolPermissionContext.Mode 实现模式切换
// EnterPlanMode → mode = "plan" （只允许只读工具，写操作全部拒绝）
// ExitPlanMode  → mode = 恢复到 prePlanMode

// 实现：更新 AppState.ToolPermissionContext
// TODO(dep): 依赖 Agent-Core 的 AppState 操作接口
```

---

## 5. 工具注册机制

### 5.1 Registry 设计

```go
// internal/tools/registry.go
package tools

import (
    // TODO(dep): tool "github.com/your-org/claude-code-go/internal/tool"
)

// Registry 持有所有已注册的工具
type Registry struct {
    mu    sync.RWMutex
    tools map[string]Tool // name → Tool
    order []string        // 保持注册顺序（用于 API 工具列表）
}

// NewRegistry 创建空注册表
func NewRegistry() *Registry {
    return &Registry{tools: make(map[string]Tool)}
}

// Register 注册一个工具（重复注册 panic）
func (r *Registry) Register(t Tool) {
    r.mu.Lock()
    defer r.mu.Unlock()
    name := t.Name()
    if _, exists := r.tools[name]; exists {
        panic(fmt.Sprintf("tool already registered: %s", name))
    }
    r.tools[name] = t
    r.order = append(r.order, name)
}

// Get 按名称查找工具
func (r *Registry) Get(name string) (Tool, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    t, ok := r.tools[name]
    return t, ok
}

// All 返回所有已启用工具（按注册顺序）
func (r *Registry) All() []Tool {
    r.mu.RLock()
    defer r.mu.RUnlock()
    result := make([]Tool, 0, len(r.order))
    for _, name := range r.order {
        t := r.tools[name]
        if t.IsEnabled() {
            result = append(result, t)
        }
    }
    return result
}

// Filter 按条件过滤工具（用于子 Agent 的 AllowedTools 限制）
func (r *Registry) Filter(allowedNames []string) []Tool {
    if len(allowedNames) == 0 {
        return r.All()
    }
    allowed := make(map[string]bool)
    for _, n := range allowedNames {
        allowed[n] = true
    }
    r.mu.RLock()
    defer r.mu.RUnlock()
    var result []Tool
    for _, name := range r.order {
        if allowed[name] {
            if t := r.tools[name]; t.IsEnabled() {
                result = append(result, t)
            }
        }
    }
    return result
}
```

### 5.2 全局默认注册表与工厂函数

```go
// internal/tools/registry.go（续）

// DefaultRegistry 是全局默认注册表
var DefaultRegistry = NewRegistry()

// RegisterAll 将所有核心内置工具注册到目标 Registry
// 在程序启动时由 CLI 入口调用
func RegisterAll(r *Registry, cfg *Config) {
    // 文件操作（始终注册）
    r.Register(fileops.FileReadTool)
    r.Register(fileops.FileWriteTool)
    r.Register(fileops.FileEditTool)
    r.Register(fileops.GlobTool)
    r.Register(fileops.GrepTool)
    r.Register(fileops.NotebookEditTool)

    // Shell
    r.Register(shell.NewBashTool(cfg.Sandbox))

    // Agent 调度
    r.Register(agent.AgentTool)
    r.Register(agent.SendMessageTool)

    // 网络
    r.Register(web.WebFetchTool)
    r.Register(web.WebSearchTool)

    // 用户交互
    r.Register(interact.TodoWriteTool)
    r.Register(interact.AskUserQuestionTool)
    r.Register(interact.EnterPlanModeTool)
    r.Register(interact.ExitPlanModeTool)

    // 条件注册（特性标志）
    if cfg.WorktreeModeEnabled {
        r.Register(interact.EnterWorktreeTool)
        r.Register(interact.ExitWorktreeTool)
    }
    if cfg.TodoV2Enabled {
        r.Register(tasks.TaskCreateTool)
        r.Register(tasks.TaskGetTool)
        r.Register(tasks.TaskListTool)
        r.Register(tasks.TaskUpdateTool)
        r.Register(tasks.TaskStopTool)
        r.Register(tasks.TaskOutputTool)
    }

    // MCP（动态注册由 MCP 客户端在连接后调用 r.Register）
    r.Register(mcp.ListMcpResourcesTool)
    r.Register(mcp.ReadMcpResourceTool)

    // 其他
    r.Register(misc.SkillTool)
    r.Register(misc.BriefTool)
    if cfg.ToolSearchEnabled {
        r.Register(misc.ToolSearchTool)
    }
}
```

---

## 6. 并发安全分类

### 6.1 可并发工具（只读，isConcurrencySafe = true）

这些工具在同一 turn 内被 LLM 并行调用时，查询引擎可使用 `errgroup` 并发执行：

| 工具 | 理由 |
|------|------|
| `Read` | 只读文件系统，无写操作，无共享状态 |
| `Glob` | 只读目录遍历 |
| `Grep` | 只读内容搜索 |
| `WebFetch` | HTTP GET，无副作用，有本地缓存（缓存写入需 mutex） |
| `WebSearch` | 调用外部 API，无本地副作用 |
| `TaskGet` | 只读任务状态 |
| `TaskList` | 只读任务列表 |
| `TaskOutput` | 只读输出 channel |
| `ListMcpResources` | 只读 MCP 资源列表 |
| `ReadMcpResource` | 只读 MCP 资源内容 |
| `Brief` | 只读会话摘要 |
| `ToolSearch` | 只读工具元数据搜索 |

### 6.2 必须串行工具（写操作，isConcurrencySafe = false）

| 工具 | 理由 |
|------|------|
| `Write` | 写文件，多并发可能互相覆盖 |
| `Edit` | 读-校验-写，需原子性（依赖 readFileState 去重缓存一致性） |
| `NotebookEdit` | 修改 .ipynb，同上 |
| `Bash` | 命令可能修改文件系统、环境变量，副作用不可叠加 |
| `AgentTool` | 子 Agent 可能内部调用写工具，且修改 AppState |
| `TaskCreate` | 写 TaskRegistry |
| `TaskUpdate` | 写 TaskRegistry |
| `TaskStop` | 写 TaskRegistry（状态转换） |
| `AskUserQuestion` | 阻塞等待用户输入，一次只能一个 |
| `TodoWrite` | 写 AppState.TodoItems |
| `EnterPlanMode` / `ExitPlanMode` | 修改 PermissionContext.Mode |
| `SendMessage` | 写目标 Agent 的 inbox channel |

### 6.3 并发编排规则（供查询引擎参考）

```
同一 turn 的工具调用批次划分：
1. 收集本 turn 所有 tool_use 请求
2. 将 isConcurrencySafe=true 的请求分为一批，用 errgroup 并发执行
3. 将 isConcurrencySafe=false 的请求各自单独串行执行
4. 如果批次中有任何写工具，整个批次退化为串行
```

---

## 7. 设计决策

### 7.1 为什么用按类别分组而非每工具独立目录？

原版 TS 每个工具是独立目录（`src/tools/BashTool/`）。Go 的包粒度与 TS 模块不同——Go package 是编译单元，40 个子 package 带来的 import 链路冗长、公用代码难以共享。同类工具（如 fileops）天然共享辅助函数（路径处理、token 计数），分组后可直接复用，无需 `internal/tools/shared` 中间层。

### 7.2 工具单例 vs 每次调用构造

所有工具实现为单例（包级 `var`）。原版 TS 也是 `export const BashTool = buildTool({...})`，每次调用是对同一对象的方法调用，状态通过参数和 context 传递而非实例变量。Go 实现遵循同样模式：工具 struct 无可变字段，调用状态完全在 `Call` 方法参数中传递。

### 7.3 输出类型使用 discriminated union 还是多接口？

选择 `type` 字段 + 单一 Output struct（discriminated union），与原版 TS 的 Zod `discriminatedUnion` 对应。这样序列化成 JSON 时格式与原版完全一致，便于将来与 SDK 对接验证。

### 7.4 BashTool：沙箱实现策略

原版 TS 的 `SandboxManager` 在 macOS 使用 Apple 沙箱（`sandbox-exec`），在 Linux 使用 seccomp + namespaces。Go 版本：
- **Phase 1（MVP）**：`ShouldUseSandbox` 逻辑保留，但沙箱执行层使用 `exec.Command` 普通执行，允许通过配置关闭。
- **Phase 2**：接入 `libseccomp-go` 或 `gVisor` 实现真正的沙箱隔离。
- 设计上沙箱层通过 `BashExecutor` 接口抽象，Phase 1 和 Phase 2 实现可替换。

### 7.5 MCPTool 动态注册

MCP 工具在运行时发现，无法在编译期注册。设计：MCP 客户端连接后调用 `Registry.Register(mcpTool)` 动态注册。为避免并发注册冲突，Register 加 mutex。为支持 MCP 断线重连后工具列表变更，Registry 提供 `Deregister` 和 `Replace` 方法。

### 7.6 Token 预算与文件读取限制

FileRead 的 token 检查（`validateContentTokens`）在 Go 中实现为：
1. 按文件扩展名做粗略估算（代码文件 ≈ 每字节 0.3 token，普通文本 ≈ 每字节 0.25 token）
2. 粗略估算 > 上限/4 时，调用 Anthropic Token Count API 精确计算
3. 精确计算 > 上限时，返回 `MaxFileReadTokenExceededError`，提示模型使用 offset/limit

### 7.7 FileEdit 的唯一性验证

FileEdit 要求 `old_string` 在文件中唯一出现一次。实现：
1. 读取文件内容（`os.ReadFile`）
2. 统计 `strings.Count(content, oldString)`
3. count == 0 → 返回"未找到匹配字符串"
4. count > 1 → 返回"匹配到多处，请提供更多上下文"
5. count == 1 → 执行替换，写回文件

唯一性检查不依赖锁，但整个"读-检查-写"必须串行（`isConcurrencySafe = false`），避免 TOCTOU。

### 7.8 AgentTool 取消传播

子 Agent 运行时父 Agent 中止（用户 Ctrl+C），需要级联取消所有子 Agent。实现：每个子 Agent 使用 `context.WithCancel(parentCtx)` 派生 context，父 context 取消时子 Agent 的主循环感知到 `ctx.Done()` 并退出。同时在 AppState 中记录所有运行中的子 Agent goroutine，用于清理检查。
