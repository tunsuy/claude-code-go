# 核心层详细设计

> 负责 Agent：Agent-Core
> 状态：设计中
> 日期：2026-04-02

---

## 1. internal/tool — Tool 接口契约 ⭐

> **这是最重要的部分**。Agent-Tools 层所有工具实现均依赖此接口。任何变更须向后兼容或通知所有 Agent。

### 1.1 包定位

```
internal/tool/
├── tool.go          # 核心接口定义（本节完整给出）
├── registry.go      # 工具注册表
├── result.go        # ToolResult / Output 类型
└── permission.go    # PermissionType 枚举 + 相关类型
```

### 1.2 完整 Go 接口定义

```go
// Package tool 定义工具接口契约，供工具层实现、引擎层调用。
// 此包不应依赖 engine/permissions/tui 等其他核心包，避免循环依赖。
package tool

import (
    "context"
    "encoding/json"
)

// ─────────────────────────────────────────────
// 基础类型
// ─────────────────────────────────────────────

// Input 表示工具调用的输入参数（JSON 对象）。
// 工具实现需将 json.RawMessage 解码到具体类型。
type Input = json.RawMessage

// InputSchema 描述工具输入的 JSON Schema（object 类型）。
type InputSchema struct {
    Type       string                      `json:"type"` // 固定为 "object"
    Properties map[string]json.RawMessage  `json:"properties,omitempty"`
    Required   []string                    `json:"required,omitempty"`
    // 允许透传其他 JSON Schema 字段（description、examples 等）
    Extra      map[string]json.RawMessage  `json:"-"`
}

// ─────────────────────────────────────────────
// 权限枚举
// ─────────────────────────────────────────────

// PermissionBehavior 表示权限判断的三级决策。
type PermissionBehavior string

const (
    PermissionAllow       PermissionBehavior = "allow"
    PermissionDeny        PermissionBehavior = "deny"
    PermissionAsk         PermissionBehavior = "ask"
    PermissionPassthrough PermissionBehavior = "passthrough"
)

// PermissionMode 表示全局权限工作模式。
type PermissionMode string

const (
    PermissionModeDefault          PermissionMode = "default"
    PermissionModeAcceptEdits      PermissionMode = "acceptEdits"
    PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
    PermissionModeDontAsk          PermissionMode = "dontAsk"
    PermissionModePlan             PermissionMode = "plan"
    PermissionModeAuto             PermissionMode = "auto"
)

// ─────────────────────────────────────────────
// 权限结果类型
// ─────────────────────────────────────────────

// PermissionResult 是 checkPermissions 的返回值。
type PermissionResult struct {
    Behavior     PermissionBehavior
    // 仅 allow 时有效：引擎可能修改了输入（hooks/规则 updatedInput）
    UpdatedInput Input
    // 仅 deny/ask 时有效：展示给用户的原因
    Message      string
    // 决策溯源（可选，用于日志/UI 展示）
    Reason       *PermissionDecisionReason
}

// PermissionDecisionReason 描述决策来源。
type PermissionDecisionReason struct {
    Type string // "rule" | "mode" | "hook" | "classifier" | "other"
    // 以下字段按 Type 择一填充
    RuleName   string
    ModeName   string
    HookName   string
    HookSource string
    Reason     string
}

// ─────────────────────────────────────────────
// 权限上下文
// ─────────────────────────────────────────────

// PermissionRuleSource 表示规则来源。
type PermissionRuleSource string

const (
    RuleSourceUserSettings    PermissionRuleSource = "userSettings"
    RuleSourceProjectSettings PermissionRuleSource = "projectSettings"
    RuleSourceLocalSettings   PermissionRuleSource = "localSettings"
    RuleSourceSession         PermissionRuleSource = "session"
    RuleSourceCLIArg          PermissionRuleSource = "cliArg"
)

// PermissionContext 是引擎在调用 CheckPermissions 时传入的只读上下文。
// 工具不得修改此结构体。
type PermissionContext struct {
    Mode                        PermissionMode
    AdditionalWorkingDirectories map[string]AdditionalWorkingDirectory
    AlwaysAllowRules            map[PermissionRuleSource][]string
    AlwaysDenyRules             map[PermissionRuleSource][]string
    AlwaysAskRules              map[PermissionRuleSource][]string
    IsBypassPermissionsModeAvailable bool
    ShouldAvoidPermissionPrompts bool
}

// AdditionalWorkingDirectory 描述权限范围内的附加目录。
type AdditionalWorkingDirectory struct {
    Path   string
    Source PermissionRuleSource
}

// ─────────────────────────────────────────────
// 工具进度
// ─────────────────────────────────────────────

// ProgressData 是工具执行过程中推送的进度事件，具体结构由各工具定义。
// 工具通过 onProgress 回调推送；引擎转发给 TUI 层。
type ProgressData struct {
    ToolUseID string
    Data      any // 工具自定义结构（BashProgress / MCPProgress 等）
}

// OnProgressFn 是工具传入的进度回调函数类型。
type OnProgressFn func(progress ProgressData)

// ─────────────────────────────────────────────
// 工具执行结果
// ─────────────────────────────────────────────

// Result 是工具执行的完整返回值。
type Result struct {
    // Data 是工具输出，序列化后写入 tool_result 消息块。
    Data any

    // NewMessages 允许工具向对话历史追加额外消息（如 SubAgent 完整对话）。
    NewMessages []Message

    // ContextModifier 仅对非并发安全工具有效，用于更新 ToolUseContext。
    // 并发安全工具返回 nil。
    ContextModifier func(ctx *UseContext) *UseContext

    // MCPMeta 用于透传 MCP 协议元数据（_meta, structuredContent）。
    MCPMeta *MCPMeta
}

// MCPMeta 存储 MCP 协议的扩展元数据。
type MCPMeta struct {
    Meta              map[string]any `json:"_meta,omitempty"`
    StructuredContent map[string]any `json:"structuredContent,omitempty"`
}

// Message 是追加到对话历史的消息（简化表示，完整类型在 types/message 包）。
type Message struct {
    Role    string          // "user" | "assistant"
    Content json.RawMessage // Anthropic API 消息内容块数组
}

// ─────────────────────────────────────────────
// 工具执行上下文
// ─────────────────────────────────────────────

// UseContext 包含工具执行时所需的全部环境信息。
// 引擎在每次 Call 时构造并传入，工具不应长期持有此引用。
type UseContext struct {
    // 基础配置
    MainLoopModel    string
    Debug            bool
    Verbose          bool
    IsNonInteractive bool

    // 工具集合（只读，用于 SubAgent 等需要递归调用的场景）
    Tools []Tool

    // 权限上下文（只读）
    PermCtx PermissionContext

    // 消息历史（只读快照）
    Messages []Message

    // 文件读取限制
    FileReadLimits *FileReadLimits

    // Glob 搜索限制
    GlobLimits *GlobLimits

    // 取消信号
    Ctx context.Context

    // 进度推送（由引擎注入）
    OnProgress OnProgressFn

    // 当前工具调用 ID（Anthropic API 的 tool_use_id）
    ToolUseID string

    // 子 Agent 标识（仅 SubAgent 场景下有值）
    AgentID   string
    AgentType string

    // 内部状态更新回调（引擎注入）
    SetInProgressToolIDs func(func(map[string]struct{}) map[string]struct{})
}

// FileReadLimits 限制文件读取大小。
type FileReadLimits struct {
    MaxTokens    int
    MaxSizeBytes int
}

// GlobLimits 限制 glob 搜索结果数量。
type GlobLimits struct {
    MaxResults int
}

// ─────────────────────────────────────────────
// ValidationResult
// ─────────────────────────────────────────────

// ValidationResult 是 ValidateInput 的返回值。
type ValidationResult struct {
    OK        bool
    Message   string // 失败原因（OK=false 时必填）
    ErrorCode int    // 可选的错误码（供模型参考）
}

// ─────────────────────────────────────────────
// 核心 Tool 接口 ⭐
// ─────────────────────────────────────────────

// Tool 是所有工具必须实现的接口。
// 方法分为五组：身份/元数据、并发/安全性、权限、执行、序列化。
type Tool interface {
    // ── 身份与元数据 ──────────────────────────────────────

    // Name 返回工具的主名称（唯一，模型使用此名调用）。
    Name() string

    // Aliases 返回向后兼容的别名列表（可为 nil）。
    Aliases() []string

    // Description 返回工具描述（用于 prompt 注入）。
    // input 为当前调用参数（可能为空），用于动态描述。
    Description(input Input, permCtx PermissionContext) string

    // InputSchema 返回工具输入的 JSON Schema。
    InputSchema() InputSchema

    // Prompt 返回注入系统提示的工具说明文本。
    // 大多数工具无需实现，返回空字符串即可；
    // 复杂工具（Bash/Edit）用此方法注入详细使用规范。
    Prompt(ctx context.Context, permCtx PermissionContext) (string, error)

    // MaxResultSizeChars 返回输出写入对话历史前的最大字符数。
    // 超出时引擎将结果持久化到磁盘，并向模型发送预览+路径。
    // 返回 -1 表示不限制（Read 等工具使用，自行控制大小）。
    MaxResultSizeChars() int

    // SearchHint 返回关键词提示（ToolSearch 用，3-10 词）。
    // 不需要可返回空字符串。
    SearchHint() string

    // ── 并发与安全性 ──────────────────────────────────────

    // IsConcurrencySafe 判断此输入是否可与其他工具并发执行。
    // true = 只读操作（可并发）；false = 写操作（需串行）。
    IsConcurrencySafe(input Input) bool

    // IsReadOnly 判断此操作是否只读（不修改任何外部状态）。
    IsReadOnly(input Input) bool

    // IsDestructive 判断此操作是否不可逆（删除/覆盖/发送等）。
    // 默认 false；实现不可逆操作的工具必须返回 true。
    IsDestructive(input Input) bool

    // IsEnabled 返回当前工具是否可用（依赖 feature flag 等动态条件）。
    IsEnabled() bool

    // InterruptBehavior 返回用户发送新消息时工具的中断行为。
    // "cancel" = 停止工具并丢弃结果；"block" = 继续执行，新消息等待。
    InterruptBehavior() string // "cancel" | "block"

    // ── 权限 ─────────────────────────────────────────────

    // ValidateInput 在权限检查前验证输入合法性。
    // 失败时引擎向模型返回错误消息，不弹权限窗口。
    // 无需验证时返回 ValidationResult{OK: true}。
    ValidateInput(input Input, ctx *UseContext) (ValidationResult, error)

    // CheckPermissions 对给定输入执行工具级权限检查。
    // 仅在 ValidateInput 通过后调用。
    // 通用权限逻辑由 permissions 包处理；此方法处理工具特有逻辑。
    CheckPermissions(input Input, ctx *UseContext) (PermissionResult, error)

    // PreparePermissionMatcher 为 hook if 条件（"git *" 等模式）准备匹配器。
    // 返回闭包供 hooks/permissions 包调用。不需要时返回 nil。
    PreparePermissionMatcher(input Input) (func(pattern string) bool, error)

    // ── 执行 ─────────────────────────────────────────────

    // Call 执行工具并返回结果。
    // onProgress 可为 nil（非交互场景）；工具应容忍 nil 回调。
    Call(
        input      Input,
        ctx        *UseContext,
        onProgress OnProgressFn,
    ) (*Result, error)

    // ── 序列化 ───────────────────────────────────────────

    // MapResultToToolResultBlock 将工具输出序列化为 Anthropic API 的
    // tool_result 消息块内容（[]ContentBlock 的 JSON）。
    MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error)

    // ToAutoClassifierInput 返回安全分类器使用的简化表示。
    // 无安全关联时返回空字符串（跳过分类器）。
    ToAutoClassifierInput(input Input) string

    // UserFacingName 返回 UI 展示用的工具名称（含参数摘要）。
    UserFacingName(input Input) string

    // GetPath 返回此工具操作的文件路径（仅文件操作工具实现）。
    // 不适用时返回空字符串。
    GetPath(input Input) string
}

// ─────────────────────────────────────────────
// 辅助类型
// ─────────────────────────────────────────────

// MCPInfo 记录 MCP 工具的原始服务器/工具名。
type MCPInfo struct {
    ServerName string
    ToolName   string
}

// SearchOrReadResult 描述工具调用是否为搜索/读取类操作（用于 UI 折叠）。
type SearchOrReadResult struct {
    IsSearch bool
    IsRead   bool
    IsList   bool
}
```

