# 基础设施层详细设计

> **文档版本**：v1.0
> **创建日期**：2026-04-02
> **作者**：Agent-Infra
> **参考**：`claude-code-main` TypeScript 源码（src/types/, src/state/, src/utils/settings/）

---

## 概述

本文档描述 `claude-code-go` 项目基础设施层（Infrastructure Layer）的详细设计。
基础设施层是整个系统的最底层，为上层的会话管理、工具执行、API 通信等模块提供类型定义、并发安全的状态管理、三级配置加载及会话持久化能力。

---

## 1. `pkg/types/` — 零依赖核心类型包

### 1.1 设计原则

- **零外部依赖**：`pkg/types` 仅使用 Go 标准库，不引入任何第三方包。
- **与 TS 源码对应**：每个类型对应 `claude-code-main/src/types/` 中的原始类型，保持语义一致。
- **使用 Go 惯用法**：TypeScript 判别联合（discriminated union）→ Go `interface` + `type switch` 或带 `Type` 字段的结构体；branded 类型 → Go 自定义字符串类型。

### 1.2 文件结构

```
pkg/types/
├── ids.go          # SessionId, AgentId 有品牌字符串类型
├── permissions.go  # PermissionMode, PermissionBehavior, PermissionRule, PermissionDecision 等
├── message.go      # Message, Role, ContentBlock 联合体
├── command.go      # Command, PromptCommand, LocalCommand 接口体系
├── logs.go         # SerializedMessage, TranscriptMessage, LogOption, Entry 联合体
├── hooks.go        # HookCallback, HookResult, AggregatedHookResult
└── plugin.go       # LoadedPlugin, PluginConfig, PluginError
```

### 1.3 `ids.go` — 有品牌 ID 类型

TypeScript 原型（`src/types/ids.ts`）：
```typescript
declare const __sessionId: unique symbol
export type SessionId = string & { [__sessionId]: true }
export type AgentId   = string & { [__agentId]: true }
```

Go 实现：
```go
package types

import (
    "fmt"
    "regexp"
)

// SessionId 是会话标识符的有类型字符串，防止与普通字符串混用。
type SessionId string

// AgentId 是 Agent 标识符的有类型字符串。
// 格式：^a(?:.+-)?[0-9a-f]{16}$
type AgentId string

var agentIdPattern = regexp.MustCompile(`^a(?:.+-)?[0-9a-f]{16}$`)

// AsSessionId 将任意字符串转换为 SessionId（无验证，保持与 TS asSessionId() 一致）。
func AsSessionId(s string) SessionId { return SessionId(s) }

// AsAgentId 将字符串转换为 AgentId，格式不合法时返回错误。
func AsAgentId(s string) (AgentId, error) {
    if !agentIdPattern.MatchString(s) {
        return "", fmt.Errorf("invalid AgentId format: %q", s)
    }
    return AgentId(s), nil
}

// NewAgentId 生成符合规范的新 AgentId（16 位 hex 随机后缀）。
func NewAgentId(prefix string) AgentId {
    // 实现见 internal/bootstrap/ids.go
    panic("see internal/bootstrap")
}
```

### 1.4 `permissions.go` — 权限类型体系

TypeScript 原型（`src/types/permissions.ts`）：
```typescript
export type PermissionMode =
  'acceptEdits' | 'bypassPermissions' | 'default' | 'dontAsk' | 'plan' | 'auto' | 'bubble'
export type PermissionBehavior = 'allow' | 'deny' | 'ask'
export type PermissionDecision<Input> =
  | PermissionAllowDecision<Input>
  | PermissionAskDecision<Input>
  | PermissionDenyDecision
```

Go 实现：
```go
package types

// PermissionMode 枚举权限策略模式。
type PermissionMode string

const (
    PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
    PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
    PermissionModeDefault           PermissionMode = "default"
    PermissionModeDontAsk           PermissionMode = "dontAsk"
    PermissionModePlan              PermissionMode = "plan"
    PermissionModeAuto              PermissionMode = "auto"
    PermissionModeBubble            PermissionMode = "bubble"
)

// PermissionBehavior 描述单次权限决策结果。
type PermissionBehavior string

const (
    BehaviorAllow PermissionBehavior = "allow"
    BehaviorDeny  PermissionBehavior = "deny"
    BehaviorAsk   PermissionBehavior = "ask"
)

// PermissionRuleSource 表示规则的来源层级。
type PermissionRuleSource string

const (
    RuleSourceUser    PermissionRuleSource = "userSettings"
    RuleSourceProject PermissionRuleSource = "projectSettings"
    RuleSourceLocal   PermissionRuleSource = "localSettings"
    RuleSourceCLI     PermissionRuleSource = "cliArg"
    RuleSourceSession PermissionRuleSource = "session"
    RuleSourceCommand PermissionRuleSource = "command"
    RuleSourcePolicy  PermissionRuleSource = "policySettings"
    RuleSourceFlag    PermissionRuleSource = "flagSettings"
)

// PermissionRuleValue 描述一条权限规则的匹配目标。
type PermissionRuleValue struct {
    ToolName    string `json:"toolName"`
    RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionRule 是一条具体的权限规则。
type PermissionRule struct {
    Source       PermissionRuleSource `json:"source"`
    RuleBehavior PermissionBehavior   `json:"ruleBehavior"`
    RuleValue    PermissionRuleValue  `json:"ruleValue"`
}

// PermissionDecision 是权限检查的结果（判别联合体）。
// 通过 Behavior 字段区分 allow / ask / deny / passthrough。
type PermissionDecision struct {
    Behavior    PermissionBehavior `json:"behavior"`
    Message     string             `json:"message,omitempty"`
    // allow 专属
    UserModified bool `json:"userModified,omitempty"`
    // ask 专属
    Suggestions []PermissionUpdate `json:"suggestions,omitempty"`
    BlockedPath string             `json:"blockedPath,omitempty"`
}

// PermissionUpdate 表示一次权限配置变更操作。
type PermissionUpdate struct {
    Type        string                    `json:"type"` // addRules|replaceRules|removeRules|setMode|addDirectories|removeDirectories
    Destination string                    `json:"destination"`
    Rules       []PermissionRuleValue     `json:"rules,omitempty"`
    Behavior    PermissionBehavior        `json:"behavior,omitempty"`
    Mode        PermissionMode            `json:"mode,omitempty"`
    Directories []string                  `json:"directories,omitempty"`
}

// ToolPermissionRulesBySource 按来源分组的工具权限规则集合。
type ToolPermissionRulesBySource map[PermissionRuleSource][]string

// ToolPermissionContext 是工具执行时的权限上下文快照（只读）。
type ToolPermissionContext struct {
    Mode                          PermissionMode
    AdditionalWorkingDirectories  map[string]AdditionalWorkingDirectory
    AlwaysAllowRules              ToolPermissionRulesBySource
    AlwaysDenyRules               ToolPermissionRulesBySource
    AlwaysAskRules                ToolPermissionRulesBySource
    IsBypassPermissionsModeAvailable bool
}

// AdditionalWorkingDirectory 是权限范围内的额外工作目录。
type AdditionalWorkingDirectory struct {
    Path   string               `json:"path"`
    Source PermissionRuleSource `json:"source"`
}
```

