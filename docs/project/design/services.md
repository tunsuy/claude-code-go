# 服务层详细设计

> 负责 Agent：Agent-Services
> 状态：设计中
> 日期：2026-04-02

---

## 概述

服务层（`internal/`）是系统与外部世界的唯一出口，向核心层（`internal/core/`）提供三个稳定的 Go 接口：

| 包 | 职责 |
|---|---|
| `internal/api` | Anthropic API 客户端：SSE 流式、多 Provider、重试、用量统计 |
| `internal/mcp` | MCP 协议客户端：多 transport、连接池、工具适配 |
| `internal/oauth` | OAuth2 授权码流程：Token 存储、keychain 抽象、自动刷新 |

**依赖方向**：服务层 → `pkg/types/`（基础设施），禁止反向依赖核心层。

---

## 1. `internal/api` — Anthropic API 客户端

### 1.1 核心接口定义

```go
// internal/api/client.go

package api

import (
    "context"
    "io"
    "net/http"
)

// Client 是 Anthropic API 的统一入口，支持 Direct / Bedrock / Vertex / Foundry 四种 Provider。
type Client interface {
    // Stream 发起一次流式消息请求，返回 SSE 事件流。
    // 调用方负责关闭 StreamReader。
    Stream(ctx context.Context, req *MessageRequest) (StreamReader, error)

    // Complete 发起非流式（同步）请求，完整返回消息体。
    Complete(ctx context.Context, req *MessageRequest) (*MessageResponse, error)
}

// StreamReader 封装 SSE 事件流读取，符合 io.Closer 语义。
type StreamReader interface {
    // Next 返回下一个 SSE 事件；当流结束时返回 (nil, io.EOF)。
    Next() (*StreamEvent, error)
    // Close 关闭底层连接，释放资源；支持幂等调用。
    io.Closer
}

// MessageRequest 对应 Anthropic Messages API 请求体。
type MessageRequest struct {
    Model       string          `json:"model"`
    MaxTokens   int             `json:"max_tokens"`
    Messages    []MessageParam  `json:"messages"`
    System      string          `json:"system,omitempty"`
    Tools       []ToolSchema    `json:"tools,omitempty"`
    Stream      bool            `json:"stream"`
    // ThinkingBudget > 0 时启用 extended thinking
    ThinkingBudget int          `json:"thinking_budget_tokens,omitempty"`
    // 请求来源标记，用于重试策略（前台/后台区分）
    QuerySource string          `json:"-"`
}

// MessageResponse 对应非流式响应体。
type MessageResponse struct {
    ID         string        `json:"id"`
    Type       string        `json:"type"`
    Role       string        `json:"role"`
    Content    []ContentBlock `json:"content"`
    Model      string        `json:"model"`
    StopReason string        `json:"stop_reason"`
    Usage      Usage         `json:"usage"`
}
```

### 1.2 Provider 工厂

TS 源码中 `getAnthropicClient()` 通过环境变量判断 Provider。Go 侧采用工厂函数 + 配置结构体：

```go
// internal/api/factory.go

// Provider 枚举支持的 API 提供商
type Provider string

const (
    ProviderDirect  Provider = "direct"   // ANTHROPIC_API_KEY
    ProviderBedrock Provider = "bedrock"  // CLAUDE_CODE_USE_BEDROCK=1
    ProviderVertex  Provider = "vertex"   // CLAUDE_CODE_USE_VERTEX=1
    ProviderFoundry Provider = "foundry"  // CLAUDE_CODE_USE_FOUNDRY=1
)

// ClientConfig 汇总客户端配置。
type ClientConfig struct {
    Provider       Provider
    APIKey         string        // Direct 模式
    BaseURL        string        // 自定义 baseURL / Foundry endpoint
    MaxRetries     int           // 默认 10，见 withRetry
    TimeoutSeconds int           // 默认 600 (API_TIMEOUT_MS / 1000)
    // HTTP 请求级自定义 Header（ANTHROPIC_CUSTOM_HEADERS 环境变量解析结果）
    CustomHeaders  map[string]string
    // Bedrock 专用
    AWSRegion      string
    // Vertex 专用
    GCPProject     string
    GCPRegion      string
}

// NewClient 根据配置创建对应的 API 客户端。
// 内部实现：Direct → directClient，Bedrock → bedrockClient，以此类推。
func NewClient(cfg ClientConfig, httpClient *http.Client) (Client, error)
```

**默认请求头**（对应 TS `defaultHeaders`）：

```go
var defaultRequestHeaders = map[string]string{
    "x-app":         "cli",
    "User-Agent":    buildUserAgent(), // "ClaudeCode/x.y.z Go/1.22"
    // X-Claude-Code-Session-Id 在每次请求时注入
}
```

### 1.3 SSE 流式响应处理