### 1.3 接口设计说明

| 方法 | TS 对应 | 必须实现 | 说明 |
|------|---------|---------|------|
| `Name()` | `name` 字段 | ✅ | 工具唯一标识 |
| `IsConcurrencySafe()` | `isConcurrencySafe()` | ✅ | 控制并发策略 |
| `IsReadOnly()` | `isReadOnly()` | ✅ | 用于权限推断 |
| `IsDestructive()` | `isDestructive?()` | 可选实现，默认 false | |
| `CheckPermissions()` | `checkPermissions()` | ✅ | 工具级权限 |
| `ValidateInput()` | `validateInput?()` | 可返回 OK=true | |
| `Call()` | `call()` | ✅ | 核心执行方法 |
| `Prompt()` | `prompt()` | 可返回空字符串 | 系统提示注入 |

---

## 2. internal/engine — 查询引擎

### 2.1 包结构

```
internal/engine/
├── engine.go          # QueryEngine 接口 + 构造函数
├── loop.go            # 主循环状态机
├── orchestrator.go    # 工具并发编排
├── token_budget.go    # Token 预算管理
├── message.go         # 消息历史管理 + normalizeForAPI
└── msg.go             # 引擎 → TUI 的 Msg 类型定义
```

### 2.2 QueryEngine 接口