### 1.5 `message.go` — 消息类型

Go 实现（对应 Anthropic SDK 的 Message 结构）：
```go
package types

import "time"

// Role 对应 Anthropic API 的消息角色。
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

// ContentBlockType 标识内容块类型（判别联合体 tag）。
type ContentBlockType string

const (
    ContentTypeText       ContentBlockType = "text"
    ContentTypeImage      ContentBlockType = "image"
    ContentTypeToolUse    ContentBlockType = "tool_use"
    ContentTypeToolResult ContentBlockType = "tool_result"
    ContentTypeThinking   ContentBlockType = "thinking"
)

// ContentBlock 是一个泛化的消息内容块。
// 具体字段按 Type 区分，使用指针字段避免零值歧义。
type ContentBlock struct {
    Type  ContentBlockType `json:"type"`
    // text
    Text  *string `json:"text,omitempty"`
    // tool_use
    ID    *string          `json:"id,omitempty"`
    Name  *string          `json:"name,omitempty"`
    Input map[string]any   `json:"input,omitempty"`
    // tool_result
    ToolUseID *string        `json:"tool_use_id,omitempty"`
    Content   []ContentBlock `json:"content,omitempty"`
    IsError   *bool          `json:"is_error,omitempty"`
    // image
    Source *ImageSource `json:"source,omitempty"`
    // thinking
    Thinking  *string `json:"thinking,omitempty"`
    Signature *string `json:"signature,omitempty"`
}

// ImageSource 描述图片的来源。
type ImageSource struct {
    Type      string `json:"type"`       // base64 | url
    MediaType string `json:"media_type,omitempty"`
    Data      string `json:"data,omitempty"`
    URL       string `json:"url,omitempty"`
}

// Message 对应一条完整的对话消息。
type Message struct {
    ID      string         `json:"id,omitempty"`
    Role    Role           `json:"role"`
    Content []ContentBlock `json:"content"`
    // 元数据（仅本地使用，API 序列化时忽略）
    UUID      string    `json:"uuid,omitempty"`
    Timestamp time.Time `json:"timestamp,omitempty"`
    SessionId SessionId `json:"session_id,omitempty"`
}
```

### 1.6 `logs.go` — 会话日志与 Entry 联合体

TypeScript 原型（`src/types/logs.ts`）：
```typescript
export type Entry =
  | TranscriptMessage | SummaryMessage | TagMessage
  | PRLinkMessage | WorktreeStateEntry | ...  // 20+ 变体
```

Go 实现（使用 `Type` 字段 + JSON `RawMessage` 延迟解码）：
```go
package types

import (
    "encoding/json"
    "time"
)

// EntryType 枚举 JSONL 日志中的条目类型。
type EntryType string

const (
    EntryTypeTranscript   EntryType = "transcript"
    EntryTypeSummary      EntryType = "summary"
    EntryTypeTag          EntryType = "tag"
    EntryTypePRLink       EntryType = "pr_link"
    EntryTypeWorktree     EntryType = "worktree_state"
    // ... 其余类型
)

// EntryEnvelope 是 JSONL 文件中一行的通用信封，用于快速判断类型后再解码。
type EntryEnvelope struct {
    Type EntryType `json:"type"`
    Raw  json.RawMessage
}

// SerializedMessage 是持久化到 JSONL 的消息快照。
type SerializedMessage struct {
    Message
    CWD       string    `json:"cwd"`
    UserType  string    `json:"userType"`
    SessionId SessionId `json:"sessionId"`
    Timestamp time.Time `json:"timestamp"`
    Version   string    `json:"version"`
    GitBranch string    `json:"gitBranch,omitempty"`
    Slug      string    `json:"slug,omitempty"`
}

// TranscriptMessage 是带有多链路信息的完整消息记录。
type TranscriptMessage struct {
    SerializedMessage
    ParentUUID  string    `json:"parentUuid,omitempty"`
    IsSidechain bool      `json:"isSidechain,omitempty"`
    AgentId     AgentId   `json:"agentId,omitempty"`
}

// LogOption 是 /resume 选择器中显示的会话摘要。
type LogOption struct {
    SessionId   SessionId `json:"sessionId"`
    Date        time.Time `json:"date"`
    FirstPrompt string    `json:"firstPrompt"`
    IsSidechain bool      `json:"isSidechain,omitempty"`
    Messages    []SerializedMessage `json:"messages,omitempty"`
    Title       string    `json:"title,omitempty"`
    GitBranch   string    `json:"gitBranch,omitempty"`
}

// SummaryEntry 是压缩会话摘要条目。
type SummaryEntry struct {
    Type    EntryType `json:"type"` // "summary"
    Summary string    `json:"summary"`
    LeafId  string    `json:"leafId,omitempty"`
}
```