TS 侧通过 `@anthropic-ai/sdk` 的 `stream()` 返回 `AsyncIterable`；Go 侧手工解析 SSE。

**事件类型映射**（来源：`src/types/message.ts`、MCP spec）：

```go
// internal/api/stream.go

// StreamEventType 对应 Anthropic SSE event type 字段
type StreamEventType string

const (
    EventMessageStart      StreamEventType = "message_start"
    EventContentBlockStart StreamEventType = "content_block_start"
    EventContentBlockDelta StreamEventType = "content_block_delta"
    EventContentBlockStop  StreamEventType = "content_block_stop"
    EventMessageDelta      StreamEventType = "message_delta"
    EventMessageStop       StreamEventType = "message_stop"
    EventPing              StreamEventType = "ping"
    EventError             StreamEventType = "error"
)

// StreamEvent 是解析后的单个 SSE 事件（使用 json.RawMessage 延迟解析 data）
type StreamEvent struct {
    Type StreamEventType    `json:"type"`
    Data json.RawMessage    `json:"data,omitempty"`
    // 解析后的强类型字段（互斥）
    MessageStart      *MessageStartData      `json:"-"`
    ContentBlockDelta *ContentBlockDeltaData `json:"-"`
    MessageDelta      *MessageDeltaData      `json:"-"`
    Error             *APIErrorData          `json:"-"`
}

// sseReader 实现 StreamReader 接口
type sseReader struct {
    resp    *http.Response
    scanner *bufio.Scanner
    done    bool
}

func (r *sseReader) Next() (*StreamEvent, error) {
    // 解析 "data: {...}\n\n" 格式；遇到 "[DONE]" 返回 io.EOF
    // 参考：https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events
}

func (r *sseReader) Close() error {
    return r.resp.Body.Close()
}
```

**流式聚合器**：核心层调用 `StreamReader.Next()` 时通常需要累积完整的 assistant message：

```go
// internal/api/accumulate.go

// Accumulator 将 SSE 事件流聚合为完整的 MessageResponse（含 Usage）
type Accumulator struct {
    mu      sync.Mutex
    message MessageResponse
}

func (a *Accumulator) Process(ev *StreamEvent) error
func (a *Accumulator) Result() *MessageResponse
```

### 1.4 重试策略（指数退避）

完整移植自 `src/services/api/withRetry.ts`。

```go
// internal/api/retry.go

const (
    DefaultMaxRetries    = 10
    BaseDelayMS          = 500           // 指数退避基础延迟
    MaxDelayMS           = 32_000        // 标准最大延迟
    Max529Retries        = 3             // 连续 529 触发 fallback 的次数阈值
    PersistentMaxBackoff = 5 * 60 * 1000 // UNATTENDED_RETRY 模式最大退避 5 min
    PersistentResetCap   = 6 * 60 * 60 * 1000 // 持久重试总上限 6h
    HeartbeatInterval    = 30 * time.Second
)

// RetryableFunc 是可被重试的 API 操作
type RetryableFunc[T any] func(ctx context.Context, attempt int) (T, error)

// RetryOptions 控制重试行为
type RetryOptions struct {
    MaxRetries              int
    Model                   string
    FallbackModel           string
    QuerySource             string    // 区分前台/后台请求
    InitialConsecutive529   int
    Signal                  context.Context // 替代 AbortSignal
}

// WithRetry 执行带重试的 API 调用（泛型实现）。
// 重试规则（与 TS 保持一致）：
//   - 401 / OAuth revoked → 触发 token 刷新，重建 client
//   - 429 / 529 前台请求 → 指数退避，遵守 retry-after header
//   - 529 后台请求 → 直接放弃（避免容量级联）
//   - 连续 3 次 529 + Opus 模型 → 抛出 FallbackTriggeredError
//   - 400 context overflow → 调整 max_tokens 后重试
//   - 5xx / 408 / 409 / ECONNRESET → 重试
//   - x-should-retry: false → 尊重（订阅用户）
func WithRetry[T any](
    ctx context.Context,
    fn RetryableFunc[T],
    opts RetryOptions,
) (T, error)

// RetryDelay 计算本次重试延迟（含 jitter），对应 TS getRetryDelay()
//   delay = min(baseDelay * 2^(attempt-1), maxDelay) * (1 + rand*0.25)
func RetryDelay(attempt int, retryAfterHeader string, maxDelayMS int) time.Duration

// 特殊错误类型
type CannotRetryError struct {
    Cause        error
    RetryContext RetryContext
}

type FallbackTriggeredError struct {
    OriginalModel string
    FallbackModel string
}
```

**前台 QuerySource 白名单**（对应 TS `FOREGROUND_529_RETRY_SOURCES`）：

```go
var foreground529RetrySources = map[string]bool{
    "repl_main_thread": true,
    "sdk":              true,
    "agent:default":    true,
    "agent:custom":     true,
    "compact":          true,
    // ...
}
```