```go
// Package engine 实现 LLM 主循环、工具编排、消息历史管理。
package engine

import (
    "context"

    "github.com/tunsuytang/claude-code-go/internal/tool"
)

// QueryEngine 是引擎对外（TUI 层）的接口。
// TUI 通过此接口发起对话、接收流式事件。
type QueryEngine interface {
    // Query 发起一次 LLM 查询，返回事件流 channel。
    // 调用方读完 channel 直到关闭（或 ctx 取消）。
    // 每次 Query 对应一个完整的 agentic turn（可能包含多轮工具调用）。
    Query(ctx context.Context, params QueryParams) (<-chan EngineMsg, error)

    // Abort 中止当前正在运行的 Query（幂等）。
    Abort()

    // History 返回当前会话的消息历史（只读快照）。
    History() []tool.Message

    // AppendUserMessage 向历史追加一条 user 消息（用户输入）。
    AppendUserMessage(content string) error

    // ResetHistory 清空消息历史（compact 后重建使用）。
    ResetHistory(msgs []tool.Message)
}

// QueryParams 是 Query 调用的入参。
type QueryParams struct {
    // 系统提示（已渲染，含动态工具描述）
    SystemPrompt string

    // 用户上下文（注入到第一条 user 消息的额外 kv）
    UserContext map[string]string

    // 工具集合
    Tools []tool.Tool

    // 主模型名称
    Model string

    // 回退模型（主模型不可用时使用）
    FallbackModel string

    // 最大对话轮数（0 = 不限制）
    MaxTurns int

    // 最大输出 token 数（0 = 使用模型默认值）
    MaxOutputTokens int

    // Token 任务预算（总量，跨 compact 边界持续递减）
    TaskBudget *TokenBudget
}

// TokenBudget 描述 token 任务预算。
type TokenBudget struct {
    Total int
}
```

### 2.3 引擎 → TUI 的 Msg 类型