### 1.7 `command.go` — 命令类型体系

```go
package types

// CommandType 标识命令实现方式。
type CommandType string

const (
    CommandTypePrompt   CommandType = "prompt"
    CommandTypeLocal    CommandType = "local"
    CommandTypeLocalJSX CommandType = "local-jsx"  // Go 等价：TUI 组件回调
)

// CommandSource 表示命令来源。
type CommandSource string

const (
    CommandSourceBuiltin CommandSource = "builtin"
    CommandSourceMCP     CommandSource = "mcp"
    CommandSourcePlugin  CommandSource = "plugin"
    CommandSourceSkills  CommandSource = "skills"
    CommandSourceBundled CommandSource = "bundled"
)

// CommandBase 是所有命令的公共元数据。
type CommandBase struct {
    Name        string        `json:"name"`
    Description string        `json:"description"`
    Type        CommandType   `json:"type"`
    Source      CommandSource `json:"source,omitempty"`
    Aliases     []string      `json:"aliases,omitempty"`
    IsHidden    bool          `json:"isHidden,omitempty"`
    IsMCP       bool          `json:"isMcp,omitempty"`
    ArgumentHint string       `json:"argumentHint,omitempty"`
    WhenToUse   string        `json:"whenToUse,omitempty"`
    Version     string        `json:"version,omitempty"`
    Immediate   bool          `json:"immediate,omitempty"`
    IsSensitive bool          `json:"isSensitive,omitempty"`
}

// PromptCommand 是基于提示词模板的命令。
type PromptCommand struct {
    CommandBase
    ContentLength int      `json:"contentLength"`
    ArgNames      []string `json:"argNames,omitempty"`
    AllowedTools  []string `json:"allowedTools,omitempty"`
    Model         string   `json:"model,omitempty"`
    // context: "inline" | "fork"
    Context string `json:"context,omitempty"`
    Agent   string `json:"agent,omitempty"`
    // GetPrompt 在运行时提供扩展后的提示词内容（非序列化字段）。
    GetPrompt func(args string) ([]ContentBlock, error) `json:"-"`
}

// LocalCommand 是由 Go 函数直接实现的命令。
type LocalCommand struct {
    CommandBase
    SupportsNonInteractive bool
    // Call 是命令的执行函数。
    Call func(args string, ctx *LocalCommandContext) (*LocalCommandResult, error) `json:"-"`
}

// LocalCommandResult 是本地命令的执行结果。
type LocalCommandResult struct {
    Type string // "text" | "compact" | "skip"
    Value string
}

// LocalCommandContext 提供本地命令执行所需的上下文（类似 TS LocalJSXCommandContext）。
type LocalCommandContext struct {
    // 依赖注入的核心服务（接口，避免循环依赖）
    AppState    AppStateReader
    SessionId   SessionId
    WorkingDir  string
}

// AppStateReader 是只读的 AppState 访问接口，供命令等低层模块使用。
type AppStateReader interface {
    GetPermissionContext() ToolPermissionContext
    GetModel() string
    GetVerbose() bool
}
```

---

## 2. `pkg/utils/` — 通用工具函数包

### 2.1 设计原则

- **无状态纯函数**：所有工具函数均为纯函数，无全局状态。
- **零或最小外部依赖**：优先使用标准库。
- **与 `pkg/types` 的单向依赖**：`pkg/utils` 可依赖 `pkg/types`，反之不可。

### 2.2 文件结构

```
pkg/utils/
├── ids/
│   └── ids.go          # NewSessionId(), NewAgentId() 生成函数
├── env/
│   └── env.go          # GetEnv(), IsEnvTruthy() 等环境变量工具
├── fs/
│   └── fs.go           # SafeReadFile(), AtomicWriteFile(), EnsureDir()
├── json/
│   └── json.go         # MarshalLine(), UnmarshalLine() JSONL 辅助
└── permission/
    └── matcher.go      # MatchPermissionRule() 规则匹配函数
```

### 2.3 `ids/ids.go`

```go
package ids

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "time"

    "github.com/your-org/claude-code-go/pkg/types"
)

// NewSessionId 生成一个新的 SessionId（时间戳 + 随机 hex）。
func NewSessionId() types.SessionId {
    b := make([]byte, 16)
    _, _ = rand.Read(b)
    return types.SessionId(fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:8])))
}

// NewAgentId 生成一个符合 ^a(?:.+-)?[0-9a-f]{16}$ 格式的 AgentId。
func NewAgentId(prefix string) types.AgentId {
    b := make([]byte, 8)
    _, _ = rand.Read(b)
    suffix := hex.EncodeToString(b)
    if prefix == "" {
        return types.AgentId("a" + suffix)
    }
    return types.AgentId(fmt.Sprintf("a%s-%s", prefix, suffix))
}
```

### 2.4 `fs/fs.go`

```go
package fs

import (
    "os"
    "path/filepath"
)

// EnsureDir 确保目录存在（mkdir -p 语义）。
func EnsureDir(path string) error {
    return os.MkdirAll(path, 0o755)
}

// AtomicWriteFile 将数据原子性地写入文件（先写临时文件，再 rename）。
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".tmp-")
    if err != nil {
        return err
    }
    tmpName := tmp.Name()
    if _, err = tmp.Write(data); err != nil {
        _ = tmp.Close()
        _ = os.Remove(tmpName)
        return err
    }
    if err = tmp.Close(); err != nil {
        _ = os.Remove(tmpName)
        return err
    }
    if err = os.Chmod(tmpName, perm); err != nil {
        _ = os.Remove(tmpName)
        return err
    }
    return os.Rename(tmpName, path)
}
```