### 1.5 用量统计类型

来源：`src/services/api/emptyUsage.ts`、`@anthropic-ai/sdk` Beta types。

```go
// internal/api/usage.go

// Usage 对应 Anthropic Beta Messages API 用量字段。
type Usage struct {
    InputTokens              int    `json:"input_tokens"`
    CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
    CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
    OutputTokens             int    `json:"output_tokens"`
    ServerToolUse            ServerToolUse `json:"server_tool_use"`
    ServiceTier              string `json:"service_tier"`   // "standard" | "priority" | "batch"
    CacheCreation            CacheCreationUsage `json:"cache_creation"`
    InferenceGeo             string `json:"inference_geo"`
    Speed                    string `json:"speed"`          // "standard" | "turbo"
}

type ServerToolUse struct {
    WebSearchRequests int `json:"web_search_requests"`
    WebFetchRequests  int `json:"web_fetch_requests"`
}

type CacheCreationUsage struct {
    Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
    Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
}

// EmptyUsage 对应 TS 的 EMPTY_USAGE 零值常量。
var EmptyUsage = Usage{ServiceTier: "standard", Speed: "standard"}

// Utilization 对应 /api/oauth/usage 响应（来源：usage.ts）。
type Utilization struct {
    FiveHour         *RateLimit    `json:"five_hour,omitempty"`
    SevenDay         *RateLimit    `json:"seven_day,omitempty"`
    SevenDayOAuthApps *RateLimit   `json:"seven_day_oauth_apps,omitempty"`
    SevenDayOpus     *RateLimit    `json:"seven_day_opus,omitempty"`
    SevenDaySonnet   *RateLimit    `json:"seven_day_sonnet,omitempty"`
    ExtraUsage       *ExtraUsage   `json:"extra_usage,omitempty"`
}

type RateLimit struct {
    Utilization *float64 `json:"utilization"` // 0-100
    ResetsAt    *string  `json:"resets_at"`   // ISO 8601
}

type ExtraUsage struct {
    IsEnabled     bool     `json:"is_enabled"`
    MonthlyLimit  *int64   `json:"monthly_limit"`
    UsedCredits   *int64   `json:"used_credits"`
    Utilization   *float64 `json:"utilization"`
}
```

### 1.6 错误类型

```go
// internal/api/errors.go

// APIErrorKind 对应 TS 中 classifyAPIError() 的分类枚举
type APIErrorKind string

const (
    ErrKindRateLimit         APIErrorKind = "rate_limit"      // 429
    ErrKindOverloaded        APIErrorKind = "overloaded"       // 529
    ErrKindUnauthorized      APIErrorKind = "unauthorized"     // 401
    ErrKindForbidden         APIErrorKind = "forbidden"        // 403
    ErrKindContextWindow     APIErrorKind = "context_window"   // 400 prompt too long
    ErrKindInvalidRequest    APIErrorKind = "invalid_request"  // 400 其他
    ErrKindServerError       APIErrorKind = "server_error"     // 5xx
    ErrKindConnectionTimeout APIErrorKind = "connection_timeout"
    ErrKindConnectionError   APIErrorKind = "connection_error"
    ErrKindUnknown           APIErrorKind = "unknown"
)

// APIError 封装 HTTP 层错误，携带状态码和响应头。
type APIError struct {
    StatusCode int
    Message    string
    Headers    http.Header
    Kind       APIErrorKind
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// Is529Error 判断是否为 529 overloaded 错误（兼容 SDK 流式序列化问题）
func Is529Error(err error) bool

// IsOAuthTokenRevokedError 判断是否为 OAuth token 被撤销（403 + 特定 message）
func IsOAuthTokenRevokedError(err error) bool

// ParseContextOverflowError 解析 "input length and max_tokens exceed context limit" 错误
// 返回 inputTokens, contextLimit；不匹配时返回 (0, 0, false)
func ParseContextOverflowError(err error) (inputTokens, contextLimit int, ok bool)
```

---

## 2. `internal/mcp` — MCP 客户端

### 2.1 MCPClient 接口