```go
// EngineMsg 是引擎通过 channel 推送给 TUI 的事件联合类型。
// TUI 使用类型断言（或 switch）分发处理。
type EngineMsg interface {
    engineMsg() // 私有方法，防止外部实现
}

// ─── 流式文本事件 ───────────────────────────────────────

// TextDeltaMsg 是 LLM 文本流式增量片段。
type TextDeltaMsg struct {
    Delta string
}

// ThinkingDeltaMsg 是 extended thinking 的流式增量片段。
type ThinkingDeltaMsg struct {
    Delta string
}

// ─── 工具调用生命周期 ────────────────────────────────────

// ToolUseStartMsg 表示工具调用开始（参数可能未完全流入）。
type ToolUseStartMsg struct {
    ToolUseID string
    ToolName  string
    InputSoFar tool.Input // 已流入的部分参数（流式时可能不完整）
}

// ToolUseInputDeltaMsg 是工具输入的流式增量（参数边流边显示）。
type ToolUseInputDeltaMsg struct {
    ToolUseID  string
    InputDelta string
}

// ToolUseProgressMsg 是工具执行中的进度事件。
type ToolUseProgressMsg struct {
    tool.ProgressData
}

// ToolUseCompleteMsg 表示工具执行完成。
type ToolUseCompleteMsg struct {
    ToolUseID string
    ToolName  string
    Input     tool.Input
    Result    *tool.Result
    IsError   bool
    Error     error
}

// ToolUseRejectedMsg 表示工具调用被用户拒绝。
type ToolUseRejectedMsg struct {
    ToolUseID string
    ToolName  string
    Input     tool.Input
    Reason    string
}

// ─── 权限请求 ────────────────────────────────────────────

// PermissionAskMsg 表示需要用户确认权限。
// TUI 收到后展示确认 UI，用户决策后通过 PermissionResponseCh 回传。
type PermissionAskMsg struct {
    ToolUseID          string
    ToolName           string
    Input              tool.Input
    Message            string
    Suggestions        []string
    PermissionResponseCh chan<- PermissionResponse
}

// PermissionResponse 是用户对权限请求的响应。
type PermissionResponse struct {
    Allow        bool
    AlwaysAllow  bool   // 写入 always-allow 规则
    UpdatedInput tool.Input
}

// ─── 会话状态 ─────────────────────────────────────────────

// AssistantMessageCompleteMsg 表示一条完整的 assistant 消息生成完毕。
type AssistantMessageCompleteMsg struct {
    MessageID string
}

// TurnCompleteMsg 表示整个 agentic turn 结束（含所有工具调用）。
type TurnCompleteMsg struct {
    StopReason string // "end_turn" | "max_tokens" | "tool_use" | ...
}

// ErrorMsg 表示不可恢复的错误。
type ErrorMsg struct {
    Err error
}

// CompactStartMsg 表示上下文压缩开始。
type CompactStartMsg struct {
    Strategy string // "auto" | "micro" | "snip"
}

// CompactEndMsg 表示上下文压缩完成。
type CompactEndMsg struct {
    Strategy   string
    TokensSaved int
}

// TokenWarningMsg 表示 token 使用量接近上限。
type TokenWarningMsg struct {
    CurrentTokens int
    MaxTokens     int
    WarningLevel  string // "warning" | "critical"
}

// 私有方法实现（让各 Msg 类型满足 EngineMsg 接口）
func (TextDeltaMsg) engineMsg()              {}
func (ThinkingDeltaMsg) engineMsg()          {}
func (ToolUseStartMsg) engineMsg()           {}
func (ToolUseInputDeltaMsg) engineMsg()      {}
func (ToolUseProgressMsg) engineMsg()        {}
func (ToolUseCompleteMsg) engineMsg()        {}
func (ToolUseRejectedMsg) engineMsg()        {}
func (PermissionAskMsg) engineMsg()          {}
func (AssistantMessageCompleteMsg) engineMsg() {}
func (TurnCompleteMsg) engineMsg()           {}
func (ErrorMsg) engineMsg()                 {}
func (CompactStartMsg) engineMsg()          {}
func (CompactEndMsg) engineMsg()            {}
func (TokenWarningMsg) engineMsg()          {}
```

### 2.4 主循环状态机设计

```
┌─────────────────────────────────────────────────────────────┐
│                         queryLoop                           │
│                                                             │
│  ┌──────────┐    API     ┌────────────┐   工具调用          │
│  │ SAMPLING │──────────>│ PROCESSING │──────────────────>  │
│  └──────────┘  streaming └────────────┘                     │
│       ↑                       │                            │
│       │      end_turn         │  工具执行完成              │
│       └───────────────────────┘                            │
│                               │  stop_sequence / max_turns │
│                               ↓                            │
│                        ┌──────────┐                        │
│                        │ TERMINAL │                        │
│                        └──────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

**状态转移触发条件**：

| 当前状态 | 事件 | 下一状态 | 说明 |
|---------|------|---------|------|
| `SAMPLING` | API 返回 `tool_use` | `PROCESSING` | 执行工具批次 |
| `SAMPLING` | API 返回 `end_turn` | `TERMINAL` | 正常结束 |
| `SAMPLING` | 上下文超限 | `COMPACT` | 触发压缩 |
| `PROCESSING` | 所有工具完成 | `SAMPLING` | 继续采样 |
| `PROCESSING` | 用户拒绝工具 | `SAMPLING` | 注入拒绝消息后继续 |
| `COMPACT` | 压缩完成 | `SAMPLING` | 用压缩后消息重采样 |

### 2.5 goroutine 架构

```
main goroutine (queryLoop)
    │
    ├─ 调用 Anthropic API（stream）
    │      ↓ 收到 tool_use blocks
    ├─ 调用 runTools(toolUseBlocks)
    │      │
    │      ├─ [isConcurrencySafe=true] 并发 goroutine 池（最多 10）
    │      │       每个 goroutine 执行一个只读工具
    │      │       通过 channel 汇聚结果
    │      │
    │      └─ [isConcurrencySafe=false] 串行执行
    │              每个写工具顺序执行
    │
    └─ 将 EngineMsg 推送到 outCh（有缓冲）
```

### 2.6 工具并发编排策略

**分批算法**（对应 TS `partitionToolCalls`）：

```go
// partitionToolCalls 将工具调用列表分割为并发安全批次和串行批次的交替序列。
// 规则：
//   - 连续的 isConcurrencySafe=true 工具合并为一个并发批次
//   - 遇到 isConcurrencySafe=false 工具，独立为一个串行批次
//   - 批次间严格有序：前一批次全部完成才执行后一批次
//
// 示例：[Read, Read, Write, Read, Write]
//   -> [{concurrent: [Read, Read]}, {serial: [Write]}, {concurrent: [Read]}, {serial: [Write]}]