### 2.5 `json/json.go` — JSONL 行辅助

```go
package json

import (
    stdjson "encoding/json"
)

// MarshalLine 将值序列化为一行 JSON（不含换行符）。
func MarshalLine(v any) ([]byte, error) {
    return stdjson.Marshal(v)
}

// AppendLine 将值追加为一行 JSONL 到字节切片。
func AppendLine(buf []byte, v any) ([]byte, error) {
    line, err := stdjson.Marshal(v)
    if err != nil {
        return buf, err
    }
    buf = append(buf, line...)
    buf = append(buf, '\n')
    return buf, nil
}
```

---

## 3. `internal/config/` — 三级配置加载

### 3.1 配置层级

对应 TS 的三级 settings 体系：

| 层级 | 路径 | 说明 | 优先级 |
|------|------|------|--------|
| User（全局） | `~/.claude/settings.json` | 用户级别，跨项目生效 | 最低 |
| Project（项目） | `<cwd>/.claude/settings.json` | 项目内共享，可提交 | 中 |
| Local（本地） | `<cwd>/.claude.local/settings.json` | 本地覆盖，不提交（.gitignore） | 最高 |

额外：企业托管配置 `managed-settings.json`（策略级，优先级最高，用于企业管控）。

### 3.2 `SettingsJson` 结构

对应 TS `SettingsSchema`（`src/utils/settings/types.ts`）：

```go
package config

import "github.com/your-org/claude-code-go/pkg/types"

// PermissionsConfig 对应 settings.json 的 permissions 字段。
type PermissionsConfig struct {
    Allow              []string               `json:"allow,omitempty"`
    Deny               []string               `json:"deny,omitempty"`
    Ask                []string               `json:"ask,omitempty"`
    DefaultMode        types.PermissionMode   `json:"defaultMode,omitempty"`
    DisableBypass      string                 `json:"disableBypassPermissionsMode,omitempty"`
    AdditionalDirs     []string               `json:"additionalDirectories,omitempty"`
}

// WorktreeConfig 对应 settings.json 的 worktree 字段。
type WorktreeConfig struct {
    SymlinkDirectories []string `json:"symlinkDirectories,omitempty"`
    SparsePaths        []string `json:"sparsePaths,omitempty"`
}

// AttributionConfig 对应 settings.json 的 attribution 字段。
type AttributionConfig struct {
    Commit string `json:"commit,omitempty"`
    PR     string `json:"pr,omitempty"`
}

// SettingsJson 对应完整的 settings.json 文件结构。
// 使用 omitempty 保证未设置的字段不写回文件（向后兼容）。
type SettingsJson struct {
    Schema               string             `json:"$schema,omitempty"`
    APIKeyHelper         string             `json:"apiKeyHelper,omitempty"`
    AWSCredentialExport  string             `json:"awsCredentialExport,omitempty"`
    AWSAuthRefresh       string             `json:"awsAuthRefresh,omitempty"`
    GCPAuthRefresh       string             `json:"gcpAuthRefresh,omitempty"`
    RespectGitignore     *bool              `json:"respectGitignore,omitempty"`
    CleanupPeriodDays    *int               `json:"cleanupPeriodDays,omitempty"`
    Env                  map[string]string  `json:"env,omitempty"`
    Attribution          *AttributionConfig `json:"attribution,omitempty"`
    Permissions          *PermissionsConfig `json:"permissions,omitempty"`
    Model                string             `json:"model,omitempty"`
    AvailableModels      []string           `json:"availableModels,omitempty"`
    ModelOverrides       map[string]string  `json:"modelOverrides,omitempty"`
    EnableAllProjectMCP  *bool              `json:"enableAllProjectMcpServers,omitempty"`
    EnabledMCPServers    []string           `json:"enabledMcpjsonServers,omitempty"`
    DisabledMCPServers   []string           `json:"disabledMcpjsonServers,omitempty"`
    Hooks                map[string]any     `json:"hooks,omitempty"`
    Worktree             *WorktreeConfig    `json:"worktree,omitempty"`
    DisableAllHooks      *bool              `json:"disableAllHooks,omitempty"`
    DefaultShell         string             `json:"defaultShell,omitempty"`
    AllowManagedHooksOnly *bool             `json:"allowManagedHooksOnly,omitempty"`
}
```

### 3.3 `Loader` — 配置加载器