```go
// internal/mcp/client.go

package mcp

// MCPClient 表示到单个 MCP Server 的连接。
// 对应 TS 的 @modelcontextprotocol/sdk Client 实例。
type MCPClient interface {
    // ListTools 列举服务器暴露的工具列表。
    ListTools(ctx context.Context) ([]MCPToolDef, error)

    // CallTool 调用指定工具，返回结构化结果。
    CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error)

    // ListResources 列举服务器暴露的资源。
    ListResources(ctx context.Context) ([]MCPResource, error)

    // ReadResource 读取单个资源内容。
    ReadResource(ctx context.Context, uri string) (*MCPResourceContent, error)

    // Ping 检查连接活性。
    Ping(ctx context.Context) error

    // Close 关闭连接并释放资源（transport 级别）。
    Close() error

    // ServerInfo 返回握手阶段获取的服务器信息。
    ServerInfo() ServerInfo
}

// ServerInfo 对应 MCP initialize 响应中的 serverInfo 字段。
type ServerInfo struct {
    Name         string
    Version      string
    Capabilities ServerCapabilities
    Instructions string // 可选的系统级提示
}

// ServerCapabilities 标记服务器支持的能力集合。
type ServerCapabilities struct {
    Tools     bool
    Resources bool
    Prompts   bool
}

// MCPToolDef 对应 MCP ListTools 响应中的单个工具定义。
type MCPToolDef struct {
    Name        string          // 原始名称（可含 "/" 等特殊字符）
    Description string
    InputSchema json.RawMessage // JSON Schema object
}

// MCPToolResult 对应 MCP CallTool 响应。
type MCPToolResult struct {
    Content []MCPContent
    IsError bool
    Meta    map[string]any
}

// MCPContent 对应 MCP content block（text / image / resource）
type MCPContent struct {
    Type     string // "text" | "image" | "resource"
    Text     string
    Data     string // base64 encoded for image
    MIMEType string
}

// MCPResource 对应 MCP resource 列表项。
type MCPResource struct {
    URI         string
    Name        string
    Description string
    MIMEType    string
}
```

### 2.2 Transport 抽象

来源：`src/services/mcp/types.ts` Transport 枚举（`stdio | sse | http | ws | sdk`）。

```go
// internal/mcp/transport.go

// TransportType 枚举支持的 MCP transport 类型。
type TransportType string

const (
    TransportStdio TransportType = "stdio"
    TransportSSE   TransportType = "sse"
    TransportHTTP  TransportType = "http"
    TransportWS    TransportType = "ws"
)

// Transport 是底层双向通信通道的抽象（对应 MCP SDK Transport interface）。
// Go 侧基于 io.ReadWriteCloser 与 json.Encoder/Decoder 组合实现。
type Transport interface {
    // Send 发送 JSON-RPC 消息。
    Send(ctx context.Context, msg *JSONRPCMessage) error
    // Recv 接收下一条 JSON-RPC 消息（阻塞）。
    Recv(ctx context.Context) (*JSONRPCMessage, error)
    // Close 关闭 transport。
    Close() error
}

// StdioTransportConfig 对应 McpStdioServerConfig
type StdioTransportConfig struct {
    Command string
    Args    []string
    Env     map[string]string
}

// SSETransportConfig 对应 McpSSEServerConfig（也兼容 StreamableHTTP）
type SSETransportConfig struct {
    URL     string
    Headers map[string]string
    OAuth   *MCPOAuthConfig
}

// HTTPTransportConfig 对应 McpHTTPServerConfig（Streamable HTTP）
type HTTPTransportConfig struct {
    URL     string
    Headers map[string]string
    OAuth   *MCPOAuthConfig
}

// MCPOAuthConfig MCP 服务器的 OAuth2 认证配置
type MCPOAuthConfig struct {
    ClientID              string
    CallbackPort          int
    AuthServerMetadataURL string
}

// NewTransport 工厂：根据 TransportType 和配置创建对应实现。
func NewTransport(t TransportType, cfg any) (Transport, error)
```

**stdio transport 实现要点**：
- 启动子进程（`os/exec`），stdin/stdout 作为双向管道
- 每行一个完整 JSON-RPC 消息（newline-delimited JSON）
- 通过 `RegisterCleanup` 在进程退出时终止子进程

**SSE transport 实现要点**（对应 TS `SSEClientTransport`）：
- POST `/message` 发送请求，GET `/sse` 接收服务器推送
- 维护 `session-id`（Streamable HTTP 协议使用 `Mcp-Session-Id` header）

### 2.3 连接池管理

对应 TS `MCPConnectionManager` 和 `useManageMCPConnections`。

```go
// internal/mcp/pool.go

// ConnectionStatus 枚举 MCP Server 连接状态（对应 TS MCPServerConnection type）
type ConnectionStatus string

const (
    StatusConnected  ConnectionStatus = "connected"
    StatusFailed     ConnectionStatus = "failed"
    StatusNeedsAuth  ConnectionStatus = "needs-auth"
    StatusPending    ConnectionStatus = "pending"
    StatusDisabled   ConnectionStatus = "disabled"
)

// ServerConnection 表示到单个 MCP Server 的完整连接记录。
type ServerConnection struct {
    Name       string
    Status     ConnectionStatus
    Config     ServerConfig
    Client     MCPClient        // nil 当 Status != connected
    Error      string           // Status == failed 时的错误信息
    ReconnectAttempt int
}

// Pool 管理多个 MCP Server 的连接生命周期。
type Pool struct {
    mu          sync.RWMutex
    connections map[string]*ServerConnection
    // ...
}

// NewPool 创建连接池。
func NewPool() *Pool

// Connect 建立到指定 Server 的连接（幂等）。
func (p *Pool) Connect(ctx context.Context, name string, cfg ServerConfig) error

// Reconnect 重新建立连接（强制清除旧连接）。
func (p *Pool) Reconnect(ctx context.Context, name string) error

// Disconnect 断开连接并标记为 disabled。
func (p *Pool) Disconnect(name string) error

// GetConnected 返回所有 connected 状态的连接。
func (p *Pool) GetConnected() []*ServerConnection

// GetAll 返回所有连接（含 failed/pending 等）。
func (p *Pool) GetAll() []*ServerConnection

// ServerConfig 汇总 MCP Server 的配置（对应 TS ScopedMcpServerConfig）。
type ServerConfig struct {
    Transport TransportType
    Stdio     *StdioTransportConfig
    SSE       *SSETransportConfig
    HTTP      *HTTPTransportConfig
    Scope     string // "local" | "user" | "project" | "enterprise"
}
```