type toolBatch struct {
    IsConcurrent bool
    Blocks       []ToolUseBlock
}
```

**并发限制**：默认最多 10 个并发工具（`CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY` 环境变量可覆盖）。

### 2.7 Token 预算管理

```go
// BudgetTracker 跟踪 token 使用量，触发 compact 决策。
type BudgetTracker struct {
    // 当前会话累计 token 使用量
    UsedTokens int

    // 上下文窗口大小（由模型决定）
    ContextWindow int

    // auto-compact 触发阈值 = ContextWindow - AUTOCOMPACT_BUFFER_TOKENS（13,000）
    AutoCompactThreshold int

    // 警告阈值 = ContextWindow - WARNING_THRESHOLD_BUFFER_TOKENS（20,000）
    WarningThreshold int

    // 连续 compact 失败次数（超过 3 次停止 auto-compact，防止无限循环）
    ConsecutiveFailures int
}
```

---

## 3. internal/permissions — 权限系统

### 3.1 包结构

```
internal/permissions/
├── checker.go      # Checker 接口
├── rules.go        # 规则匹配算法
├── ask.go          # ask-user channel 协议
└── denial.go       # 权限拒绝历史记录
```

### 3.2 Checker 接口

```go
// Package permissions 实现工具调用的权限检查系统。
package permissions

import (
    "github.com/tunsuytang/claude-code-go/internal/tool"
)

// Checker 是权限检查的核心接口。
// 引擎在每次工具调用前调用 Check，获得 allow/deny/ask 决策。
type Checker interface {
    // Check 对工具调用执行完整权限检查（规则匹配 + 工具自检）。
    // 检查顺序：alwaysDeny → alwaysAllow → alwaysAsk → tool.CheckPermissions → mode 默认值
    Check(
        toolName string,
        input     tool.Input,
        ctx       *tool.UseContext,
        t         tool.Tool,
    ) (tool.PermissionResult, error)

    // UpdateContext 替换权限上下文（用户确认 always-allow 时更新规则）。
    UpdateContext(ctx tool.PermissionContext)

    // GetContext 返回当前权限上下文的只读快照。
    GetContext() tool.PermissionContext
}
```

### 3.3 规则匹配算法

```
权限决策流程（三级优先级）：

输入: toolName, input, permCtx

1. 检查 alwaysDenyRules
   │ 匹配 → DENY（立即返回，不再检查）
   ↓
2. 检查 alwaysAllowRules
   │ 匹配 → ALLOW（立即返回）
   ↓
3. 检查 alwaysAskRules
   │ 匹配 → ASK（触发用户确认流程）
   ↓
4. 调用 tool.CheckPermissions()（工具自定义逻辑）
   │ deny/ask → 使用工具决策
   │ allow / passthrough → 继续
   ↓
5. 根据 PermissionMode 决定默认行为：
   - bypassPermissions  → ALLOW
   - acceptEdits        → ALLOW（仅 Edit 类工具）
   - dontAsk            → ALLOW
   - plan               → DENY（禁止写操作）
   - default            → ASK（弹出确认 UI）
   - auto               → 委托分类器判断
```

**模式匹配规则格式**：`ToolName(pattern)`

```
例子：
  "Bash"          → 匹配所有 Bash 调用
  "Bash(git *)"   → 匹配 git 开头的 Bash 命令
  "Edit(src/*)"   → 匹配 src/ 下的编辑操作
```

### 3.4 ask-user channel 协议

```
权限 ASK 流程：

engine/loop.go                    permissions/ask.go         TUI
     │                                    │                   │
     │──── Check() → ASK ──────────────>  │                   │
     │                                    │──PermissionAskMsg→ │
     │                              (阻塞等待用户响应)         │
     │                                    │ <─PermissionResp── │
     │ <── PermissionResult(allow/deny) ──│                   │
     │                                    │                   │

实现：
  - permissions 包暴露 AskCh chan PermissionAskMsg
  - 引擎将 AskCh 桥接到 outCh（转为 PermissionAskMsg 推送给 TUI）
  - TUI 展示确认 UI 后写入 PermissionAskMsg.PermissionResponseCh
  - permissions 从 ResponseCh 读取结果，返回最终 PermissionResult
  - 超时（默认 60s）自动 deny（非交互场景防止死锁）
```

### 3.5 权限拒绝历史记录

```go
// DenialTrackingState 记录工具调用被拒绝的历史。
// 用于：连续拒绝超阈值时自动降级到 ask（防止 auto-approve 失控）。
type DenialTrackingState struct {
    // 按工具名分组的拒绝计数
    CountByTool map[string]int

    // 最近一次拒绝时间（Unix ms）
    LastDenialMs int64

    // 连续拒绝计数（任意工具）
    ConsecutiveDenials int
}
```

---

## 4. internal/compact — 上下文压缩

### 4.1 包结构

```
internal/compact/
├── compressor.go   # Compressor 接口
├── auto.go         # Auto-compact 策略
├── micro.go        # Micro-compact 策略
└── snip.go         # Snip-compact 策略
```

### 4.2 Compressor 接口

```go
// Package compact 实现三种上下文压缩策略。
package compact

import (
    "context"
    "github.com/tunsuytang/claude-code-go/internal/tool"
)

// Strategy 表示压缩策略类型。
type Strategy string

const (
    StrategyAuto  Strategy = "auto"
    StrategyMicro Strategy = "micro"
    StrategySnip  Strategy = "snip"
)

// CompactionResult 是压缩操作的返回值。
type CompactionResult struct {
    // 压缩后的消息列表（替换原历史）
    Messages []tool.Message

    // 压缩摘要（注入为新的 system/user 消息）
    Summary string

    // 节省的 token 数（估算）
    TokensSaved int
}

// Compressor 是压缩策略的统一接口。
type Compressor interface {
    // Strategy 返回此 Compressor 实现的策略名称。
    Strategy() Strategy

    // ShouldCompact 判断当前状态是否需要压缩。
    ShouldCompact(messages []tool.Message, usedTokens, contextWindow int, model string) bool

    // Compact 执行压缩，返回压缩后的消息列表。
    Compact(ctx context.Context, params CompactParams) (*CompactionResult, error)
}