```go
package config

import (
    "encoding/json"
    "os"
    "path/filepath"
)

// SettingSource 标识配置来源层级。
type SettingSource string

const (
    SourceUser    SettingSource = "userSettings"
    SourceProject SettingSource = "projectSettings"
    SourceLocal   SettingSource = "localSettings"
    SourcePolicy  SettingSource = "policySettings"
)

// LayeredSettings 持有所有层级的原始配置和合并后的有效配置。
type LayeredSettings struct {
    User    *SettingsJson
    Project *SettingsJson
    Local   *SettingsJson
    Policy  *SettingsJson // managed-settings.json（企业策略）
    Merged  *SettingsJson // 合并后的有效配置（Local > Project > User > Policy）
}

// Loader 负责从文件系统加载三级配置。
type Loader struct {
    homeDir    string
    projectDir string
}

// NewLoader 创建配置加载器。
func NewLoader(homeDir, projectDir string) *Loader {
    return &Loader{homeDir: homeDir, projectDir: projectDir}
}

// Load 加载并合并三级配置，返回 LayeredSettings。
func (l *Loader) Load() (*LayeredSettings, error) {
    ls := &LayeredSettings{}

    paths := map[SettingSource]string{
        SourceUser:    filepath.Join(l.homeDir, ".claude", "settings.json"),
        SourceProject: filepath.Join(l.projectDir, ".claude", "settings.json"),
        SourceLocal:   filepath.Join(l.projectDir, ".claude.local", "settings.json"),
        SourcePolicy:  filepath.Join(l.homeDir, ".claude", "managed-settings.json"),
    }

    ptrs := map[SettingSource]**SettingsJson{
        SourceUser:    &ls.User,
        SourceProject: &ls.Project,
        SourceLocal:   &ls.Local,
        SourcePolicy:  &ls.Policy,
    }

    for src, path := range paths {
        data, err := os.ReadFile(path)
        if os.IsNotExist(err) {
            continue // 允许层级文件缺失
        }
        if err != nil {
            return nil, err
        }
        s := &SettingsJson{}
        if err := json.Unmarshal(data, s); err != nil {
            return nil, err
        }
        *ptrs[src] = s
    }

    ls.Merged = mergeSettings(ls.Policy, ls.User, ls.Project, ls.Local)
    return ls, nil
}

// mergeSettings 按优先级从低到高合并配置（后者覆盖前者）。
// 规则：非零值字段覆盖零值字段；Permissions.Allow/Deny/Ask 数组合并（去重）。
func mergeSettings(layers ...*SettingsJson) *SettingsJson {
    merged := &SettingsJson{}
    for _, layer := range layers {
        if layer == nil {
            continue
        }
        applyLayer(merged, layer)
    }
    return merged
}

// applyLayer 将 src 的非零字段覆写到 dst。
func applyLayer(dst, src *SettingsJson) {
    if src.Model != "" {
        dst.Model = src.Model
    }
    if src.APIKeyHelper != "" {
        dst.APIKeyHelper = src.APIKeyHelper
    }
    if src.DefaultShell != "" {
        dst.DefaultShell = src.DefaultShell
    }
    if src.RespectGitignore != nil {
        dst.RespectGitignore = src.RespectGitignore
    }
    if src.CleanupPeriodDays != nil {
        dst.CleanupPeriodDays = src.CleanupPeriodDays
    }
    // env 合并（src 覆盖 dst 同名键）
    if len(src.Env) > 0 {
        if dst.Env == nil {
            dst.Env = make(map[string]string)
        }
        for k, v := range src.Env {
            dst.Env[k] = v
        }
    }
    // permissions 合并
    if src.Permissions != nil {
        if dst.Permissions == nil {
            dst.Permissions = &PermissionsConfig{}
        }
        mergePermissions(dst.Permissions, src.Permissions)
    }
    // ... 其余字段类似处理
}

func mergePermissions(dst, src *PermissionsConfig) {
    dst.Allow = uniqueAppend(dst.Allow, src.Allow...)
    dst.Deny  = uniqueAppend(dst.Deny,  src.Deny...)
    dst.Ask   = uniqueAppend(dst.Ask,   src.Ask...)
    if src.DefaultMode != "" {
        dst.DefaultMode = src.DefaultMode
    }
    if src.DisableBypass != "" {
        dst.DisableBypass = src.DisableBypass
    }
    dst.AdditionalDirs = uniqueAppend(dst.AdditionalDirs, src.AdditionalDirs...)
}

func uniqueAppend(dst []string, src ...string) []string {
    seen := make(map[string]struct{}, len(dst))
    for _, v := range dst {
        seen[v] = struct{}{}
    }
    for _, v := range src {
        if _, ok := seen[v]; !ok {
            dst = append(dst, v)
            seen[v] = struct{}{}
        }
    }
    return dst
}
```

### 3.4 配置文件路径常量

```go
package config

const (
    ClaudeDir           = ".claude"
    ClaudeLocalDir      = ".claude.local"
    SettingsFile        = "settings.json"
    ManagedSettingsFile = "managed-settings.json"

    // 全局 Claude 主目录（~/.claude/）中的子目录
    SessionsDir = "projects"  // 会话 JSONL 存储
    TodosDir    = "todos"
    StatsFile   = "statsig.json"
)
```

---

## 4. `internal/state/` — 并发安全应用状态

### 4.1 设计目标

对应 TS 的 `Store<AppState>` + `AppStateStore`（`src/state/store.ts`, `src/state/AppStateStore.ts`）。

**TS 原型**：
```typescript
export type Store<T> = {
  getState: () => T
  setState: (updater: (prev: T) => T) => void
  subscribe: (listener: Listener) => () => void
}
```

**Go 实现策略**：
- `sync.RWMutex` 保护状态读写
- `setState` 接受 `func(*AppState)` mutator（就地修改，避免深拷贝）
- 订阅者用函数切片 + `sync.Mutex` 保护；通知在锁释放后异步调用（防死锁）

### 4.2 通用 Store

```go
package state

import "sync"

// Listener 是状态变更的订阅回调。
type Listener[T any] func(newState, oldState T)

// Store 是线程安全的泛型状态容器。
type Store[T any] struct {
    mu        sync.RWMutex
    state     T
    listenerMu sync.Mutex
    listeners  []Listener[T]
    onChange   func(newState, oldState T)
}

// NewStore 创建一个新的 Store，使用 initialState 初始化。
func NewStore[T any](initialState T, onChange func(newState, oldState T)) *Store[T] {
    return &Store[T]{state: initialState, onChange: onChange}
}

// GetState 返回当前状态的快照（读锁保护）。
func (s *Store[T]) GetState() T {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.state
}

// SetState 使用 updater 函数更新状态，并通知所有订阅者。
// updater 在写锁保护下执行，不能在 updater 内调用 GetState/SetState（避免死锁）。
func (s *Store[T]) SetState(updater func(prev T) T) {
    s.mu.Lock()
    prev := s.state
    next := updater(prev)
    s.state = next
    s.mu.Unlock()

    // 在锁外触发回调，防止回调中再次加锁造成死锁
    if s.onChange != nil {
        s.onChange(next, prev)
    }
    s.notifyListeners(next, prev)
}

// Subscribe 注册状态变更监听器，返回取消订阅函数。
func (s *Store[T]) Subscribe(l Listener[T]) func() {
    s.listenerMu.Lock()
    s.listeners = append(s.listeners, l)
    idx := len(s.listeners) - 1
    s.listenerMu.Unlock()

    return func() {
        s.listenerMu.Lock()
        defer s.listenerMu.Unlock()
        // 用 nil 标记已取消，避免切片重组
        s.listeners[idx] = nil
    }
}

func (s *Store[T]) notifyListeners(newState, oldState T) {
    s.listenerMu.Lock()
    ls := make([]Listener[T], len(s.listeners))
    copy(ls, s.listeners)
    s.listenerMu.Unlock()

    for _, l := range ls {
        if l != nil {
            l(newState, oldState)
        }
    }
}
```

