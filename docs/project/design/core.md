# 核心层详细设计

> 负责 Agent：Agent-Core
> 状态：设计中
> 日期：2026-04-02

---

## 目录

1. [internal/tool — Tool 接口契约 ⭐](#1-internaltool--tool-接口契约-)
2. [internal/engine — 查询引擎](#2-internalengine--查询引擎)
3. [internal/permissions — 权限系统](#3-internalpermissions--权限系统)
4. [internal/compact — 上下文压缩](#4-internalcompact--上下文压缩)
5. [internal/hooks — Hooks 系统](#5-internalhooks--hooks-系统)
6. [internal/coordinator — 多 Agent 协调](#6-internalcoordinator--多-agent-协调)
7. [设计决策](#7-设计决策)
8. [TS → Go 行为映射](#8-ts--go-行为映射)

---

## 1. internal/tool — Tool 接口契约 ⭐

> **这是整个系统最重要的契约包**。Agent-Tools 层的所有工具实现必须满足此接口。
> 本包只定义接口和类型，**零业务逻辑**，**零外部依赖**。

### 包路径

```
internal/tool/
├── tool.go          # Tool 接口 + 核心类型
├── permission.go    # 权限相关类型
├── progress.go      # 进度推送类型
├── result.go        # 执行结果类型
└── context.go       # ToolUseContext
```

### 1.1 核心接口定义

```go
// tool.go

package tool

import (
    "context"
    "encoding/json"
)

// -----------------------------------------------------------------
// Tool 是所有工具的契约接口。
// Agent-Tools 层中每个工具都必须实现此接口。
// -----------------------------------------------------------------

// Tool 定义了一个可被 LLM 调用的工具的完整契约。
type Tool interface {
    // --- 元数据 ---

    // Name 返回工具的唯一名称，与 LLM API 中的 tool_use.name 一致。
    Name() string

    // Aliases 返回向后兼容的别名列表（工具改名时使用）。
    Aliases() []string

    // Description 根据当前输入和权限上下文动态生成工具描述。
    // 该描述会注入到 system prompt 中供 LLM 理解工具用途。
    Description(ctx context.Context, input json.RawMessage, opts DescriptionOptions) (string, error)

    // Prompt 生成注入到 system prompt 的工具说明文本。
    Prompt(ctx context.Context, opts PromptOptions) (string, error)

    // InputSchema 返回 JSON Schema（object 类型），用于向 LLM 声明参数格式。
    InputSchema() JSONSchema

    // UserFacingName 返回面向用户展示的工具名称（可与 Name() 不同）。
    UserFacingName(input json.RawMessage) string

    // SearchHint 返回用于工具搜索关键词匹配的提示短语（3-10 词）。
    SearchHint() string

    // --- 执行 ---

    // Execute 执行工具调用的核心逻辑。
    // onProgress 回调用于向引擎推送执行进度（可为 nil）。
    Execute(
        ctx context.Context,
        input json.RawMessage,
        tctx *UseContext,
        onProgress ProgressFn,
    ) (*Result, error)

    // ValidateInput 在 Execute 之前验证输入合法性。
    // 返回 ValidationResult.OK=false 时 Execute 不会被调用，引擎直接将错误返回给 LLM。
    ValidateInput(ctx context.Context, input json.RawMessage, tctx *UseContext) ValidationResult

    // --- 权限 ---

    // CheckPermissions 执行工具特定的权限检查逻辑。
    // 通用权限逻辑由 permissions 包处理；此方法处理工具独有的逻辑。
    // 仅在 ValidateInput 通过后调用。
    CheckPermissions(ctx context.Context, input json.RawMessage, tctx *UseContext) (PermissionResult, error)

    // PreparePermissionMatcher 为 Hook if-条件（如 "git *"）构造匹配器闭包。
    // 调用一次；返回的 fn 按 pattern 逐一匹配。
    // 未实现时仅支持工具名级别的规则匹配。
    PreparePermissionMatcher(ctx context.Context, input json.RawMessage) (func(pattern string) bool, error)

    // --- 并发与只读语义 ---

    // IsConcurrencySafe 返回此工具在给定输入下是否可并发执行（即只读）。
    // false（默认）= 串行执行，确保安全。
    IsConcurrencySafe(input json.RawMessage) bool

    // IsReadOnly 返回工具是否为纯只读操作（不修改任何状态）。
    IsReadOnly(input json.RawMessage) bool

    // IsDestructive 返回工具是否执行不可逆操作（删除/覆盖/发送）。
    // 默认 false，影响权限提示的措辞。
    IsDestructive(input json.RawMessage) bool

    // IsEnabled 返回工具当前是否可用（运行时特性门控）。
    IsEnabled() bool

    // --- 中断行为 ---

    // InterruptBehavior 定义用户在工具运行期间提交新消息时的行为：
    //   InterruptCancel — 停止工具，丢弃结果
    //   InterruptBlock  — 继续运行，新消息等待（默认）
    InterruptBehavior() InterruptBehavior

    // --- 分类与展示辅助 ---

    // IsSearchOrReadCommand 判断工具是否为搜索/读取类操作（用于 UI 折叠展示）。
    IsSearchOrReadCommand(input json.RawMessage) SearchReadClassification

    // IsOpenWorld 返回工具是否可能读取项目目录之外的资源。
    IsOpenWorld(input json.RawMessage) bool

    // RequiresUserInteraction 返回工具执行是否需要用户交互（如 vim/less）。
    RequiresUserInteraction() bool

    // GetPath 返回工具操作的主要文件路径（可选，用于权限路径匹配）。
    GetPath(input json.RawMessage) (string, bool)

    // MaxResultSizeChars 返回结果持久化到磁盘的字节阈值。
    // math.MaxInt = 永不持久化（如 Read 工具）。
    MaxResultSizeChars() int

    // ToAutoClassifierInput 返回用于安全分类器的简洁表示。
    // 返回 "" 表示跳过此工具的分类。
    ToAutoClassifierInput(input json.RawMessage) string

    // InputsEquivalent 判断两次调用的输入是否语义等价（用于去重）。
    // 未实现时使用 JSON 深度比较。
    InputsEquivalent(a, b json.RawMessage) bool

    // BackfillObservableInput 在输入被 hooks/permission 观察前就地补充派生字段。
    // 必须幂等；不修改 API 侧原始输入（prompt cache 保护）。
    BackfillObservableInput(input map[string]any)

    // --- MCP/LSP 标志 ---

    // IsMCP 返回是否为 MCP 工具。
    IsMCP() bool

    // MCPInfo 返回 MCP 服务信息（仅 MCP 工具有效）。
    MCPInfo() *MCPToolInfo

    // IsLSP 返回是否为 LSP 工具。
    IsLSP() bool

    // ShouldDefer 返回工具是否需要 ToolSearch 才能使用（延迟加载）。
    ShouldDefer() bool

    // AlwaysLoad 返回是否始终在初始 prompt 中加载（绕过延迟机制）。
    AlwaysLoad() bool

    // Strict 返回是否启用严格模式（API 强制遵循参数 schema）。
    Strict() bool
}

// Tools 是 Tool 的有序集合。使用具名类型便于跨层追踪工具集的组装与过滤。
type Tools []Tool
```

### 1.2 输入与 Schema 类型

```go
// tool.go（续）

// JSONSchema 描述工具输入的 JSON Schema（必须为 object 类型）。
type JSONSchema struct {
    Type                 string                    `json:"type"` // 固定为 "object"
    Properties           map[string]PropertySchema `json:"properties,omitempty"`
    Required             []string                  `json:"required,omitempty"`
    AdditionalProperties *bool                     `json:"additionalProperties,omitempty"`
    // 原始 JSON Schema（MCP 工具直接携带，无需从 Zod 转换）
    Raw json.RawMessage `json:"-"`
}

// PropertySchema 描述单个参数的 Schema。
type PropertySchema struct {
    Type        string                    `json:"type,omitempty"`
    Description string                    `json:"description,omitempty"`
    Enum        []any                     `json:"enum,omitempty"`
    Items       *PropertySchema           `json:"items,omitempty"`
    Properties  map[string]PropertySchema `json:"properties,omitempty"`
    Required    []string                  `json:"required,omitempty"`
}

// ValidationResult 是 ValidateInput 的返回结果。
type ValidationResult struct {
    OK        bool
    Message   string // OK=false 时的错误信息（发送给 LLM）
    ErrorCode int    // 可选错误码
}

// DescriptionOptions 是 Description 方法的配置项。
type DescriptionOptions struct {
    IsNonInteractiveSession bool
    PermissionCtx           PermissionContext
    Tools                   Tools
}

// PromptOptions 是 Prompt 方法的配置项。
type PromptOptions struct {
    GetPermissionContext func(ctx context.Context) (PermissionContext, error)
    Tools                Tools
    Agents               []AgentDefinition
    AllowedAgentTypes    []string
}

// SearchReadClassification 描述工具是否为搜索/读取类操作。
type SearchReadClassification struct {
    IsSearch bool
    IsRead   bool
    IsList   bool
}

// InterruptBehavior 定义用户中断时的工具行为。
type InterruptBehavior string

const (
    InterruptCancel InterruptBehavior = "cancel"
    InterruptBlock  InterruptBehavior = "block" // 默认
)

// MCPToolInfo 存储 MCP 工具的原始服务信息。
type MCPToolInfo struct {
    ServerName string
    ToolName   string
}

// AgentDefinition 描述一个可被调用的 Agent 类型。
type AgentDefinition struct {
    Name        string
    Description string
}
```

### 1.3 执行结果类型

```go
// result.go

package tool

import "encoding/json"

// Result 是工具执行的返回结果。
type Result struct {
    // Data 是工具的原始输出，将被序列化为 tool_result 的 content。
    Data any

    // NewMessages 是工具执行后需要追加到消息历史的额外消息（如子 Agent 消息）。
    NewMessages []Message

    // ContextModifier 允许工具修改 UseContext（仅非并发安全工具支持）。
    ContextModifier func(ctx *UseContext) *UseContext

    // MCPMeta 存储 MCP 协议元数据，透传给 SDK 消费方。
    MCPMeta *MCPResultMeta
}

// MCPResultMeta 存储 MCP 结果的协议元数据。
type MCPResultMeta struct {
    Meta              map[string]any `json:"_meta,omitempty"`
    StructuredContent map[string]any `json:"structuredContent,omitempty"`
}

// ProgressFn 是工具向引擎推送执行进度的回调类型。
type ProgressFn func(progress Progress)

// Progress 是进度事件的载体。
type Progress struct {
    ToolUseID string
    Data      ProgressData
}

// ProgressData 是各工具类型的进度数据（见 progress.go）。
type ProgressData interface {
    progressTag() // 私有方法，防止外部实现
}
```

### 1.4 进度类型

```go
// progress.go

package tool

// BashProgress 是 Bash 工具的进度数据。
type BashProgress struct {
    Type    string // "bash_progress"
    Output  string
    IsError bool
}

func (BashProgress) progressTag() {}

// AgentToolProgress 是 Agent 工具的进度数据。
type AgentToolProgress struct {
    Type       string // "agent_progress"
    AgentID    string
    StatusMsg  string
    SubMessage *Message
}

func (AgentToolProgress) progressTag() {}

// MCPProgress 是 MCP 工具的进度数据。
type MCPProgress struct {
    Type    string // "mcp_progress"
    Content string
}

func (MCPProgress) progressTag() {}

// WebSearchProgress 是 Web 搜索工具的进度数据。
type WebSearchProgress struct {
    Type  string // "web_search_progress"
    Query string
    URL   string
}

func (WebSearchProgress) progressTag() {}

// REPLToolProgress 是 REPL 工具的进度数据。
type REPLToolProgress struct {
    Type   string // "repl_progress"
    Output string
}

func (REPLToolProgress) progressTag() {}

// SkillToolProgress 是 Skill 工具的进度数据。
type SkillToolProgress struct {
    Type    string // "skill_progress"
    Message string
}

func (SkillToolProgress) progressTag() {}

// TaskOutputProgress 是任务输出的进度数据。
type TaskOutputProgress struct {
    Type    string // "task_output_progress"
    Content string
}

func (TaskOutputProgress) progressTag() {}

// HookProgress 是 Hook 执行的进度数据。
type HookProgress struct {
    Type          string // "hook_progress"
    HookEvent     string
    HookName      string
    Command       string
    PromptText    string
    StatusMessage string
}

func (HookProgress) progressTag() {}
```

### 1.5 权限类型

```go
// permission.go

package tool

// PermissionMode 定义权限模式枚举。
type PermissionMode string

const (
    PermissionModeDefault           PermissionMode = "default"
    PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
    PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
    PermissionModeDontAsk           PermissionMode = "dontAsk"
    PermissionModePlan              PermissionMode = "plan"
    PermissionModeAuto              PermissionMode = "auto"    // 内部模式
    PermissionModeBubble            PermissionMode = "bubble"  // 内部模式
)

// PermissionBehavior 是三级权限决策。
type PermissionBehavior string

const (
    PermissionAllow       PermissionBehavior = "allow"
    PermissionDeny        PermissionBehavior = "deny"
    PermissionAsk         PermissionBehavior = "ask"
    PermissionPassthrough PermissionBehavior = "passthrough"
)

// PermissionRuleSource 标识规则来源。
type PermissionRuleSource string

const (
    RuleSourceUserSettings    PermissionRuleSource = "userSettings"
    RuleSourceProjectSettings PermissionRuleSource = "projectSettings"
    RuleSourceLocalSettings   PermissionRuleSource = "localSettings"
    RuleSourceFlagSettings    PermissionRuleSource = "flagSettings"
    RuleSourcePolicySettings  PermissionRuleSource = "policySettings"
    RuleSourceCLIArg          PermissionRuleSource = "cliArg"
    RuleSourceCommand         PermissionRuleSource = "command"
    RuleSourceSession         PermissionRuleSource = "session"
)

// PermissionRuleValue 描述一条规则的目标（工具名 + 可选内容模式）。
type PermissionRuleValue struct {
    ToolName    string
    RuleContent string // 可选，如 "git *"
}

// PermissionRule 是带来源和行为的完整规则。
type PermissionRule struct {
    Source       PermissionRuleSource
    RuleBehavior PermissionBehavior
    RuleValue    PermissionRuleValue
}

// PermissionRulesBySource 按来源索引的规则集合（值为匹配 pattern 列表）。
type PermissionRulesBySource map[PermissionRuleSource][]string

// PermissionContext 是权限检查所需的只读上下文快照。
// 对应 TS 中的 ToolPermissionContext（DeepImmutable）。
type PermissionContext struct {
    Mode                             PermissionMode
    AdditionalWorkingDirectories     map[string]AdditionalWorkingDirectory
    AlwaysAllowRules                 PermissionRulesBySource
    AlwaysDenyRules                  PermissionRulesBySource
    AlwaysAskRules                   PermissionRulesBySource
    IsBypassPermissionsModeAvailable bool
    StrippedDangerousRules           PermissionRulesBySource
    ShouldAvoidPermissionPrompts     bool
    AwaitAutomatedChecksBeforeDialog bool
    PrePlanMode                      PermissionMode // 进入 plan 模式前的状态
}

// AdditionalWorkingDirectory 是扩展工作目录权限范围的目录条目。
type AdditionalWorkingDirectory struct {
    Path   string
    Source PermissionRuleSource
}

// PermissionResult 是工具 CheckPermissions 的返回值。
// 对应 TS 中的 PermissionResult（allow | ask | deny | passthrough）。
type PermissionResult struct {
    Behavior               PermissionBehavior
    Message                string             // deny/ask 时的提示信息
    UpdatedInput           map[string]any     // allow 时可选地修改输入
    UserModified           bool
    DecisionReason         *PermissionDecisionReason
    Suggestions            []PermissionUpdate
    BlockedPath            string
    PendingClassifierCheck *PendingClassifierCheck
    ContentBlocks          []ContentBlock     // 附加内容块（如截图反馈）
    AcceptFeedback         string
}

// PermissionDecisionReason 解释权限决策的原因（union type）。
type PermissionDecisionReason struct {
    Type string // "rule" | "mode" | "hook" | "classifier" | "workingDir" | "safetyCheck" | "other"

    // Type="rule"
    Rule *PermissionRule

    // Type="mode"
    Mode PermissionMode

    // Type="hook"
    HookName   string
    HookSource string
    Reason     string

    // Type="classifier"
    Classifier string

    // Type="safetyCheck"
    ClassifierApprovable bool
}

// PermissionUpdate 描述权限配置的变更操作。
type PermissionUpdate struct {
    Type        string                      // "addRules" | "replaceRules" | "removeRules" | "setMode" | "addDirectories" | "removeDirectories"
    Destination PermissionUpdateDestination
    Rules       []PermissionRuleValue
    Behavior    PermissionBehavior
    Mode        PermissionMode
    Directories []string
}

// PermissionUpdateDestination 标识权限更新的持久化目标。
type PermissionUpdateDestination string

const (
    UpdateDestUserSettings    PermissionUpdateDestination = "userSettings"
    UpdateDestProjectSettings PermissionUpdateDestination = "projectSettings"
    UpdateDestLocalSettings   PermissionUpdateDestination = "localSettings"
    UpdateDestSession         PermissionUpdateDestination = "session"
    UpdateDestCLIArg          PermissionUpdateDestination = "cliArg"
)

// PendingClassifierCheck 描述异步安全分类器检查的参数。
type PendingClassifierCheck struct {
    Command      string
    CWD          string
    Descriptions []string
}

// ContentBlock 是附加到权限结果中的内容块（文本或图片）。
type ContentBlock struct {
    Type   string // "text" | "image"
    Text   string
    Source *ImageSource
}

// ImageSource 描述图片内容的来源。
type ImageSource struct {
    Type      string // "base64"
    MediaType string // "image/png" 等
    Data      string
}
```

### 1.6 UseContext（工具调用上下文）

```go
// context.go

package tool

import (
    "time"
)

// UseContext 是工具执行期间的完整运行时上下文。
// 对应 TS 中的 ToolUseContext，是贯穿整个查询循环的核心状态载体。
type UseContext struct {
    // --- 配置选项（不可变，每次查询固定）---
    Options UseContextOptions

    // --- 中止控制 ---
    // AbortCh 关闭时表示本次查询已被取消。
    AbortCh <-chan struct{}

    // --- 消息历史 ---
    Messages []Message

    // --- Agent 标识（子 Agent 专用）---
    AgentID   string // 主线程为空
    AgentType string // 子 Agent 类型名

    // --- 查询链追踪 ---
    QueryTracking *QueryChainTracking

    // --- 权限上下文 ---
    PermissionCtx PermissionContext

    // --- 进度回调（由引擎注入）---
    OnProgress    ProgressFn
    SetStreamMode func(mode string)

    // --- 文件限制 ---
    FileReadLimits *FileReadLimits
    GlobLimits     *GlobLimits

    // --- 工具决策记录（用于去重/审计）---
    ToolDecisions map[string]ToolDecisionRecord

    // --- 内部状态（引擎管理）---
    ToolUseID string // 当前工具调用的 ID
}

// UseContextOptions 是不可变的查询级配置。
type UseContextOptions struct {
    Commands                []Command
    Debug                   bool
    MainLoopModel           string
    Tools                   Tools
    Verbose                 bool
    MCPClients              []MCPServerConnection
    IsNonInteractiveSession bool
    MaxBudgetUSD            float64
    CustomSystemPrompt      string
    AppendSystemPrompt      string
    QuerySource             string
    RefreshTools            func() Tools // MCP 热插拔时的工具刷新回调
}

// QueryChainTracking 追踪查询链的深度（用于子 Agent 递归计量）。
type QueryChainTracking struct {
    ChainID string
    Depth   int
}

// FileReadLimits 限制文件读取的规模。
type FileReadLimits struct {
    MaxTokens    int
    MaxSizeBytes int64
}

// GlobLimits 限制 Glob 搜索的结果数量。
type GlobLimits struct {
    MaxResults int
}

// ToolDecisionRecord 记录单次工具调用的决策信息（用于审计）。
type ToolDecisionRecord struct {
    Source    string    // 决策来源
    Decision  string    // "accept" | "reject"
    Timestamp time.Time
}
```

### 1.7 默认实现基类（BaseTool）

```go
// tool.go（续）

// BaseTool 为 Tool 接口提供安全的默认实现，减少工具实现的样板代码。
// 工具实现通过嵌入 BaseTool 并覆盖必要方法来满足接口要求。
//
// 默认策略（fail-closed）：
//   - IsConcurrencySafe → false（假设不安全）
//   - IsReadOnly        → false（假设有写操作）
//   - IsDestructive     → false
//   - IsEnabled         → true
//   - CheckPermissions  → allow（交由通用权限系统处理）
//   - InterruptBehavior → InterruptBlock
//   - ToAutoClassifierInput → ""（跳过分类器）
type BaseTool struct {
    ToolName    string
    ToolAliases []string
}

func (b *BaseTool) Name() string                              { return b.ToolName }
func (b *BaseTool) Aliases() []string                         { return b.ToolAliases }
func (b *BaseTool) IsConcurrencySafe(_ json.RawMessage) bool  { return false }
func (b *BaseTool) IsReadOnly(_ json.RawMessage) bool         { return false }
func (b *BaseTool) IsDestructive(_ json.RawMessage) bool      { return false }
func (b *BaseTool) IsEnabled() bool                           { return true }
func (b *BaseTool) InterruptBehavior() InterruptBehavior       { return InterruptBlock }
func (b *BaseTool) IsMCP() bool                               { return false }
func (b *BaseTool) MCPInfo() *MCPToolInfo                     { return nil }
func (b *BaseTool) IsLSP() bool                               { return false }
func (b *BaseTool) ShouldDefer() bool                         { return false }
func (b *BaseTool) AlwaysLoad() bool                          { return false }
func (b *BaseTool) Strict() bool                              { return false }
func (b *BaseTool) SearchHint() string                        { return "" }
func (b *BaseTool) IsOpenWorld(_ json.RawMessage) bool        { return false }
func (b *BaseTool) RequiresUserInteraction() bool             { return false }
func (b *BaseTool) GetPath(_ json.RawMessage) (string, bool)  { return "", false }
func (b *BaseTool) ToAutoClassifierInput(_ json.RawMessage) string { return "" }
func (b *BaseTool) UserFacingName(_ json.RawMessage) string   { return b.ToolName }
func (b *BaseTool) BackfillObservableInput(_ map[string]any)  {}
func (b *BaseTool) MaxResultSizeChars() int                   { return 100_000 }
func (b *BaseTool) IsSearchOrReadCommand(_ json.RawMessage) SearchReadClassification {
    return SearchReadClassification{}
}
func (b *BaseTool) InputsEquivalent(a, b2 json.RawMessage) bool {
    return string(a) == string(b2)
}
func (b *BaseTool) CheckPermissions(
    _ context.Context,
    input json.RawMessage,
    _ *UseContext,
) (PermissionResult, error) {
    return PermissionResult{
        Behavior:     PermissionAllow,
        UpdatedInput: nil,
    }, nil
}
func (b *BaseTool) ValidateInput(
    _ context.Context,
    _ json.RawMessage,
    _ *UseContext,
) ValidationResult {
    return ValidationResult{OK: true}
}
func (b *BaseTool) PreparePermissionMatcher(
    _ context.Context,
    _ json.RawMessage,
) (func(string) bool, error) {
    return nil, nil
}

// 工具帮助函数

// FindByName 在工具集合中按名称或别名查找工具。
func FindByName(tools Tools, name string) (Tool, bool) {
    for _, t := range tools {
        if t.Name() == name {
            return t, true
        }
        for _, alias := range t.Aliases() {
            if alias == name {
                return t, true
            }
        }
    }
    return nil, false
}
```

---

## 2. internal/engine — 查询引擎

> 查询引擎是驱动整个 LLM 对话循环的核心。它从 TUI/SDK 层接收用户消息，
> 驱动 LLM 采样、工具编排、上下文压缩，并通过 channel 向上层推送流式事件。

### 2.1 QueryEngine 接口（供 TUI 层调用）

```go
// internal/engine/engine.go

package engine

import (
    "context"
    "github.com/your-org/claude-code-go/internal/tool"
)

// QueryEngine 是引擎层暴露给 TUI/SDK 层的顶层接口。
type QueryEngine interface {
    // Query 启动一次完整的 LLM 查询循环，返回流式消息 channel。
    // 调用方通过读取返回的 channel 消费所有事件，直到 channel 关闭。
    // 取消通过 ctx 传递。
    Query(ctx context.Context, params QueryParams) (<-chan Msg, error)

    // Interrupt 中断当前正在运行的查询（等价于用户按下 Ctrl+C）。
    Interrupt(ctx context.Context)

    // GetMessages 返回当前会话的完整消息历史。
    GetMessages() []tool.Message

    // SetMessages 替换消息历史（用于 /compact 后重置）。
    SetMessages(messages []tool.Message)
}

// QueryParams 是一次查询调用的完整参数。
type QueryParams struct {
    Messages                []tool.Message
    SystemPrompt            SystemPrompt
    UserContext             map[string]string
    SystemContext           map[string]string
    ToolUseContext          *tool.UseContext
    QuerySource             string
    MaxOutputTokensOverride int
    MaxTurns                int
    SkipCacheWrite          bool
    TaskBudget              *TaskBudget
    FallbackModel           string
}

// TaskBudget 定义 API task_budget（output_config.task_budget）。
type TaskBudget struct {
    Total int
}

// SystemPrompt 封装系统提示词（可含 cache_control 标记）。
type SystemPrompt struct {
    Parts []SystemPromptPart
}

// SystemPromptPart 是系统提示词的一个片段。
type SystemPromptPart struct {
    Text         string
    CacheControl string // "ephemeral" | ""
}
```

### 2.2 引擎向 TUI 推送的 Msg 类型

```go
// internal/engine/msg.go

package engine

import "github.com/your-org/claude-code-go/internal/tool"

// Msg 是引擎通过 channel 向 TUI 层推送的事件（sum type）。
// TUI 层通过 switch msg.Type 分发处理。
type Msg struct {
    Type MsgType
    // 各字段按 Type 使用，未使用的字段为零值。

    // --- MsgTypeStreamText ---
    TextDelta string

    // --- MsgTypeToolUseStart ---
    ToolUseID  string
    ToolName   string
    InputDelta string // 流式 JSON 片段

    // --- MsgTypeToolUseComplete ---
    ToolInput string // 完整 JSON

    // --- MsgTypeToolResult ---
    ToolResult *ToolResultMsg

    // --- MsgTypeAssistantMessage ---
    AssistantMsg *tool.Message

    // --- MsgTypeUserMessage ---
    UserMsg *tool.Message

    // --- MsgTypeProgress ---
    Progress *tool.Progress

    // --- MsgTypeError ---
    Err error

    // --- MsgTypeRequestStart ---
    RequestID string
    Model     string

    // --- MsgTypeTurnComplete ---
    StopReason         string
    InputTokens        int
    OutputTokens       int
    CacheReadTokens    int
    CacheCreatedTokens int

    // --- MsgTypeCompactStart / MsgTypeCompactEnd ---
    CompactStrategy string // "auto" | "micro" | "snip"

    // --- MsgTypeSystemMessage ---
    SystemText string
}

// MsgType 是 Msg 的消息类型枚举。
type MsgType string

const (
    MsgTypeStreamRequestStart MsgType = "stream_request_start"
    MsgTypeStreamText         MsgType = "stream_text"
    MsgTypeThinkingDelta      MsgType = "thinking_delta"
    MsgTypeToolUseStart       MsgType = "tool_use_start"
    MsgTypeToolUseComplete    MsgType = "tool_use_complete"
    MsgTypeToolResult         MsgType = "tool_result"
    MsgTypeAssistantMessage   MsgType = "assistant_message"
    MsgTypeUserMessage        MsgType = "user_message"
    MsgTypeProgress           MsgType = "progress"
    MsgTypeError              MsgType = "error"
    MsgTypeRequestStart       MsgType = "request_start"
    MsgTypeTurnComplete       MsgType = "turn_complete"
    MsgTypeCompactStart       MsgType = "compact_start"
    MsgTypeCompactEnd         MsgType = "compact_end"
    MsgTypeSystemMessage      MsgType = "system_message"
    MsgTypeTombstone          MsgType = "tombstone" // 消息删除标记
)

// ToolResultMsg 是工具执行结果的消息体。
type ToolResultMsg struct {
    ToolUseID string
    Content   string
    IsError   bool
}
```

### 2.3 主循环状态机设计

```
QueryLoop 状态机：

                  ┌─────────────────────────────┐
                  │  INIT                        │
                  │  - 构建 budgetTracker         │
                  │  - buildQueryConfig()         │
                  │  - startMemoryPrefetch()      │
                  └──────────────┬──────────────┘
                                 │
                  ┌──────────────▼──────────────────────────────────────────┐
                  │  LOOP START (每次迭代)                                    │
                  │                                                          │
                  │  1. applyToolResultBudget()  — 大工具结果持久化到磁盘     │
                  │  2. snipCompactIfNeeded()    — Snip 压缩                 │
                  │  3. microcompact()           — 微压缩（缓存编辑模式）      │
                  │  4. autocompact()            — 自动压缩（上下文接近 limit）│
                  │  5. 构建 API 请求参数                                     │
                  │  6. StreamingLLMCall()       — 流式调用 LLM              │
                  │     ├─ yield MsgTypeStreamText（文本 delta）              │
                  │     ├─ yield MsgTypeThinkingDelta（思考 delta）           │
                  │     └─ yield MsgTypeToolUseStart/Complete（工具调用）     │
                  └──────────────┬──────────────────────────────────────────┘
                                 │
             ┌───────────────────┼──────────────────────┐
             │                   │                      │
    stop_reason=end_turn  stop_reason=tool_use   stop_reason=max_output_tokens
             │                   │                      │
             ▼                   ▼                      ▼
        TERMINAL         TOOL_EXECUTION          MAX_TOKENS_RECOVERY
        （正常结束）      runTools()              （最多重试 3 次）
                         ├─ isConcurrencySafe=true
                         │   → 并发执行（goroutine pool）
                         └─ isConcurrencySafe=false
                             → 串行执行
                                 │                      │
                                 └──────────────────────┘
                                          │
                                     LOOP START（继续）
```

### 2.4 goroutine 架构

```
┌─────────────────────────────────────────────────────────────────┐
│ TUI goroutine（主 goroutine）                                     │
│   - 读取 msgCh，驱动 Bubble Tea model 更新                        │
└──────────────────────────┬──────────────────────────────────────┘
                           │ msgCh <-chan Msg
┌──────────────────────────▼──────────────────────────────────────┐
│ QueryLoop goroutine                                              │
│   - 执行完整的 while(true) 查询循环                                │
│   - 向 msgCh 推送所有 Msg 事件                                    │
│   - 通过 ctx 感知取消                                             │
└──────────┬────────────────────────────────────┬─────────────────┘
           │ LLM stream                         │ runTools()
┌──────────▼──────────┐          ┌──────────────▼──────────────────┐
│ SSE Reader goroutine │          │ Tool Executor goroutines         │
│  - 读取 HTTP 流      │          │  - 并发安全工具：goroutine pool   │
│  - 解析 SSE events   │          │    (max=CLAUDE_CODE_MAX_TOOL_    │
│  - 推送到内部 ch      │          │     USE_CONCURRENCY, 默认 10)   │
└─────────────────────┘          │  - 非并发安全工具：串行队列        │
                                 └────────────────────────────────┘
```

### 2.5 工具并发编排策略

```go
// 对应 TS 中的 toolOrchestration.ts：partitionToolCalls + runTools

// 分区策略：将一批 tool_use 调用分成连续的"并发安全"和"非并发安全"分区。
// 策略：只要相邻调用均为 isConcurrencySafe=true，它们进入同一并发批次；
//       一旦遇到 isConcurrencySafe=false，单独成批串行执行。
//
// 执行规则：
//   - 并发批次：使用 errgroup + semaphore（大小=MaxToolUseConcurrency）并发执行
//   - 串行批次：顺序执行，前一个的 contextModifier 在下一个启动前应用
//   - contextModifier 只对串行批次的调用有效（并发批次忽略，队列化后应用）

// 默认并发上限
const DefaultMaxToolUseConcurrency = 10
// 环境变量覆盖：CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY
```

### 2.6 Token 预算管理

```go
// internal/engine/budget.go

// BudgetTracker 追踪 Token 预算消耗，决定是否继续输出。
type BudgetTracker struct {
    ContinuationCount    int
    LastDeltaTokens      int
    LastGlobalTurnTokens int
    StartedAt            time.Time
}

// TokenBudgetDecision 是预算检查的决策结果。
type TokenBudgetDecision struct {
    Action             string // "continue" | "stop"
    NudgeMessage       string // continue 时注入给 LLM 的提示
    ContinuationCount  int
    Pct                int    // 已使用百分比
    TurnTokens         int
    Budget             int
    DiminishingReturns bool   // stop 时是否因收益递减
    DurationMs         int64
}

// 阈值常量（对应 TS query/tokenBudget.ts）：
//   CompletionThreshold  = 0.90（达到 90% 触发 continue nudge）
//   DiminishingThreshold = 500 tokens（连续 delta < 500 视为收益递减）
//   MaxContinuations     = 3（3 次 nudge 后仍无进展则停止）
```

---

## 3. internal/permissions — 权限系统

### 3.1 Checker 接口

```go
// internal/permissions/checker.go

package permissions

import (
    "context"
    "github.com/your-org/claude-code-go/internal/tool"
)

// Checker 是权限系统的核心接口。
// 它接收工具调用请求，综合模式、规则、Hook 结果，产出三级决策。
type Checker interface {
    // CanUseTool 是供 Hook useCanUseTool 调用的快速检查（不弹 UI）。
    // 返回 allow/deny/ask 三级决策。
    CanUseTool(
        ctx context.Context,
        toolName string,
        input tool.JSONRaw,
        tctx *tool.UseContext,
    ) (tool.PermissionResult, error)

    // RequestPermission 执行完整的权限流程：
    //   1. CanUseTool 快速检查
    //   2. 若 ask → 通过 askCh 向 TUI 发送交互请求
    //   3. 等待用户响应（或超时）
    //   4. 记录决策到历史
    RequestPermission(
        ctx context.Context,
        req PermissionRequest,
        tctx *tool.UseContext,
    ) (tool.PermissionResult, error)

    // GetDenialCount 返回当前会话的累计拒绝次数（用于自动降级策略）。
    GetDenialCount() int
}

// PermissionRequest 是完整权限请求的参数包。
type PermissionRequest struct {
    ToolName   string
    ToolUseID  string
    Input      tool.JSONRaw
    // 来自 tool.CheckPermissions 的初步结果（可能需要进一步确认）
    ToolResult tool.PermissionResult
}
```

### 3.2 规则匹配算法（allow/deny/ask 三级决策）

```
决策优先级（从高到低）：

1. bypassPermissions 模式 → 直接 allow（最高优先级）
2. alwaysDenyRules 精确/模式匹配 → deny
3. 工具 validateInput() → deny（输入不合法）
4. Hook PreToolUse → allow/deny/ask（Hook 结果覆盖后续规则）
5. alwaysAllowRules 精确/模式匹配 → allow
6. alwaysAskRules 精确/模式匹配 → ask
7. PermissionMode 决策：
   - acceptEdits → allow（编辑类）/ ask（其他写操作）
   - dontAsk     → allow（跳过所有询问）
   - plan        → deny（禁止写操作）
   - default     → ask（write 类工具需询问）
8. 工具 checkPermissions() → tool-specific 逻辑
9. auto 模式 → 异步分类器评估（非阻塞）

Pattern 匹配规则：
  - "ToolName"          精确工具名匹配
  - "ToolName(pattern)" 工具名 + 内容模式（如 "Bash(git *)"）
  - pattern 支持 glob（* 匹配任意字符串，不含路径分隔符）
  - 工具通过 PreparePermissionMatcher 提供领域感知的匹配器
```

### 3.3 ask-user channel 协议

```go
// internal/permissions/ask.go

// AskRequest 是权限系统向 TUI 层发送的交互请求。
type AskRequest struct {
    ID          string               // 请求 ID，用于配对响应
    ToolName    string
    ToolUseID   string
    Message     string               // 展示给用户的权限说明
    Input       tool.JSONRaw         // 工具输入（用于展示）
    Suggestions []tool.PermissionUpdate // 建议的快捷选项
    BlockedPath string               // 被阻止的文件路径（如有）
}

// AskResponse 是 TUI 层对权限请求的响应。
type AskResponse struct {
    ID           string               // 配对的请求 ID
    Decision     tool.PermissionBehavior // "allow" | "deny"
    Updates      []tool.PermissionUpdate // 用户选择的规则更新
    UserModified bool
}

// 协议：
//   engine → askCh  (chan<- AskRequest)  发送请求
//   TUI    → respCh (chan<- AskResponse) 发送响应
//   engine 阻塞在 select { case r := <-respCh; case <-ctx.Done() }
//
// 超时处理：ctx 取消时自动 deny，避免无限等待
```

### 3.4 权限拒绝历史记录

```go
// internal/permissions/denial.go

// DenialTrackingState 追踪权限拒绝历史，驱动自动降级策略。
// 对应 TS 中的 DenialTrackingState（utils/permissions/denialTracking.ts）。
type DenialTrackingState struct {
    DenialCount   int           // 累计拒绝次数
    LastDeniedAt  time.Time     // 最后一次拒绝的时间
    RecentDenials []DenialRecord // 最近的拒绝记录
}

// DenialRecord 是单次权限拒绝的记录。
type DenialRecord struct {
    ToolName  string
    ToolUseID string
    Reason    string
    DeniedAt  time.Time
}

// 降级策略：
//   DenialCount >= DenialThreshold → 自动切换为 prompting 模式
//   async 子 Agent 使用 localDenialTracking（与 AppState 隔离）
//   主线程使用全局 AppState 中的拒绝计数
```

---

## 4. internal/compact — 上下文压缩

### 4.1 Compressor 接口

```go
// internal/compact/compressor.go

package compact

import (
    "context"
    "github.com/your-org/claude-code-go/internal/tool"
)

// Compressor 是上下文压缩的统一接口。
type Compressor interface {
    // NeedsCompaction 检查当前消息列表是否需要压缩。
    NeedsCompaction(messages []tool.Message, model string, extra CompactionExtra) bool

    // Compact 执行压缩，返回压缩后的消息列表。
    Compact(
        ctx context.Context,
        messages []tool.Message,
        tctx *tool.UseContext,
        params CompactionParams,
    ) (*CompactionResult, error)
}

// CompactionExtra 是压缩判断的附加信息。
type CompactionExtra struct {
    SnipTokensFreed int // Snip 压缩已释放的 token 数
}

// CompactionParams 是压缩调用的参数。
type CompactionParams struct {
    SystemPrompt  tool.SystemPrompt
    UserContext   map[string]string
    SystemContext map[string]string
    QuerySource   string
    ForkMessages  []tool.Message // 用于 fork 子 Agent 的上下文快照
}

// CompactionResult 是压缩的结果。
type CompactionResult struct {
    SummaryMessages           []tool.Message
    Attachments               []tool.Message
    HookResults               []tool.Message
    PreCompactTokenCount      int
    PostCompactTokenCount     int
    TruePostCompactTokenCount int
    CompactionUsage           *TokenUsage
}

// TokenUsage 记录 LLM API 的 token 消耗。
type TokenUsage struct {
    InputTokens              int
    OutputTokens             int
    CacheReadInputTokens     int
    CacheCreationInputTokens int
}
```

### 4.2 Auto-compact 策略

```go
// internal/compact/auto.go

// AutoCompactor 实现基于 token 阈值的自动压缩。
// 对应 TS 中的 services/compact/autoCompact.ts。
//
// 触发条件：
//   当前上下文 token 数 > getAutoCompactThreshold(model)
//   threshold = effectiveContextWindow - AUTOCOMPACT_BUFFER_TOKENS(13_000)
//   effectiveContextWindow = contextWindow - min(maxOutputTokens, 20_000)
//
// 电路熔断：
//   连续失败 >= MAX_CONSECUTIVE_FAILURES(3) 时停止重试
//
// 压缩流程：
//   1. 运行 pre_compact Hook
//   2. 调用 LLM 生成摘要（summary）
//   3. 构建 postCompactMessages（summary + tail）
//   4. 运行 post_compact Hook
//   5. 更新 autoCompactTracking 状态（turnId、turnCounter 重置）

// AutoCompactTrackingState 追踪自动压缩的会话状态。
type AutoCompactTrackingState struct {
    Compacted           bool
    TurnCounter         int
    TurnID              string
    ConsecutiveFailures int
}

// 关键常量：
//   AUTOCOMPACT_BUFFER_TOKENS     = 13_000
//   WARNING_THRESHOLD_BUFFER      = 20_000
//   MAX_OUTPUT_TOKENS_FOR_SUMMARY = 20_000
//   MAX_CONSECUTIVE_FAILURES      = 3
// 环境变量覆盖：CLAUDE_CODE_AUTO_COMPACT_WINDOW
```

### 4.3 Micro-compact 策略

```go
// internal/compact/micro.go

// MicroCompactor 实现细粒度的工具结果压缩。
// 对应 TS 中的 services/compact/microCompact.ts。
//
// 触发条件：
//   单条 tool_result 超过 microcompact 阈值时，
//   用摘要替换原始结果（保留 tool_use_id，不改变消息结构）。
//
// Cached MicroCompact（CACHED_MICROCOMPACT feature flag）：
//   使用 cache editing 机制，通过 cache_control 标记实现，
//   defers boundary message 直到 API 响应（获得真实 cache_deleted_tokens）。

// MicroCompactResult 是微压缩的返回结果。
type MicroCompactResult struct {
    Messages       []tool.Message
    CompactionInfo *MicroCompactionInfo
}

// MicroCompactionInfo 是微压缩的元信息。
type MicroCompactionInfo struct {
    // PendingCacheEdits 仅在 cached microcompact 模式下设置
    PendingCacheEdits []CacheEdit
}

// CacheEdit 描述一条缓存编辑记录。
type CacheEdit struct {
    ToolUseID string
    Summary   string
}
```

### 4.4 Snip-compact 策略

```go
// internal/compact/snip.go

// SnipCompactor 实现消息历史的"剪枝"压缩（HISTORY_SNIP feature flag）。
// 对应 TS 中的 services/compact/snipCompact.ts。
//
// 策略：
//   从历史消息中识别并移除"可剪枝"的旧 tool_use/tool_result 对，
//   保留最近的 N 轮和所有 assistant 文本消息。
//   比 autocompact 更轻量，不需要 LLM 生成摘要。
//
// 触发时机：
//   在 microcompact 之前、autocompact 之前运行（query.ts 主循环中）。

// SnipResult 是剪枝压缩的返回结果。
type SnipResult struct {
    Messages        []tool.Message
    TokensFreed     int            // 释放的 token 估算数
    BoundaryMessage *tool.Message  // 如有，插入到消息流的分界标记
}

// SnipCompactIfNeeded 按需执行剪枝压缩。
// snipTokensFreed 传给 autocompact 用于阈值校准。
func SnipCompactIfNeeded(messages []tool.Message) SnipResult
```

---

## 5. internal/hooks — Hooks 系统

### 5.1 Hook 类型枚举

```go
// internal/hooks/types.go

package hooks

// HookEvent 定义所有 Hook 触发点的枚举。
// 对应 TS 中的 HOOK_EVENTS（entrypoints/agentSdkTypes.ts）。
type HookEvent string

const (
    HookEventPreToolUse         HookEvent = "PreToolUse"
    HookEventPostToolUse        HookEvent = "PostToolUse"
    HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
    HookEventPreSampling        HookEvent = "PreSampling"
    HookEventPostSampling       HookEvent = "PostSampling"
    HookEventSessionStart       HookEvent = "SessionStart"
    HookEventSessionEnd         HookEvent = "SessionEnd"
    HookEventSetup              HookEvent = "Setup"
    HookEventSubagentStart      HookEvent = "SubagentStart"
    HookEventPermissionRequest  HookEvent = "PermissionRequest"
    HookEventPermissionDenied   HookEvent = "PermissionDenied"
    HookEventNotification       HookEvent = "Notification"
    HookEventElicitation        HookEvent = "Elicitation"
    HookEventElicitationResult  HookEvent = "ElicitationResult"
    HookEventCwdChanged         HookEvent = "CwdChanged"
    HookEventFileChanged        HookEvent = "FileChanged"
    HookEventWorktreeCreate     HookEvent = "WorktreeCreate"
    HookEventUserPromptSubmit   HookEvent = "UserPromptSubmit"
    HookEventStop               HookEvent = "Stop"
)
```

### 5.2 Executor 接口

```go
// internal/hooks/executor.go

package hooks

import (
    "context"
    "github.com/your-org/claude-code-go/internal/tool"
)

// Executor 是 Hook 系统的执行接口。
type Executor interface {
    // RunHooks 触发指定事件的所有已注册 Hook，返回聚合结果。
    RunHooks(
        ctx context.Context,
        event HookEvent,
        input HookInput,
        toolUseID string,
    ) (*AggregatedHookResult, error)

    // RegisterCallback 注册一个回调类型的 Hook。
    RegisterCallback(event HookEvent, matcher string, cb HookCallback)

    // RegisterCommand 注册一个命令行类型的 Hook（通过 subprocess 执行）。
    RegisterCommand(event HookEvent, matcher string, cmd HookCommandConfig)
}

// HookInput 是传给 Hook 的输入数据。
type HookInput struct {
    Event     HookEvent
    ToolName  string
    ToolUseID string
    Input     tool.JSONRaw
    Messages  []tool.Message
    SessionID string
    AgentID   string
}

// HookCallback 是回调类型的 Hook 定义。
type HookCallback struct {
    Type     string // "callback"
    Fn       func(ctx context.Context, input HookInput, toolUseID string) (*HookJSONOutput, error)
    Timeout  int    // 秒，0=无超时
    Internal bool   // 内部 Hook 不计入 metrics
}

// HookCommandConfig 是命令行类型的 Hook 配置。
type HookCommandConfig struct {
    Command string
    Timeout int // 秒
    Env     map[string]string
}

// HookJSONOutput 是 Hook 的 JSON 输出结构（sync 或 async）。
type HookJSONOutput struct {
    // Sync 字段
    Continue           *bool               `json:"continue,omitempty"`
    SuppressOutput     *bool               `json:"suppressOutput,omitempty"`
    StopReason         string              `json:"stopReason,omitempty"`
    Decision           string              `json:"decision,omitempty"` // "approve" | "block"
    Reason             string              `json:"reason,omitempty"`
    SystemMessage      string              `json:"systemMessage,omitempty"`
    HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`

    // Async 字段（互斥于 sync 字段）
    Async        bool `json:"async,omitempty"`
    AsyncTimeout int  `json:"asyncTimeout,omitempty"`
}

// HookSpecificOutput 是各 Hook 类型的专属输出字段。
type HookSpecificOutput struct {
    HookEventName string `json:"hookEventName"`

    // PreToolUse 专属
    PermissionDecision       string         `json:"permissionDecision,omitempty"`
    PermissionDecisionReason string         `json:"permissionDecisionReason,omitempty"`
    UpdatedInput             map[string]any `json:"updatedInput,omitempty"`
    AdditionalContext        string         `json:"additionalContext,omitempty"`

    // PostToolUse 专属
    UpdatedMCPToolOutput any `json:"updatedMCPToolOutput,omitempty"`

    // PermissionDenied 专属
    Retry *bool `json:"retry,omitempty"`

    // SessionStart 专属
    InitialUserMessage string   `json:"initialUserMessage,omitempty"`
    WatchPaths         []string `json:"watchPaths,omitempty"`
}

// AggregatedHookResult 是多个 Hook 结果的聚合输出。
type AggregatedHookResult struct {
    Message                      *tool.Message
    BlockingErrors               []HookBlockingError
    PreventContinuation          bool
    StopReason                   string
    HookPermissionDecisionReason string
    PermissionBehavior           tool.PermissionBehavior
    AdditionalContexts           []string
    InitialUserMessage           string
    UpdatedInput                 map[string]any
    UpdatedMCPToolOutput         any
    PermissionRequestResult      *PermissionRequestResult
    Retry                        bool
}

// HookBlockingError 是 Hook 产生的阻塞性错误。
type HookBlockingError struct {
    BlockingError string
    Command       string
}

// PermissionRequestResult 是 PermissionRequest Hook 的决策结果。
type PermissionRequestResult struct {
    Behavior           tool.PermissionBehavior
    UpdatedInput       map[string]any
    UpdatedPermissions []tool.PermissionUpdate
    Message            string
    Interrupt          bool
}
```

### 5.3 PreToolUse / PostToolUse / PreSampling 各阶段

```
PreToolUse（工具调用前）：
  触发时机：工具 Execute() 调用前
  输入：ToolName、Input、Messages、SessionID
  可做：
    - 修改 updatedInput（覆盖工具输入）
    - 返回 permissionDecision=approve/block（覆盖权限系统结论）
    - 添加 additionalContext（注入到 tool_result 之前的系统消息）
    - 设置 continue=false + stopReason（阻止整个查询继续）
  执行顺序：与 permissions.CanUseTool 并发（awaitAutomatedChecksBeforeDialog=true 时等待）

PostToolUse（工具调用后）：
  触发时机：工具 Execute() 返回后，tool_result 追加到消息前
  输入：ToolName、Input、Output（tool_result content）
  可做：
    - updatedMCPToolOutput（仅 MCP 工具，修改输出）
    - additionalContext（追加到工具结果之后的系统消息）
    - continue=false（停止后续采样）

PreSampling / UserPromptSubmit（采样前）：
  触发时机：用户提交消息后、LLM API 调用前
  输入：Messages（完整历史）、UserMessage
  可做：
    - additionalContext（注入额外上下文到本次请求）
    - initialUserMessage（替换或补充用户消息）
    - continue=false（完全阻止本次采样）
    - watchPaths（SessionStart 时注册文件监听路径）

PostSampling（采样后）：
  触发时机：LLM 返回 stop_reason 后、工具执行前
  可做：
    - 审计/日志
    - 注入 systemMessage（在下一轮 system prompt 中附加）
```

---

## 6. internal/coordinator — 多 Agent 协调

### 6.1 Coordinator 接口

```go
// internal/coordinator/coordinator.go

package coordinator

import (
    "context"
    "github.com/your-org/claude-code-go/internal/tool"
)

// Coordinator 管理多 Agent 的生命周期和消息路由。
// 对应 TS 中的 coordinator/ 目录及 coordinatorMode.ts 的逻辑。
type Coordinator interface {
    // SpawnAgent 启动一个新的子 Agent，返回 Agent ID。
    SpawnAgent(ctx context.Context, req SpawnRequest) (AgentID, error)

    // SendMessage 向已有子 Agent 发送后续消息（continue 语义）。
    SendMessage(ctx context.Context, to AgentID, message string) error

    // StopAgent 停止一个运行中的子 Agent。
    StopAgent(ctx context.Context, agentID AgentID) error

    // GetAgentStatus 查询子 Agent 的当前状态。
    GetAgentStatus(ctx context.Context, agentID AgentID) (AgentStatus, error)

    // Subscribe 订阅子 Agent 的结果通知（task-notification）。
    Subscribe(agentID AgentID) (<-chan TaskNotification, error)

    // IsCoordinatorMode 返回当前是否处于 coordinator 模式。
    IsCoordinatorMode() bool
}

// AgentID 是子 Agent 的唯一标识符。
type AgentID string

// SpawnRequest 是启动子 Agent 的请求参数。
type SpawnRequest struct {
    Description   string    // 子 Agent 的任务描述
    SubagentType  string    // "worker" | 自定义类型
    Prompt        string    // 子 Agent 的完整初始提示
    Model         string    // 可选，空=使用默认模型
    AllowedTools  []string  // 可选，限制子 Agent 可用工具
    MaxTurns      int       // 可选，最大轮数
    ParentAgentID AgentID   // 父 Agent ID（嵌套时设置）
}

// AgentStatus 是子 Agent 的运行状态枚举。
type AgentStatus string

const (
    AgentStatusRunning   AgentStatus = "running"
    AgentStatusCompleted AgentStatus = "completed"
    AgentStatusFailed    AgentStatus = "failed"
    AgentStatusStopped   AgentStatus = "killed"
)

// TaskNotification 是子 Agent 完成时发送给父 Agent（Coordinator）的通知。
// 对应 TS coordinatorMode.ts 中的 <task-notification> XML 协议。
type TaskNotification struct {
    TaskID  AgentID
    Status  AgentStatus
    Summary string      // 人类可读的状态摘要
    Result  string      // Agent 的最终文本输出
    Usage   *AgentUsage
}

// AgentUsage 记录子 Agent 的资源消耗。
type AgentUsage struct {
    TotalTokens int
    ToolUses    int
    DurationMs  int64
}
```

### 6.2 子 Agent 生命周期管理

```
子 Agent 生命周期：

  SPAWNED ──→ RUNNING ──→ COMPLETED
                │               ↑
                ├── error ──→ FAILED
                │
                ├── StopAgent() ──→ STOPPED
                │
                └── Timeout ──→ FAILED

每个子 Agent 运行在独立的 goroutine 中：
  - 拥有独立的 QueryLoop（与父 Agent 完全隔离）
  - 通过 createSubagentContext() 继承父 Agent 的工具和权限配置
  - setAppState 为 no-op（避免污染父 Agent 状态）
  - 使用 localDenialTracking（独立的拒绝计数）
  - contentReplacementState 从父 Agent 克隆（共享缓存决策）

子 Agent 完成后：
  - 通过 taskNotificationCh 向父（Coordinator）发送 TaskNotification
  - Coordinator 将通知转为 <task-notification> XML 追加到消息历史
  - 父 Agent 的下一次 LLM 采样可看到工作结果
```

### 6.3 SendMessage 路由协议

```go
// 路由协议（对应 TS SendMessageTool）：
//
// Coordinator → Worker：
//   coordinator.SendMessage(ctx, agentID, "修复 src/auth/validate.ts:42 的空指针...")
//   → 找到 agentID 对应的 Agent goroutine
//   → 将消息追加为 UserMessage 到该 Agent 的消息队列
//   → Agent 在下一次 LLM 采样时处理该消息
//
// 消息队列机制：
//   每个子 Agent 有独立的 inboxCh (chan string)，buffer=16
//   SendMessage 发送到 inboxCh
//   Agent 的 QueryLoop 在工具执行完毕后 drain inboxCh

// Coordinator 模式专有工具（不暴露给 worker）：
//   AgentTool       → coordinator.SpawnAgent()
//   SendMessageTool → coordinator.SendMessage()
//   TaskStopTool    → coordinator.StopAgent()
//   TeamCreateTool  → 创建 worker 团队（批量 spawn）
//   TeamDeleteTool  → 解散 worker 团队（批量 stop）
```

### 6.4 协调模式系统提示注入

```go
// internal/coordinator/prompt.go

// GetCoordinatorSystemPrompt 返回 Coordinator 模式的专用系统提示。
// 对应 TS coordinatorMode.ts:getCoordinatorSystemPrompt()。
//
// 注入内容包括：
//   - Coordinator 的角色定义（编排 worker，不直接执行代码）
//   - 可用工具列表（AgentTool、SendMessageTool、TaskStopTool）
//   - Worker 能力描述（标准工具 + MCP 工具）
//   - task-notification XML 格式说明
//   - 任务工作流（Research → Synthesis → Implementation → Verification）
//   - 并发策略（只读任务自由并行，写操作串行）
//   - Worker Prompt 编写指南（synthesize findings, avoid lazy delegation）
func GetCoordinatorSystemPrompt(isSimpleMode bool) string

// GetCoordinatorUserContext 返回注入到 user context 的 worker 工具信息。
// 包含 worker 可用的工具列表 + MCP 服务名称 + scratchpad 目录。
func GetCoordinatorUserContext(mcpClients []MCPClientInfo, scratchpadDir string) map[string]string

// MCPClientInfo 是 MCP 服务的简要信息（用于注入 user context）。
type MCPClientInfo struct {
    Name string
}
```

---

## 7. 设计决策

### 7.1 tool 包为什么不依赖任何外部包？

**决策**：`internal/tool` 包仅依赖标准库（`context`、`encoding/json`、`time`）。

**原因**：
- tool 接口是系统的**依赖核心**——所有其他层（engine、permissions、hooks、tools/\*）都依赖它
- 任何对外部包的依赖都会通过传递性依赖污染所有实现层
- 保持零外部依赖使接口可以在不引入任何外部因素的情况下被独立测试
- 对应 TS 中通过 `// Import X from centralized location to break import cycles` 注释反映的设计意图

### 7.2 为什么用 json.RawMessage 而不是泛型？

**决策**：Tool 接口中 `input` 参数统一使用 `json.RawMessage`，而非 `T any` 泛型。

**原因**：
- TS 中使用了 `z.ZodType<T>` 泛型，但 Go 泛型接口存在无法添加到 slice 的限制（`[]Tool[any]` 不合法）
- `Tools []Tool` 需要同质集合，泛型接口无法满足
- `json.RawMessage` 保留了延迟解析的灵活性，工具实现内部自行解析为强类型
- 方案：工具实现内部使用辅助函数 `ParseInput[T any](raw json.RawMessage) (T, error)` 恢复类型安全

### 7.3 并发编排：为什么不用 goroutine-per-tool？

**决策**：并发工具通过 semaphore（最大 10）控制，非并发工具严格串行。

**原因**：
- 文件系统状态可变，并发写操作会产生竞争条件
- TS 源码中明确 partition 逻辑：连续的只读工具合并为并发批次，写操作独立串行
- semaphore 上限防止 goroutine 数量失控（API 连接池、文件描述符）
- 环境变量 `CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY` 允许运行时调整

### 7.4 权限系统为什么是 channel-based 而非 callback？

**决策**：ask-user 通过双向 channel（askCh + respCh）而非回调函数实现。

**原因**：
- Go 的 goroutine 模型天然适合 channel 通信
- 避免回调地狱（TS 中的 Promise 链在 Go 中等价于 goroutine + channel）
- `ctx.Done()` 可以优雅处理超时/取消，无需额外的清理逻辑
- TUI 层（Bubble Tea）本身也是 channel-driven，两者模型统一

### 7.5 Compact 为什么分三种策略而非一种？

**决策**：保留 Auto/Micro/Snip 三种独立策略，按顺序组合运行（非互斥）。

**原因**（对应 TS query.ts 主循环）：
- **Snip**：最轻量，无需 LLM，直接删除旧 tool_use 对，在 micro/auto 之前运行
- **Micro**：针对单条超大 tool_result，局部压缩，不改变消息结构
- **Auto**：全局摘要，需要 LLM 生成，上下文接近 limit 时才触发
- 三者按顺序叠加：先 snip 释放空间，再 micro 处理大结果，最后 auto 全局摘要
- 若 snip 后 token 已低于 auto 阈值，auto 可跳过（节省 LLM 调用）

---

## 8. TS → Go 行为映射

| TS 概念 | TS 位置 | Go 对应 | Go 位置 |
|---------|---------|---------|---------|
| `Tool<Input, Output, P>` interface | `src/Tool.ts` | `tool.Tool` interface | `internal/tool/tool.go` |
| `ToolUseContext` | `src/Tool.ts` | `tool.UseContext` | `internal/tool/context.go` |
| `PermissionResult` | `src/types/permissions.ts` | `tool.PermissionResult` | `internal/tool/permission.go` |
| `PermissionMode` | `src/types/permissions.ts` | `tool.PermissionMode` | `internal/tool/permission.go` |
| `buildTool()` + `TOOL_DEFAULTS` | `src/Tool.ts` | `tool.BaseTool` struct | `internal/tool/tool.go` |
| `query()` / `queryLoop()` | `src/query.ts` | `engine.QueryEngine.Query()` | `internal/engine/engine.go` |
| `runTools()` | `src/services/tools/toolOrchestration.ts` | `engine.runTools()` (internal) | `internal/engine/orchestration.go` |
| `partitionToolCalls()` | `src/services/tools/toolOrchestration.ts` | `engine.partitionToolCalls()` | `internal/engine/orchestration.go` |
| `autocompact()` | `src/services/compact/autoCompact.ts` | `compact.AutoCompactor` | `internal/compact/auto.go` |
| `microcompact()` | `src/services/compact/microCompact.ts` | `compact.MicroCompactor` | `internal/compact/micro.go` |
| `snipCompactIfNeeded()` | `src/services/compact/snipCompact.ts` | `compact.SnipCompactIfNeeded()` | `internal/compact/snip.go` |
| `HookEvent` | `src/types/hooks.ts` | `hooks.HookEvent` | `internal/hooks/types.go` |
| `AggregatedHookResult` | `src/types/hooks.ts` | `hooks.AggregatedHookResult` | `internal/hooks/executor.go` |
| `isCoordinatorMode()` | `src/coordinator/coordinatorMode.ts` | `coordinator.Coordinator.IsCoordinatorMode()` | `internal/coordinator/coordinator.go` |
| `getCoordinatorSystemPrompt()` | `src/coordinator/coordinatorMode.ts` | `coordinator.GetCoordinatorSystemPrompt()` | `internal/coordinator/prompt.go` |
| `TaskNotification` XML 协议 | `src/coordinator/coordinatorMode.ts` | `coordinator.TaskNotification` struct | `internal/coordinator/coordinator.go` |
| `DenialTrackingState` | `src/utils/permissions/denialTracking.ts` | `permissions.DenialTrackingState` | `internal/permissions/denial.go` |
| `BudgetTracker` | `src/query/tokenBudget.ts` | `engine.BudgetTracker` | `internal/engine/budget.go` |
| `QueryParams` | `src/query.ts` | `engine.QueryParams` | `internal/engine/engine.go` |
| `StreamEvent` / `Message` yield | `src/query.ts` | `engine.Msg` via channel | `internal/engine/msg.go` |
| `isConcurrencySafe` → 并发批次 | `toolOrchestration.ts` | semaphore + goroutine pool | `internal/engine/orchestration.go` |
| `contextModifier` 串行应用 | `toolOrchestration.ts` | serial apply after batch | `internal/engine/orchestration.go` |
| `PromptRequest` / `PromptResponse` | `src/types/hooks.ts` | `permissions.AskRequest/Response` | `internal/permissions/ask.go` |
| `async: true` Hook | `src/types/hooks.ts` | `hooks.HookJSONOutput.Async` | `internal/hooks/executor.go` |

---

*文档由 Agent-Core 生成，详细描述核心层的 Go 接口设计，作为 Agent-Tools 实现工具的契约依据。*