// CompactParams 是压缩调用的参数。
type CompactParams struct {
    Messages    []tool.Message
    Model       string
    SystemPrompt string
    // 压缩前的 token 使用量
    UsedTokens  int
    // 上下文窗口大小
    ContextWindow int
}
```

### 4.3 Auto-compact 策略

**触发条件**：`usedTokens > contextWindow - 13,000`

**算法**：
1. 调用 LLM 对完整对话生成结构化摘要（`~4,000 token` 输出）
2. 用单条系统消息 + 摘要替换历史所有消息
3. 保留最后 N 条 user/assistant 消息（滑动窗口）
4. 插入 compact boundary 标记消息
5. 连续失败超 3 次（`MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES`）停止尝试

**触发后行为**：
- 推送 `CompactStartMsg` → 执行压缩 → 推送 `CompactEndMsg`
- 压缩失败时：推送 `ErrorMsg`，降级为继续使用旧历史（不中断对话）

### 4.4 Micro-compact 策略

**触发条件**：按时间/轮次触发（`TimeBasedMCConfig`），非 token 超限触发。

**算法**（选择性压缩，TS `microCompact.ts`）：
- 仅压缩特定工具的输出（Bash/Shell/FileRead/Grep/Glob/WebSearch/WebFetch/FileEdit/FileWrite）
- 对大型工具结果（超过 `IMAGE_MAX_TOKEN_SIZE=2000 token`）进行截断/摘要
- 保留工具调用结构，只替换过大的 `tool_result` 内容
- 比 Auto-compact 更轻量，不调用 LLM，纯规则处理

### 4.5 Snip-compact 策略

**触发条件**：`feature("HISTORY_SNIP")` 开启时，作为 Auto-compact 的补充。

**算法**：
- 识别历史中可"剪切"的片段（重复的只读工具结果、冗长的搜索输出）
- 用占位符 `[Content snipped - N tokens]` 替换
- 不调用 LLM，O(n) 扫描复杂度
- 优先剪切最旧、最大、与当前任务关联度最低的片段

---

## 5. internal/hooks — Hooks 系统

### 5.1 包结构

```
internal/hooks/
├── types.go     # Hook 类型枚举 + 数据结构
├── executor.go  # Executor 接口
├── runner.go    # 执行引擎（超时、重试、聚合）
└── events.go    # 各阶段事件类型
```

### 5.2 Hook 类型枚举

```go
// Package hooks 实现 Claude Code Hooks 协议。
// Hooks 是外部进程，通过 stdin/stdout JSON 与引擎交互。
package hooks

// HookEvent 是 hook 触发的事件类型。
type HookEvent string

const (
    HookEventPreToolUse       HookEvent = "PreToolUse"
    HookEventPostToolUse      HookEvent = "PostToolUse"
    HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
    HookEventPreSampling      HookEvent = "PreSampling"     // TS: UserPromptSubmit
    HookEventSessionStart     HookEvent = "SessionStart"
    HookEventSubagentStart    HookEvent = "SubagentStart"
    HookEventNotification     HookEvent = "Notification"
    HookEventPermissionDenied HookEvent = "PermissionDenied"
    HookEventPermissionRequest HookEvent = "PermissionRequest"
    HookEventCwdChanged       HookEvent = "CwdChanged"
    HookEventFileChanged      HookEvent = "FileChanged"
    HookEventWorktreeCreate   HookEvent = "WorktreeCreate"
)
```

### 5.3 Executor 接口

```go
// Executor 负责执行 hooks 并聚合结果。
type Executor interface {
    // RunPreToolUse 在工具调用前执行 PreToolUse hooks。
    // 返回聚合结果：可能修改 input，可能 block 工具调用。
    RunPreToolUse(
        ctx       context.Context,
        toolName  string,
        toolUseID string,
        input     json.RawMessage,
    ) (*AggregatedHookResult, error)

    // RunPostToolUse 在工具调用后执行 PostToolUse hooks。
    RunPostToolUse(
        ctx       context.Context,
        toolName  string,
        toolUseID string,
        input     json.RawMessage,
        output    json.RawMessage,
        isError   bool,
    ) (*AggregatedHookResult, error)

    // RunPreSampling 在 LLM 采样前执行（对应 UserPromptSubmit）。
    RunPreSampling(
        ctx           context.Context,
        userMessage   string,
        sessionID     string,
    ) (*AggregatedHookResult, error)

    // RunSessionStart 在会话启动时执行。
    RunSessionStart(ctx context.Context, sessionID string) (*AggregatedHookResult, error)

    // RunPermissionRequest 执行 PermissionRequest hooks（替代用户弹窗）。
    RunPermissionRequest(
        ctx       context.Context,
        toolName  string,
        toolUseID string,
        input     json.RawMessage,
        message   string,
    ) (*AggregatedHookResult, error)
}

// AggregatedHookResult 是多个 hook 执行结果的聚合。
type AggregatedHookResult struct {
    // PreventContinuation: hooks 投票阻止继续（等同于用户中断）
    PreventContinuation bool
    StopReason          string

    // PermissionBehavior: PreToolUse hooks 对权限的投票
    PermissionBehavior string // "allow" | "deny" | "ask" | "passthrough"
    PermissionDecisionReason string

    // UpdatedInput: hooks 修改后的工具输入
    UpdatedInput json.RawMessage

    // AdditionalContexts: hooks 注入的额外上下文（追加到 system prompt）
    AdditionalContexts []string

    // BlockingErrors: 阻塞性错误（展示给用户）
    BlockingErrors []HookBlockingError

    // 系统消息（注入对话历史）
    SystemMessage *tool.Message
}