### 4.3 `AppState` 结构

对应 TS 的 `AppState`（`src/state/AppStateStore.ts`），去除 React 专用字段（如 `speculation`、JSX 回调），保留运行时核心状态：

```go
package state

import (
    "sync"

    "github.com/your-org/claude-code-go/pkg/types"
    "github.com/your-org/claude-code-go/internal/config"
)

// ModelSetting 描述当前使用的模型配置。
type ModelSetting struct {
    ModelID  string `json:"modelId"`
    Provider string `json:"provider,omitempty"` // anthropic | bedrock | vertex
}

// TaskState 表示单个 Task（子 Agent）的运行状态。
type TaskState struct {
    AgentId   types.AgentId `json:"agentId"`
    Status    string        `json:"status"` // running | done | error
    SessionId types.SessionId `json:"sessionId,omitempty"`
}

// AppState 是全局应用状态，所有字段均在 AppStateStore 的 RWMutex 保护下访问。
type AppState struct {
    // 配置与模型
    Settings              config.SettingsJson          `json:"settings"`
    Verbose               bool                         `json:"verbose"`
    MainLoopModel         ModelSetting                 `json:"mainLoopModel"`
    ToolPermissionContext types.ToolPermissionContext  `json:"toolPermissionContext"`

    // 会话
    SessionId    types.SessionId `json:"sessionId"`
    WorkingDir   string          `json:"workingDir"`
    GitBranch    string          `json:"gitBranch,omitempty"`

    // 任务树（子 Agent）
    // key: taskId（string），value: TaskState
    // 注意：Tasks 本身是 map，由外层 AppStateStore.mu 保护，无需额外锁。
    Tasks map[string]TaskState `json:"tasks,omitempty"`

    // AgentId 注册表（name -> AgentId）
    AgentNameRegistry map[string]types.AgentId `json:"agentNameRegistry,omitempty"`

    // MCP 客户端/工具/命令（运行时对象，不序列化）
    MCPClients   []any `json:"-"` // *mcp.ServerConnection
    MCPTools     []any `json:"-"` // *types.Tool
    MCPCommands  []any `json:"-"` // *types.Command

    // 插件
    PluginsEnabled  []any `json:"-"` // *types.LoadedPlugin
    PluginsDisabled []any `json:"-"`

    // UI 相关（非 React，用于 TUI 状态）
    IsLoading       bool   `json:"isLoading"`
    InputPlaceholder string `json:"inputPlaceholder,omitempty"`
}

// AppStateStore 是 AppState 的并发安全存储。
type AppStateStore = Store[AppState]

// NewAppStateStore 创建并初始化 AppStateStore。
func NewAppStateStore(initial AppState) *AppStateStore {
    return NewStore(initial, nil)
}

// GetDefaultAppState 返回零值 AppState（对应 TS getDefaultAppState()）。
func GetDefaultAppState() AppState {
    return AppState{
        Tasks:             make(map[string]TaskState),
        AgentNameRegistry: make(map[string]types.AgentId),
        ToolPermissionContext: types.ToolPermissionContext{
            Mode:                         types.PermissionModeDefault,
            AdditionalWorkingDirectories: make(map[string]types.AdditionalWorkingDirectory),
            AlwaysAllowRules:             make(types.ToolPermissionRulesBySource),
            AlwaysDenyRules:              make(types.ToolPermissionRulesBySource),
            AlwaysAskRules:               make(types.ToolPermissionRulesBySource),
        },
    }
}
```

### 4.4 状态更新惯例

```go
// 示例：更新权限模式
store.SetState(func(prev AppState) AppState {
    prev.ToolPermissionContext.Mode = types.PermissionModeBypassPermissions
    return prev
})

// 示例：添加任务
store.SetState(func(prev AppState) AppState {
    if prev.Tasks == nil {
        prev.Tasks = make(map[string]TaskState)
    }
    prev.Tasks[taskId] = TaskState{AgentId: agentId, Status: "running"}
    return prev
})
```

> **注意**：由于 Go map 和 slice 是引用类型，`updater` 函数中直接修改 map/slice 时需确保不产生竞争。最安全的方式是在 `SetState` 中使用 `deepCopy` 或采用 copy-on-write 策略。中期可考虑使用 `github.com/google/go-cmp` 配合结构体深拷贝。

---

## 5. `internal/session/` — 会话持久化

### 5.1 存储格式

- **JSONL（JSON Lines）**：每条 `Entry` 序列化为一行 JSON，追加写入。
- **存储路径**：`~/.claude/projects/<project-hash>/<session-id>.jsonl`
- **文件命名**：`<ISO8601时间戳>_<slug>.jsonl`

### 5.2 文件结构

```
internal/session/
├── store.go     # SessionStore — JSONL 读写
├── manager.go   # SessionManager — 会话生命周期管理
├── resume.go    # 会话恢复逻辑（对应 TS --resume 功能）
└── cleanup.go   # 过期会话清理（cleanupPeriodDays）
```