**连接错误处理**：
- `McpAuthError` → 标记 `StatusNeedsAuth`，触发 OAuth 流程
- `McpSessionExpiredError`（HTTP 404 + JSON-RPC -32001）→ 自动重连
- 通用错误 → 标记 `StatusFailed`，记录错误信息

**工具描述截断**（对应 TS `MAX_MCP_DESCRIPTION_LENGTH = 2048`）：

```go
const MaxMCPDescriptionLength = 2048

func truncateDescription(s string) string {
    if len([]rune(s)) <= MaxMCPDescriptionLength {
        return s
    }
    return string([]rune(s)[:MaxMCPDescriptionLength])
}
```

### 2.4 工具适配为 Tool 接口

// TODO(dep): 等待 Agent-Core #6 定义 Tool 接口后补全适配代码

```go
// internal/mcp/adapter.go

// AdaptToTool 将 MCPToolDef 适配为核心层 Tool 接口。
// 工具名称规范化：将 "/" "." 等特殊字符替换为 "_"，确保符合 API 命名规范。
// 命名格式："{serverName}__{normalizedToolName}"（双下划线分隔，对应 TS buildMcpToolName）
//
// TODO(dep): 依赖 pkg/types.Tool 接口，待 Agent-Core #6 完成后实现。
func AdaptToTool(serverName string, def MCPToolDef, client MCPClient) /* pkg/types.Tool */ interface{}

// NormalizeToolName 将 MCP 工具名规范化为 Anthropic API 合法名称。
// 规则：仅保留 [a-zA-Z0-9_-]，其他字符替换为 "_"。
func NormalizeToolName(name string) string
```

---

## 3. `internal/oauth` — OAuth2 认证

### 3.1 OAuth2 流程设计

对应 TS `src/services/oauth/`，实现标准 **PKCE 授权码流程**（RFC 7636）。

```
用户                   Claude Code CLI                    Anthropic OAuth Server
 |                          |                                        |
 |    /login 命令           |                                        |
 |------------------------->|                                        |
 |                          | 1. generateCodeVerifier()              |
 |                          | 2. generateCodeChallenge(verifier)     |
 |                          | 3. generateState()                     |
 |                          | 4. 启动 AuthCodeListener (随机端口)      |
 |                          | 5. 构建 authURL (PKCE + scope)         |
 | 打开浏览器（authURL）     |                                        |
 |<-------------------------|                                        |
 |    在浏览器完成登录       |                                        |
 |-------------------------------------------------------------->|
 |                          |  重定向 localhost:{port}/callback      |
 |                          |<--------------------------------------|
 |                          | 6. AuthCodeListener 捕获 code + state  |
 |                          | 7. 校验 state (CSRF 防护)              |
 |                          | 8. exchangeCodeForTokens(code, verifier)|
 |                          |--------------------------------------->|
 |                          |  access_token + refresh_token          |
 |                          |<---------------------------------------|
 |                          | 9. 存储 Token (keychain)               |
 |                          | 10. 重定向浏览器到成功页                |
 | 登录成功                  |                                        |
 |<-------------------------|                                        |
```

**Scope 说明**（来源：`src/constants/oauth.ts`）：

| scope | 用途 |
|---|---|
| `org:inference` | Claude.ai 推理（订阅用户） |
| `user:profile` | 读取用户资料 |
| `org:billing` | 计费信息 |
| `org:teams` | 团队管理 |

### 3.2 核心类型与接口