// HookBlockingError 表示 hook 返回的阻塞性错误。
type HookBlockingError struct {
    Error   string
    Command string
}
```

### 5.4 各阶段说明

| 阶段 | TS 事件 | 时机 | 主要用途 |
|------|---------|------|---------|
| `PreToolUse` | `PreToolUse` | 工具执行前（权限检查后） | 修改 input、拦截调用、自定义权限 |
| `PostToolUse` | `PostToolUse` | 工具成功返回后 | 修改输出、触发副作用、记录日志 |
| `PostToolUseFailure` | `PostToolUseFailure` | 工具执行失败后 | 错误上报、重试决策 |
| `PreSampling` | `UserPromptSubmit` | 用户消息发送、LLM 采样前 | 注入上下文、消息过滤 |
| `SessionStart` | `SessionStart` | 会话初始化时 | 环境准备、注入初始消息 |
| `PermissionRequest` | `PermissionRequest` | 权限弹窗替代 | 自动化权限决策（CI/CD 场景） |

**Hook 执行协议（进程通信）**：

```
引擎                    Hook 进程（外部可执行文件）
  │                           │
  │ stdin: JSON (HookInput)   │
  │ ─────────────────────────>│
  │                           │ 处理（可写 stdout 进度）
  │     stdout: JSON (HookJSONOutput)
  │ <─────────────────────────│
  │ 超时: hookTimeout（秒）    │
```

---

## 6. internal/coordinator — 多 Agent 协调

### 6.1 包结构

```
internal/coordinator/
├── coordinator.go   # Coordinator 接口
├── subagent.go      # 子 Agent 生命周期管理
├── swarm.go         # Swarm 通信协议
└── prompt.go        # 协调模式系统提示注入
```

### 6.2 Coordinator 接口

```go
// Package coordinator 实现多 Agent 协调能力。
// 对应 TS src/hooks/coordinatorMode.ts 中的 Swarm/SubAgent 机制。
package coordinator

import (
    "context"
    "github.com/tunsuytang/claude-code-go/internal/tool"
)

// Coordinator 管理子 Agent 的创建、通信和生命周期。
type Coordinator interface {
    // SpawnSubAgent 创建并启动一个子 Agent。
    // 子 Agent 在独立 goroutine 中运行，通过 channel 通信。
    SpawnSubAgent(ctx context.Context, params SubAgentParams) (*SubAgentHandle, error)

    // SendMessage 向指定 Agent 发送消息（路由协议）。
    // agentID 为空时发送给主 Agent（main thread）。
    SendMessage(agentID string, msg AgentMessage) error

    // GetAgentStatus 返回子 Agent 的当前状态。
    GetAgentStatus(agentID string) (AgentStatus, error)

    // AbortSubAgent 中止指定子 Agent。
    AbortSubAgent(agentID string) error

    // WaitAll 等待所有子 Agent 完成（或 ctx 取消）。
    WaitAll(ctx context.Context) error
}

// SubAgentParams 是创建子 Agent 的参数。
type SubAgentParams struct {
    // AgentType: 子 Agent 类型（对应 --agent CLI 参数）
    AgentType string

    // InitialMessage: 子 Agent 的初始 user 消息
    InitialMessage string

    // Tools: 子 Agent 可用的工具集合
    Tools []tool.Tool

    // SystemPromptAppend: 追加到子 Agent 系统提示的内容
    SystemPromptAppend string

    // MaxTurns: 子 Agent 最大对话轮数
    MaxTurns int

    // InheritPermissions: 是否继承父 Agent 的权限上下文
    InheritPermissions bool

    // ShouldAvoidPermissionPrompts: 是否禁止弹权限窗口
    // true = 自动 deny 所有需要确认的操作（后台 Agent 使用）
    ShouldAvoidPermissionPrompts bool
}

// SubAgentHandle 是子 Agent 的句柄，用于通信和控制。
type SubAgentHandle struct {
    // AgentID: 子 Agent 唯一标识（UUID）
    AgentID string

    // MsgCh: 从子 Agent 接收事件的 channel（只读）
    MsgCh <-chan AgentEvent

    // Abort: 中止子 Agent 的函数
    Abort func()
}

// AgentMessage 是发送给 Agent 的消息。
type AgentMessage struct {
    Role    string // "user"
    Content string
}

// AgentEvent 是子 Agent 推送的事件（与 EngineMsg 类似，但含 AgentID）。
type AgentEvent struct {
    AgentID string
    Event   interface{} // 具体类型与 engine.EngineMsg 相同
}

// AgentStatus 表示子 Agent 的当前状态。
type AgentStatus struct {
    AgentID   string
    State     string // "running" | "completed" | "aborted" | "error"
    TurnCount int
    Error     error
}
```

### 6.3 子 Agent 生命周期

```
创建                    运行                      结束
  │                      │                         │
SpawnSubAgent()      goroutine 内               completed/
  │                  queryLoop 运行              aborted/error
  │                      │                         │
  ↓                      ↓                         ↓
SubAgentHandle    推送 AgentEvent         MsgCh 关闭
  .MsgCh                到 MsgCh           WaitAll() 返回
```

**父子 Agent 权限隔离**：
- `ShouldAvoidPermissionPrompts=true`：后台子 Agent 不弹权限窗口，自动 deny
- `InheritPermissions=false`：子 Agent 从零开始权限规则（沙箱隔离）
- `InheritPermissions=true`：子 Agent 复用父 Agent 的 always-allow 规则

### 6.4 SendMessage 路由协议

```
消息路由规则：
  agentID = ""        → 主 Agent（main thread QueryEngine）
  agentID = "xxx"     → 对应子 Agent（通过 Coordinator 查找）
  agentID = "swarm"   → Swarm 广播（所有活跃子 Agent）

Swarm 通信模式（对应 TS useSwarmInitialization.ts）：
  - 主 Agent 作为协调者（Orchestrator）
  - 子 Agent 作为工作者（Worker）
  - Worker 通过 SendMessage 向 Orchestrator 报告进度
  - Orchestrator 通过 SpawnSubAgent + SendMessage 分发任务