### 5.3 `SessionStore`

```go
package session

import (
    "bufio"
    "encoding/json"
    "os"
    "sync"

    "github.com/your-org/claude-code-go/pkg/types"
    utiljson "github.com/your-org/claude-code-go/pkg/utils/json"
)

// SessionStore 负责单个会话的 JSONL 文件读写。
type SessionStore struct {
    path string
    mu   sync.Mutex
    file *os.File
}

// OpenSessionStore 打开（或创建）会话存储文件。
func OpenSessionStore(path string) (*SessionStore, error) {
    f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return nil, err
    }
    return &SessionStore{path: path, file: f}, nil
}

// AppendEntry 将一条 Entry 追加写入 JSONL 文件。
func (s *SessionStore) AppendEntry(entry any) error {
    line, err := json.Marshal(entry)
    if err != nil {
        return err
    }
    s.mu.Lock()
    defer s.mu.Unlock()
    _, err = s.file.Write(append(line, '\n'))
    return err
}

// ReadAll 读取并解析所有 Entry（用于会话恢复）。
func (s *SessionStore) ReadAll() ([]types.EntryEnvelope, error) {
    f, err := os.Open(s.path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var entries []types.EntryEnvelope
    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4MB 行缓冲
    for scanner.Scan() {
        line := scanner.Bytes()
        if len(line) == 0 {
            continue
        }
        var env types.EntryEnvelope
        if err := json.Unmarshal(line, &env); err != nil {
            continue // 跳过损坏行，保证鲁棒性
        }
        env.Raw = json.RawMessage(line)
        entries = append(entries, env)
    }
    return entries, scanner.Err()
}

// Close 关闭底层文件。
func (s *SessionStore) Close() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.file.Close()
}
```

### 5.4 会话恢复流程

```
用户 --resume <session-id>
        │
        ▼
SessionManager.LoadSession(sessionId)
        │
        ▼
SessionStore.ReadAll()  → []EntryEnvelope
        │
        ▼
ResumeBuilder.Rebuild() → []types.Message  (从 TranscriptMessage 重建对话历史)
        │
        ▼
AppStateStore.SetState() → 注入恢复的 Messages 和 SessionId
        │
        ▼
继续对话
```

---

## 6. `internal/bootstrap/` — 启动引导

### 6.1 职责

- 解析 CLI 参数（`--verbose`, `--model`, `--permission-mode`, `--resume`, etc.）
- 加载三级配置（`config.Loader`）
- 初始化 `AppStateStore`（`GetDefaultAppState()` + 配置注入）
- 初始化 `SessionStore`（新建或恢复）
- 初始化 MCP 客户端（延后至需要时）
- 返回就绪的 `App` 对象供主循环使用

### 6.2 文件结构

```
internal/bootstrap/
├── bootstrap.go   # Bootstrap() 入口函数
├── cli.go         # CLI 参数解析（flag 包）
├── ids.go         # NewAgentId() 实现
└── app.go         # App 结构体定义
```

### 6.3 `App` 结构体

```go
package bootstrap

import (
    "github.com/your-org/claude-code-go/internal/config"
    "github.com/your-org/claude-code-go/internal/session"
    "github.com/your-org/claude-code-go/internal/state"
    "github.com/your-org/claude-code-go/pkg/types"
)

// App 是启动完成后的应用根对象，持有所有核心服务引用。
type App struct {
    State      *state.AppStateStore
    Config     *config.LayeredSettings
    Session    *session.SessionManager
    SessionId  types.SessionId
    WorkingDir string
}

// CLIFlags 保存命令行参数解析结果。
type CLIFlags struct {
    Verbose        bool
    Model          string
    PermissionMode types.PermissionMode
    Resume         string // session-id or empty
    WorkingDir     string
    NonInteractive bool
    PrintOutput    bool
    MaxTurns       int
}

// Bootstrap 执行完整的启动序列，返回就绪的 App。
func Bootstrap(flags CLIFlags) (*App, error) {
    // 1. 加载配置
    loader := config.NewLoader(homeDir(), flags.WorkingDir)
    settings, err := loader.Load()
    if err != nil {
        return nil, err
    }

    // 2. 初始化状态
    initial := state.GetDefaultAppState()
    initial.Settings = *settings.Merged
    initial.Verbose = flags.Verbose
    initial.WorkingDir = flags.WorkingDir
    if flags.Model != "" {
        initial.MainLoopModel = state.ModelSetting{ModelID: flags.Model}
    }
    if flags.PermissionMode != "" {
        initial.ToolPermissionContext.Mode = flags.PermissionMode
    }
    store := state.NewAppStateStore(initial)

    // 3. 初始化会话
    var sessionId types.SessionId
    var mgr *session.SessionManager
    if flags.Resume != "" {
        sessionId, mgr, err = session.Resume(flags.Resume, flags.WorkingDir)
    } else {
        sessionId, mgr, err = session.New(flags.WorkingDir)
    }
    if err != nil {
        return nil, err
    }

    return &App{
        State:      store,
        Config:     settings,
        Session:    mgr,
        SessionId:  sessionId,
        WorkingDir: flags.WorkingDir,
    }, nil
}
```

---

## 7. 设计决策

### 7.1 泛型 Store vs 具体类型 Store

**决策**：使用 Go 1.18+ 泛型实现 `Store[T]`，`AppStateStore = Store[AppState]`。

**理由**：
- 与 TS 原型一一对应，便于维护和对比。
- 泛型版本可复用于其他状态（如 `TaskStore`、`PluginStore`）。
- 编译期类型安全，无需 `interface{}` 类型断言。

### 7.2 `SetState` 设计：updater 函数 vs 直接赋值

**决策**：采用 `func(prev T) T` updater 模式（与 TS 完全一致）。