```go
// internal/oauth/types.go

package oauth

// OAuthTokens 存储完整的 OAuth Token 集合。
type OAuthTokens struct {
    AccessToken    string   `json:"access_token"`
    RefreshToken   string   `json:"refresh_token"`
    ExpiresAt      int64    `json:"expires_at"`   // Unix ms
    Scopes         []string `json:"scopes"`
    SubscriptionType string `json:"subscription_type,omitempty"` // "max"|"pro"|"enterprise"|"team"
    RateLimitTier  string   `json:"rate_limit_tier,omitempty"`
    // 关联账户信息
    TokenAccount   *TokenAccount `json:"token_account,omitempty"`
}

// TokenAccount 对应 token exchange 响应中的 account 字段
type TokenAccount struct {
    UUID              string `json:"uuid"`
    EmailAddress      string `json:"email_address"`
    OrganizationUUID  string `json:"organization_uuid,omitempty"`
}

// OAuthTokenExchangeResponse 是 /oauth/token 端点的响应体
type OAuthTokenExchangeResponse struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token,omitempty"`
    ExpiresIn    int    `json:"expires_in"`   // 秒
    Scope        string `json:"scope"`
    TokenType    string `json:"token_type"`
    Account      *struct {
        UUID         string `json:"uuid"`
        EmailAddress string `json:"email_address"`
    } `json:"account,omitempty"`
    Organization *struct {
        UUID string `json:"uuid"`
    } `json:"organization,omitempty"`
}

// ProfileInfo 对应 fetchProfileInfo() 结果
type ProfileInfo struct {
    SubscriptionType string
    RateLimitTier    string
    DisplayName      string
    HasExtraUsage    bool
    BillingType      string
    AccountCreatedAt string
    OrgCreatedAt     string
}
```

```go
// internal/oauth/client.go

// Client 提供完整的 OAuth2 操作。
type Client struct {
    cfg        OAuthConfig
    httpClient *http.Client
    store      TokenStore
}

// OAuthConfig 汇总 OAuth 端点配置（对应 TS getOauthConfig()）
type OAuthConfig struct {
    ClientID           string
    AuthorizeURL       string // console.anthropic.com/oauth/authorize
    ClaudeAIAuthorizeURL string // claude.ai/oauth/authorize
    TokenURL           string
    ManualRedirectURL  string
    BaseAPIURL         string
    ConsoleSuccessURL  string
    ClaudeAISuccessURL string
    RolesURL           string
    APIKeyURL          string
}

// BuildAuthURL 构建 PKCE 授权 URL（对应 TS buildAuthUrl()）。
func (c *Client) BuildAuthURL(params AuthURLParams) string

// ExchangeCodeForTokens 用授权码换取 access token（对应 TS exchangeCodeForTokens()）。
// timeout: 15s
func (c *Client) ExchangeCodeForTokens(
    ctx context.Context,
    code, state, verifier string,
    port int,
) (*OAuthTokens, error)

// RefreshToken 使用 refresh_token 获取新的 access token（对应 TS refreshOAuthToken()）。
// 内置优化：若 ProfileInfo 已缓存则跳过 /api/oauth/profile 请求（节省约 7M req/day）。
func (c *Client) RefreshToken(ctx context.Context, refreshToken string, scopes []string) (*OAuthTokens, error)

// IsTokenExpired 判断 token 是否过期（含 5 分钟缓冲，对应 TS isOAuthTokenExpired()）。
func IsTokenExpired(expiresAt int64) bool {
    bufferMS := int64(5 * 60 * 1000)
    return time.Now().UnixMilli()+bufferMS >= expiresAt
}

// AuthURLParams 构建授权 URL 的参数集合。
type AuthURLParams struct {
    CodeChallenge    string
    State            string
    Port             int
    IsManual         bool
    LoginWithClaude  bool   // true → 使用 claude.ai OAuth 端点
    InferenceOnly    bool   // true → 仅申请 org:inference scope（长期 token）
    OrgUUID          string
    LoginHint        string
    LoginMethod      string // "sso" | "magic_link" | "google"
}
```

### 3.3 Token 存储接口（keychain 抽象）

```go
// internal/oauth/store.go

// TokenStore 是 Token 持久化的接口抽象，解耦具体存储实现。
// 设计原则：接口简单，支持 macOS Keychain、文件、内存三种实现。
type TokenStore interface {
    // Load 加载 Token；若不存在返回 (nil, nil)。
    Load() (*OAuthTokens, error)
    // Save 持久化 Token。
    Save(tokens *OAuthTokens) error
    // Delete 删除 Token（登出）。
    Delete() error
}

// KeychainStore 使用 macOS Keychain 存储（通过 CGo 或 security 命令行工具）。
// Service: "claude-code", Account: "oauth-tokens"
type KeychainStore struct {
    serviceName string
    accountName string
}

// FileStore 使用加密文件存储（非 macOS 平台的 fallback）。
// 文件路径：$XDG_CONFIG_HOME/claude-code/tokens.enc
type FileStore struct {
    path string
}

// MemoryStore 纯内存存储，用于测试。
type MemoryStore struct {
    mu     sync.Mutex
    tokens *OAuthTokens
}