```

### 6.5 协调模式系统提示注入

针对多 Agent 协调场景，引擎在系统提示中注入额外说明：

```
[Coordinator Mode]
你是一个协调者 Agent。你可以使用以下工具来管理子 Agent：
- Task(description, prompt): 创建一个子 Agent 执行具体任务

子 Agent 规则：
1. 子 Agent 完成后，结果通过 tool_result 返回
2. 多个子 Agent 可并行执行（Task 工具 isConcurrencySafe=true）
3. 避免让子 Agent 执行破坏性操作（除非显式授权）
```

---

## 7. 设计决策

### 7.1 Tool 接口：Go 接口 vs TS 鸭子类型

**问题**：TS `Tool` 是一个带可选方法的大型对象类型，Go 接口不支持可选方法。

**决策**：
- **必需方法**：直接定义在 `Tool` 接口中（`Name`、`Call`、`CheckPermissions` 等）
- **可选方法**：通过子接口扩展（`SearchOrReadTool`、`PathTool`、`GroupRenderTool`）
- 引擎在运行时用 `interface{}` 类型断言检查是否实现可选接口
- 这比 TS 的 `?.` 可选调用更显式，避免遗漏实现

```go
// 可选接口示例
type PathTool interface {
    Tool
    GetPath(input Input) string
}

// 引擎调用示例
if pt, ok := t.(PathTool); ok {
    path = pt.GetPath(input)
}
```

### 7.2 EngineMsg：channel vs callback

**问题**：TS 使用 `AsyncGenerator` 流式推送事件，Go 如何对应？

**决策**：使用带缓冲 channel（`chan EngineMsg`）
- 容量：256（防止工具密集期背压导致引擎阻塞）
- TUI 在独立 goroutine 消费 channel（Bubble Tea 的 `tea.Cmd` 包装）
- 引擎关闭 channel 表示 turn 结束（TUI 检测到 channel 关闭后更新状态）

### 7.3 权限 ask 流程：同步阻塞 vs 异步

**问题**：TS 使用 React hook（`useCanUseTool`）在 UI 线程处理权限；Go 如何在引擎 goroutine 等待 TUI 响应？

**决策**：双向 channel 阻塞
- 引擎推送 `PermissionAskMsg`（含 `chan<- PermissionResponse`）到 outCh
- 引擎 goroutine 阻塞在 `<-responseCh`
- TUI goroutine 展示确认 UI，用户操作后写入 responseCh
- 引擎恢复执行
- 超时（60s）自动写入 deny response，防止死锁

### 7.4 上下文压缩：同步 vs 异步

**决策**：同步执行（在主循环内）
- Auto-compact 在检测到 token 超限后，暂停 sampling，执行压缩，再重启
- 压缩期间推送 `CompactStartMsg`/`CompactEndMsg` 供 TUI 展示进度
- 压缩失败不中断对话（降级跳过，记录连续失败次数，超限停止重试）

---

## 8. TS → Go 行为映射

| TS 概念 | TS 实现 | Go 对应 |
|---------|---------|---------|
| `Tool` object type | 鸭子类型对象 | `tool.Tool` interface |
| `tool.call()` | async function | `Tool.Call()` 返回 `(*Result, error)` |
| `AsyncGenerator<StreamEvent>` | `yield` 流式推送 | `chan EngineMsg`（带缓冲） |
| `ToolUseContext` | 大型 context 对象 | `tool.UseContext` struct |
| `CanUseToolFn` | 权限检查函数 | `permissions.Checker` interface |
| `runTools()` | async generator | `engine.runTools()` + goroutine 池 |
| `partitionToolCalls()` | 并发分组 | `engine.partitionToolCalls()` |
| `autoCompact` service | module | `compact.AutoCompactor` struct |
| `microCompact` service | module | `compact.MicroCompactor` struct |
| `snipCompact` service | module | `compact.SnipCompactor` struct |
| Hook JSON protocol | subprocess + JSON | `hooks.Executor` + os/exec |
| `coordinatorMode.ts` | React hooks | `coordinator.Coordinator` interface |
| `SubAgent` | async context clone | goroutine + `SubAgentHandle` |
| `isConcurrencySafe` | boolean method | `Tool.IsConcurrencySafe()` |
| `PermissionMode` | string union | `tool.PermissionMode` string type |
| `PermissionResult` | union type | `tool.PermissionResult` struct |
| `feature()` flags | bun:bundle | `config.Feature()` / 编译时常量 |
| `AbortController` | Web API | `context.Context` + `context.CancelFunc` |
| `z.ZodType` schema | Zod v4 | `json.RawMessage` + `jsonschema` tag |

### 8.1 关键行为差异说明

1. **错误处理**：TS 使用 `throw`，Go 使用 `(result, error)` 二元组；引擎对 `error != nil` 推送 `ErrorMsg`。

2. **React 渲染**：TS `Tool` 包含大量 `render*` 方法（返回 `React.ReactNode`）；Go 工具**不包含渲染逻辑**，渲染由 TUI 层按工具名分发到独立渲染器。

3. **Zod Schema**：TS 用 Zod 实现运行时参数验证；Go 使用 `encoding/json` + 手动验证（`ValidateInput` 方法），或引入 `github.com/invopop/jsonschema` 生成 schema。

4. **feature flags**：TS `feature('REACTIVE_COMPACT')` 在打包时消除死代码；Go 使用 `sync.OnceValue` + 环境变量/配置文件，运行时决策（不做编译时消除）。

5. **MCP 工具**：TS MCP 工具动态注入，`isMcp=true`；Go MCP 工具实现 `tool.Tool` 接口，通过 `MCPInfo()` 可选接口区分，引擎统一处理。