**理由**：
- 强制显式的状态转换，便于追踪和测试。
- 与 TS `setState(prev => ({...prev, ...}))` 语义一致。
- 在写锁保护下执行 updater，防止并发竞争。

### 7.3 深拷贝策略

**决策**：Phase 1 使用浅拷贝（updater 返回修改后的结构体值），map/slice 字段采用引用语义。
**Phase 2** 引入 `deepcopy` 或 copy-on-write（待定）。

**理由**：深拷贝在状态较大时有性能开销。Go 结构体值语义已提供基本安全性；map 并发访问由 Store 的 `sync.RWMutex` 保护。

### 7.4 配置合并：数组追加 vs 覆盖

**决策**：`permissions.allow/deny/ask` 数组在各层间**追加去重**；标量字段**覆盖**。

**理由**：与 TS 源码注释一致（"Arrays merge across settings sources"）。允许项目级和用户级同时生效，更灵活。

### 7.5 JSONL vs SQLite 会话存储

**决策**：Phase 1 使用 JSONL，与 TS 原型保持一致，便于与现有会话文件互操作。

**理由**：
- JSONL 人类可读，便于调试和迁移。
- 原始 TS 实现使用 JSONL（`src/types/logs.ts` 的 Entry 类型直接映射 JSONL 行）。
- SQLite 可作为 Phase 2 优化选项（提供更好的查询能力）。

### 7.6 判别联合体的 Go 表达

**决策**：使用带 `Type` 字段的结构体 + `json.RawMessage` 延迟解码（用于 Entry 联合体）；使用 Go 接口 + 类型断言（用于命令体系）。

**理由**：
- JSONL Entry 类型较多（20+），接口方式需要大量类型断言，不如 `RawMessage` 灵活。
- `PermissionDecision` 等字段数量有限，使用 `Behavior` 字段区分即可（避免接口转换的冗余）。

---

## 8. 模块依赖关系图

```
┌─────────────────────────────────────────────────────────────────┐
│                        cmd/claude-code/                         │
│                    (main 入口，CLI 解析)                         │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    internal/bootstrap/                          │
│        (Bootstrap, App, CLIFlags, 启动序列编排)                  │
└──────┬───────────────┬────────────────────┬─────────────────────┘
       │               │                    │
       ▼               ▼                    ▼
┌──────────────┐ ┌─────────────┐ ┌──────────────────────┐
│internal/     │ │internal/    │ │internal/session/     │
│config/       │ │state/       │ │(SessionStore,        │
│(Loader,      │ │(Store[T],   │ │ SessionManager,      │
│ SettingsJson,│ │ AppState,   │ │ Resume, Cleanup)     │
│ mergeSettings│ │ AppStateStore│ │                     │
│)             │ │)            │ └──────────┬───────────┘
└──────┬───────┘ └──────┬──────┘            │
       │                │                   │
       └────────────────┴───────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                         pkg/types/                              │
│  ids.go  permissions.go  message.go  command.go  logs.go        │
│  hooks.go  plugin.go                                            │
│                    (零依赖核心类型)                               │
└──────────────────────────┬──────────────────────────────────────┘
                           │  (被 pkg/utils 单向依赖)
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                         pkg/utils/                              │
│   ids/  env/  fs/  json/  permission/                           │
│               (通用工具函数，无状态)                              │
└─────────────────────────────────────────────────────────────────┘

依赖规则（单向，禁止循环）：
  cmd → internal/bootstrap → internal/{config,state,session}
  internal/* → pkg/types
  pkg/utils  → pkg/types
  pkg/types  → (仅 Go 标准库)
  pkg/utils  → (仅 Go 标准库 + pkg/types)
```

---

## 附录 A：TypeScript → Go 类型映射速查

| TypeScript 类型 | Go 类型 | 备注 |
|----------------|---------|------|
| `SessionId` (branded string) | `type SessionId string` | Go 自定义类型防混用 |
| `AgentId` (branded string) | `type AgentId string` | 同上 |
| `Store<T>` | `Store[T]` (泛型结构体) | sync.RWMutex 保护 |
| `DeepImmutable<AppState>` | `AppState` (值语义) | 通过 Store.mu 保护不变性 |
| `PermissionMode` (union string) | `type PermissionMode string` + const | |
| `PermissionDecision` (discriminated union) | `PermissionDecision` struct + `Behavior` field | |
| `Entry` (20+ variant union) | `EntryEnvelope` + `json.RawMessage` | 延迟解码 |
| `Map<K,V>` | `map[K]V` | |
| `undefined` / optional field | Go 指针类型 (`*string`) | `omitempty` |
| `Promise<T>` | 返回值 + `error`，或 `chan T` | |
| React hook (`useSyncExternalStore`) | `Store.Subscribe` 回调 | 无 React，直接订阅 |
| JSX 命令 (`LocalJSXCommand`) | `LocalCommand` with TUI callback | TUI 框架替代 |

---

## 附录 B：关键文件对应表

| Go 文件 | 对应 TypeScript 源文件 |
|---------|----------------------|
| `pkg/types/ids.go` | `src/types/ids.ts` |
| `pkg/types/permissions.go` | `src/types/permissions.ts` |
| `pkg/types/message.go` | `src/types/message.ts`（推断） |
| `pkg/types/command.go` | `src/types/command.ts` |
| `pkg/types/logs.go` | `src/types/logs.ts` |
| `pkg/types/hooks.go` | `src/types/hooks.ts` |
| `pkg/types/plugin.go` | `src/types/plugin.ts` |
| `internal/state/store.go` | `src/state/store.ts` |
| `internal/state/app_state.go` | `src/state/AppStateStore.ts` |
| `internal/config/settings.go` | `src/utils/settings/types.ts` |
| `internal/config/loader.go` | `src/utils/settings/settings.ts` |
| `internal/session/store.go` | `src/types/logs.ts` + JSONL 写入逻辑 |