// NewTokenStore 根据平台返回合适的 TokenStore 实现：
//   - darwin → KeychainStore（优先）
//   - 其他   → FileStore
func NewTokenStore() TokenStore
```

### 3.4 PKCE 加密工具

```go
// internal/oauth/crypto.go
// 对应 TS src/services/oauth/crypto.ts

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
)

// GenerateCodeVerifier 生成 32 字节随机 base64url 编码的 code_verifier。
func GenerateCodeVerifier() (string, error)

// GenerateCodeChallenge 对 verifier 进行 SHA-256 后 base64url 编码（S256 方法）。
func GenerateCodeChallenge(verifier string) string

// GenerateState 生成 32 字节随机 base64url 编码的 state 参数（CSRF 防护）。
func GenerateState() (string, error)
```

### 3.5 AuthCodeListener（授权码监听器）

```go
// internal/oauth/listener.go
// 对应 TS src/services/oauth/auth-code-listener.ts

// AuthCodeListener 在本地启动临时 HTTP Server 捕获 OAuth 回调。
type AuthCodeListener struct {
    server        *http.Server
    port          int
    expectedState string
    codeCh        chan string
    errCh         chan error
    callbackPath  string  // 默认 "/callback"
}

// NewAuthCodeListener 创建监听器。
func NewAuthCodeListener(callbackPath string) *AuthCodeListener

// Start 在指定端口（0 表示 OS 随机分配）启动监听，返回实际端口。
func (l *AuthCodeListener) Start(port int) (int, error)

// WaitForCode 阻塞等待授权码；ctx 超时/取消时返回 error。
func (l *AuthCodeListener) WaitForCode(ctx context.Context, state string) (string, error)

// RedirectSuccess 将浏览器重定向到成功页面（根据 scopes 选择 URL）。
func (l *AuthCodeListener) RedirectSuccess(scopes []string) error

// Close 关闭监听器。
func (l *AuthCodeListener) Close() error
```

### 3.6 Token 刷新逻辑

对应 TS `checkAndRefreshOAuthTokenIfNeeded()`，提供全局单例刷新锁（防止并发重复刷新）：

```go
// internal/oauth/refresh.go

// TokenManager 封装 Token 的生命周期管理（加载、检查、刷新）。
type TokenManager struct {
    store      TokenStore
    client     *Client
    mu         sync.Mutex    // 保护刷新操作
    refreshing bool          // 是否正在刷新（防止并发）
}

// CheckAndRefreshIfNeeded 检查 Token 有效性并在必要时自动刷新。
// 线程安全；并发调用时只有第一个 goroutine 执行刷新，其余等待。
func (m *TokenManager) CheckAndRefreshIfNeeded(ctx context.Context) (*OAuthTokens, error)

// HandleOAuth401Error 处理 API 返回 401 时的 Token 刷新（对应 TS handleOAuth401Error()）。
// 仅当 failedAccessToken 与当前存储的 accessToken 一致时才触发刷新（防止已刷新的 token 被再次刷新）。
func (m *TokenManager) HandleOAuth401Error(ctx context.Context, failedAccessToken string) error
```

---

## 4. 外部依赖

```
# go.mod 依赖（服务层所需）

## HTTP / SSE
net/http (stdlib)                    # 标准 HTTP 客户端

## MCP SDK
github.com/modelcontextprotocol/go-sdk  # 官方 Go MCP SDK（如已发布）
# 备用：基于 JSON-RPC 2.0 手工实现（若 Go SDK 尚未成熟）
# github.com/sourcegraph/jsonrpc2

## JSON 处理
encoding/json (stdlib)

## macOS Keychain（oauth）
github.com/keybase/go-keychain      # macOS Keychain CGo 绑定
# 或
# github.com/zalando/go-keyring     # 跨平台 keyring 抽象（更轻量）

## 进程管理（stdio transport）
os/exec (stdlib)

## 并发工具
sync (stdlib)
golang.org/x/sync                   # singleflight 用于 Token 刷新去重

## 测试
github.com/stretchr/testify         # assert / mock
github.com/jarcoal/httpmock         # HTTP mock
```

**MCP SDK 选型说明**：截至 2026-04，MCP 官方 Go SDK（`github.com/modelcontextprotocol/go-sdk`）仍处于 early preview，核心 JSON-RPC 协议层稳定。若稳定性不足，以 `github.com/sourcegraph/jsonrpc2` 作为底层，自行实现 MCP client 握手协议。

---

## 5. 设计决策

### 5.1 接口优先，实现隔离

每个子包对外只暴露接口（`Client`/`MCPClient`/`TokenStore`），具体实现位于同包内部，便于 Mock 测试：

```go
// 测试时注入 mock
var _ api.Client = (*mockAPIClient)(nil)
var _ mcp.MCPClient = (*mockMCPClient)(nil)
var _ oauth.TokenStore = (*oauth.MemoryStore)(nil)
```

### 5.2 context.Context 替代 AbortSignal

TS 使用 `AbortSignal` 做请求取消；Go 使用 `context.Context`（语言原生，链式传播）：

```go
// TS: signal?.aborted → throw APIUserAbortError()
// Go: ctx.Err() != nil → return ctx.Err()
select {
case <-ctx.Done():
    return nil, ctx.Err()
default:
}
```

### 5.3 泛型 WithRetry

利用 Go 1.21 泛型消除 `interface{}` 类型断言，RetryableFunc 类型安全：

```go
// WithRetry[*MessageResponse](ctx, fn, opts) — 类型推断，无需显式指定
```

### 5.4 SSE 解析：标准库 + bufio.Scanner

不引入第三方 SSE 库，使用 `bufio.Scanner` 逐行解析，轻量且易于测试。

### 5.5 Token 刷新：singleflight

使用 `golang.org/x/sync/singleflight` 确保并发请求下 Token 刷新只执行一次（对应 TS 的 `refreshPromise` 机制）：

```go
var refreshGroup singleflight.Group

func (m *TokenManager) CheckAndRefreshIfNeeded(ctx context.Context) (*OAuthTokens, error) {
    v, err, _ := refreshGroup.Do("refresh", func() (interface{}, error) {
        return m.doRefresh(ctx)
    })
    // ...
}
```

### 5.6 MCP 工具名规范化

TS 使用 `normalization.ts` + `buildMcpToolName()` 生成 `"serverName__toolName"` 格式。Go 侧保持相同约定，确保 API 侧工具名稳定（与 TS 版本互操作时无需重新训练工具调用）。

### 5.7 错误分层

```
transport error（网络层）
    ↓ 包装为
api.APIError（HTTP 层，含 StatusCode + Headers）
    ↓ 包装为
CannotRetryError（重试耗尽）/ FallbackTriggeredError（模型 fallback）
    ↓ 向上传播给核心层
```

---

## 6. TS → Go 行为映射

| TS 源码 | Go 实现 | 说明 |
|---|---|---|
| `getAnthropicClient()` | `api.NewClient(cfg)` | 工厂函数，env 变量驱动 Provider 选择 |
| `withRetry<T>()` (AsyncGenerator) | `api.WithRetry[T]()` (泛型) | Generator yield → 错误回调；同步等待替代 async/await |
| `BASE_DELAY_MS * 2^(attempt-1)` | `RetryDelay(attempt, ...)` | 指数退避，含 25% jitter |
| `is529Error()` | `api.Is529Error(err)` | 兼容 SDK 流式编码问题的字符串检查 |
| `@modelcontextprotocol/sdk Client` | `mcp.MCPClient` interface | Go 接口 + 具体实现；JSON-RPC 2.0 |
| `StdioClientTransport` | `mcp.StdioTransport` | `os/exec` 启动子进程 |
| `SSEClientTransport` | `mcp.SSETransport` | 标准 SSE + POST 混合 |
| `StreamableHTTPClientTransport` | `mcp.HTTPTransport` | `Mcp-Session-Id` header 管理 |
| `MCPConnectionManager` | `mcp.Pool` | 连接池 + 状态机（connected/failed/needs-auth） |
| `buildMcpToolName()` | `mcp.AdaptToTool()` | `serverName__toolName` 命名约定 |
| `AuthCodeListener` | `oauth.AuthCodeListener` | 临时 HTTP Server 捕获授权码 |
| `generateCodeVerifier/Challenge/State` | `oauth.GenerateCode*()` | PKCE S256，`crypto/rand` |
| `exchangeCodeForTokens()` | `oauth.Client.ExchangeCodeForTokens()` | POST /oauth/token, 15s timeout |
| `refreshOAuthToken()` | `oauth.Client.RefreshToken()` | 内置 profile 缓存优化 |
| `isOAuthTokenExpired()` | `oauth.IsTokenExpired()` | 5 分钟缓冲 |
| `checkAndRefreshOAuthTokenIfNeeded()` | `oauth.TokenManager.CheckAndRefreshIfNeeded()` | `singleflight` 防并发刷新 |
| Keychain (`saveApiKey` via `keytar`) | `oauth.KeychainStore` | `go-keychain` CGo / `go-keyring` |
| `EMPTY_USAGE` | `api.EmptyUsage` | 零值常量，ServiceTier="standard" |
| `Utilization` type | `api.Utilization` | /api/oauth/usage 响应 |
| `MAX_MCP_DESCRIPTION_LENGTH = 2048` | `mcp.MaxMCPDescriptionLength = 2048` | 工具描述截断上限 |
| `DEFAULT_MCP_TOOL_TIMEOUT_MS` | `mcp.DefaultToolTimeout = 100_000_000ms` | 约 27.8h，实际由 ctx 超时控制 |

---

*文档版本：v0.1 · 2026-04-02 · Agent-Services*
