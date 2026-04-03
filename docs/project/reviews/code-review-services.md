# Services Layer Code Review

> **Reviewer**: Tech Lead
> **Date**: 2026-04-03
> **Subject**: 任务 #14 实现代码（Agent-Services）— `internal/api`、`internal/oauth`、`internal/mcp`
> **Verdict**: APPROVED_WITH_CHANGES

---

## 1. Overall Assessment

服务层整体实现质量较高，三个子包均已落地可运行代码，并在很大程度上解决了上轮设计评审（`review-services.md`，2026-04-02）提出的所有 P0/P1 问题：

- **`TokenManager`** 已改用自行实现的 `singleflightGroup`（基于 `sync.Mutex` + `sync.WaitGroup`），正确消除并发 token 刷新竞争，与设计文档 §5.5 契合。
- **`AdaptToTool`** 已对接 `internal/tool.Tool` 接口，不再有编译阻塞的 `TODO(dep)`。
- **`FileStore` AES-256-GCM** 加密方案已实现，密钥通过内联 PBKDF2-SHA256（100,000 次迭代）从 hostname 派生，格式文档已写入注释。
- **`AuthCodeListener`** 端口冲突已处理（随机回退最多 3 次）。
- **MCP 单 reader goroutine**（P0-A）已正确修复：`startReader` + `sync.Once` + `pending` channel 分发，不再存在并发 `Recv()` 竞争。
- **`WsTransportConfig`** 已在 `transport.go` 中补充结构体定义，WebSocket 已明确标注为"未实现"。

但本次评审在 macOS Keychain Store 中发现 **1 个新增 P0 安全问题**（OAuth token 明文暴露于进程参数），必须在 merge 前修复。其余剩余问题以 P1 / P2 为主，需在后续迭代中跟进。

---

## 2. Strengths

1. **MCP 并发架构（P0-A 修复正确）**
   `jsonrpc.go` 的单 background goroutine 模式实现严谨：`startOnce sync.Once` 保证只启动一个 reader；`pending map[int64]chan *JSONRPCMessage` 通过 `mu sync.Mutex` 保护；`recvErr chan error` 为 buffered(1) 并在 `call()` 中正确复播（re-broadcast）给其他等待者。整体结构符合 Go 并发惯用法。

2. **`Accumulator` 线程安全**
   `sync.Mutex` 贯穿 `Process` 和 `Result`，并发调用安全。`EventContentBlockStop` 时将临时 `blocks` 图中的块提交到 `message.Content`，`Result()` 中对残留 blocks 的 flush 逻辑兜底正确。

3. **重试逻辑完整**
   `WithRetry[T]` 泛型实现忠实移植了 TS 的 `withRetry.ts`：指数退避 + 25% jitter、`x-should-retry` 头部优先、529 前台/后台差异处理、`FallbackTriggeredError` Opus 降级、context 取消立即退出。`RetryDelay` 独立函数便于单测。

4. **PKCE 加密正确**
   `crypto.go` 的 `GenerateCodeVerifier`/`GenerateCodeChallenge` 符合 RFC 7636（S256/SHA-256/base64url，无 padding）；`GenerateState` 使用 `crypto/rand`（非 `math/rand`）。

5. **AES-256-GCM 实现健康**
   `crypto_aes.go` 的 `encryptTokenFile`/`decryptTokenFile`：nonce 随机生成（12 字节，符合 GCM 标准推荐），salt 随机（32 字节），格式 `[version 4B][salt 32B][nonce 12B][ciphertext+tag]` 已文档化，版本前缀便于算法迁移，`gcm.Open` 的 tag 校验在解密时自动提供完整性验证。

6. **SSE 解析健壮**
   `sseReader.Next()` 使用 `bufio.Scanner`，buffer 上限 1 MiB，正确处理 Anthropic 的 `event: message_stop`（不检查 OpenAI `[DONE]` 哨兵，注释明确说明）。`parseSSEEvent` 的 typed sub-field 解析失败时设为 nil（宽容策略），调用方有完整控制权。

7. **`TokenManager` singleflight 正确**
   `refresh.go` 内联实现了等效于 `golang.org/x/sync/singleflight` 的 `singleflightGroup`，逻辑正确（持锁插入、不持锁执行 fn、执行后删除），完全避免 `refreshing bool` 的条件竞争。`HandleOAuth401Error` 的"只有 token 与失败 token 匹配才刷新"保护正确消除重复刷新。

8. **平台存储抽象干净**
   `NewTokenStore()` 的 build-tag 分发（`darwin` → KeychainStore；other → FileStore）清晰，`store_other.go` 的委托模式避免重复实现。macOS `security` CLI 方案免去 CGO 依赖，可接受。

9. **MCP 工具命名约定完整**
   `adapter.go` 的 `serverName__normalizedToolName` 双下划线约定、`NormalizeToolName` 正则（`[^a-zA-Z0-9_\-]`）与 TS `buildMcpToolName()` 一致；`UserFacingName` 输出 `MCP(server::tool)` 用于 UI 展示，接口实现完整。

10. **错误分层清晰**
    `api.APIError`（含 `Kind` 枚举）→ `CannotRetryError`（Unwrap 链）→ `FallbackTriggeredError`；`ParseContextOverflowError` 提取 token 数/上下文限制；三个辅助函数（`Is529Error`、`IsOAuthTokenRevokedError`、`ParseContextOverflowError`）都使用 `errors.As` 而非类型断言，Go 惯用法正确。

---

## 3. Issues

### P0 — Must Fix Before Merge

#### P0-1: macOS Keychain — OAuth Token 明文暴露于进程参数

**文件**: `internal/oauth/store_darwin.go` L55–63

```go
cmd := exec.Command("security", "add-generic-password",
    "-s", k.serviceName,
    "-a", k.accountName,
    "-w", string(data),   // ← 完整 token JSON（含 access_token / refresh_token）
)
```

OAuth token JSON（包含 `access_token` 和 `refresh_token`）以命令行参数 `-w <value>` 形式传递给 `security` 进程。在 macOS 上，进程参数对同机所有用户可见（`ps aux`、`/proc/<pid>/cmdline`）。在 `exec.Command` 创建到 `security` 进程退出这段窗口期内，任意本地用户可读取完整 OAuth token，直接导致 token 泄露。

**修复**：通过 stdin 管道传递密码（使用 `-p -` 参数），彻底消除进程参数暴露：

```go
cmd := exec.Command("security", "add-generic-password",
    "-s", k.serviceName,
    "-a", k.accountName,
    "-p", "-",   // read password from stdin
)
cmd.Stdin = strings.NewReader(string(data))
out, err := cmd.CombinedOutput()
```

---

### P1 — Should Fix Soon

#### P1-1: `stdioTransport.Recv()` 每次调用都 spawn 一个 goroutine — 潜在 goroutine 泄漏

**文件**: `internal/mcp/transport.go` L173–198

```go
func (t *stdioTransport) Recv(ctx context.Context) (*JSONRPCMessage, error) {
    ch := make(chan result, 1)
    go func() {
        line, err := t.stdout.ReadString('\n')   // 阻塞读
        ...
    }()
    select {
    case <-ctx.Done():
        return nil, ctx.Err()   // goroutine 泄漏！
    case r := <-ch:
        ...
    }
}
```

`jsonRPCClient.startReader` 会持续循环调用 `c.transport.Recv(ctx)`，而 `stdioTransport.Recv` 在每次调用时都 spawn 一个 goroutine 去读取 stdout。当 `ctx` 被取消时，`Recv` 提前返回，但阻塞在 `ReadString('\n')` 的 goroutine 继续存活，直到有新的一行输出到达——如果子进程已停止，该 goroutine 将永远泄漏。

**更严重的是**：单 reader goroutine（`startReader`）加上 `stdioTransport.Recv` 的每次调用均 spawn 新 goroutine，意味着并发度为 1 的情况下每次 `Recv` 仍会产生不必要的 goroutine，额外增加调度开销。

**修复方向**：`stdioTransport` 应在构造时启动一个专属的 stdout reader goroutine，通过内部 channel 向上层分发消息，`Recv` 直接从该 channel 读取（与 `sseTransport` 的 `recvCh` 模式一致）。

---

#### P1-2: `KeychainStore.Load()` 对非 exit-44 keychain 错误静默吞掉

**文件**: `internal/oauth/store_darwin.go` L27–34

```go
if err != nil {
    if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
        return nil, nil   // not found — OK
    }
    // Other errors (including "The specified item could not be found")
    return nil, nil   // ← 其他所有错误均被静默忽略
}
```

macOS `security find-generic-password` 退出码 44 表示"item not found"，但其他错误（如权限拒绝、keychain 被锁、进程无法 fork）也会走到 `return nil, nil`，导致应用程序静默跳过 OAuth 认证而非报告错误。注释中的说明"Other errors (including...)"自相矛盾：它把需要区分的情况混在一起。

**修复**：非 44 的 exit error 应返回 `fmt.Errorf("keychain: load: %w", err)`，让调用方决定如何处理。"item not found" 仅对 exit-44 返回 `(nil, nil)`。

---

#### P1-3: `WaitForCode` 的 `expectedState` 存在时序竞争

**文件**: `internal/oauth/listener.go` L134–144

```go
func (l *AuthCodeListener) WaitForCode(ctx context.Context, state string) (string, error) {
    l.expectedState = state   // ← 无锁写入
    select { ... }
}
```

`handleCallback` 在 HTTP server goroutine 中读取 `l.expectedState`（L105），而 `WaitForCode` 在调用方 goroutine 中写入。两者之间没有同步机制。虽然在典型调用顺序下（先 `Start`，再 `WaitForCode`，再等 HTTP 回调）时序通常不会引发问题，但 Go 的 race detector 会将此标记为数据竞争。

**修复**：将 `expectedState` 改为在 `NewAuthCodeListener` 时通过参数传入，或在 `WaitForCode` 中使用 `atomic.StorePointer` / `sync/atomic.Value` 存储，或改用 `sync.Mutex` 保护读写。

---

#### P1-4: `singleflightGroup` 重复造轮子但缺少关键特性

**文件**: `internal/oauth/refresh.go` L9–46

内联 `singleflightGroup` 实现缺少 `golang.org/x/sync/singleflight` 的关键特性：**panic 传播**。标准库版本会将 fn 内的 panic 安全捕获并转换为错误传播给所有等待者；自行实现的版本不做 panic recovery，一次 `doRefresh` 的 panic 会直接 crash 整个进程，而其他等待 `wg.Wait()` 的 goroutine 永久阻塞（wg.Done() 永远不会被调用）。

另外，`fn` 执行完毕后立即 `delete(g.m, key)` 的时机正确（避免 stale 结果被返回），但缺少关于"fn 执行期间新到来的 goroutine会 wait 而不是执行新的 fn"的注释，可读性偏低。

**修复**：建议添加 `defer` + `recover` 保护，或直接使用 `golang.org/x/sync`（该模块已在 Go 生态中视为准标准库），一劳永逸。

---

#### P1-5: `httpTransport.Recv()` 每次调用都新建 SSE GET 请求

**文件**: `internal/mcp/transport.go`（`httpTransport` 实现）

根据 [MCP Streamable HTTP 规范](https://modelcontextprotocol.io/docs/concepts/transports)，客户端应建立一条**持久的 SSE 连接**，所有服务器推送事件均通过该连接送达。而当前实现的 `httpTransport.Recv` 在每次调用时都发起一个新的 HTTP GET 请求来"监听" SSE 事件，导致：

1. **协议违反**：每次 `Recv` 打开一个新的 SSE 流；前一个响应流在下次调用前被丢弃，服务端推送的中间事件丢失。
2. **连接开销**：高频调用会产生大量短连接，浪费 TCP/TLS 握手资源。
3. **会话 ID 混乱**：每次 GET 请求均携带同一 `Mcp-Session-Id`，但服务端可能对多个并发 GET 报 409 冲突。

相比之下，`sseTransport` 正确地在构造时启动一个 background goroutine 持续读取 SSE 流，并通过内部 `recvCh(64)` 向 `Recv` 分发事件，是正确的模式。

**修复**：`httpTransport` 应参考 `sseTransport` 模式，在 `Connect`/`Initialize` 阶段启动一个持久 GET 连接并持续读入 `recvCh`；`Recv` 直接从该 channel 消费。

---

### P2 — Minor / Suggestions

#### P2-1: `machinePassword()` 熵值不足，静态 fallback 极弱

**文件**: `internal/oauth/crypto_aes.go` L146–152

```go
func machinePassword() []byte {
    hostname, err := os.Hostname()
    if err != nil || hostname == "" {
        hostname = "claude-code-fallback-host"   // ← 静态常量
    }
    return []byte("claude-code-token-key-v1:" + hostname)
}
```

PBKDF2 的安全性依赖"密码"的不可预测性。Hostname 在以下场景中熵值极低：
- **容器/CI 环境**：hostname 通常是短 UUID 或 pod 名，部分甚至固定为 `localhost`；
- **静态 fallback**：`"claude-code-fallback-host"` 完全公开，攻击者可离线暴力破解任何用该 fallback 加密的 tokens.enc；
- **共享主机**：多用户共享同一 hostname，任意用户可解密他人的 token 文件（若文件权限配置不严）。

PBKDF2 的 100k 次迭代提供一定保护，但密码空间过小（hostname 通常 < 256 种）会使暴力破解成本可接受。

**建议**：引入一个机器绑定的随机熵源（例如首次运行时生成并存储到 `~/.config/claude-code/machine-id` 的随机 bytes），或在 `machinePassword()` 中混合多个系统标识符（hostname + OS username + machine-id）以增加熵。

---

#### P2-2: `isOpusModel` 手工字符串匹配可用 `strings.Contains` 替代

**文件**: `internal/api/retry.go` L180–191

```go
func isOpusModel(model string) bool {
    for i := 0; i < len(model)-3; i++ {
        if model[i] == 'o' || model[i] == 'O' {
            sub := model[i:]
            if len(sub) >= 4 && (sub[:4] == "opus" || sub[:4] == "Opus") {
                return true
            }
        }
    }
    return false
}
```

逻辑上等价于 `strings.Contains(strings.ToLower(model), "opus")`，但手工循环可读性低且容易出 off-by-one（`len(model)-3` 若 model 长度 < 3 会返回负数，但 Go 不会崩溃，仅条件不满足时跳过）。建议使用标准库函数替代。

---

#### P2-3: `accumulate.go` 中 `input_json_delta` 初始化值 `""` 语义不明

**文件**: `internal/api/accumulate.go` L65–69

```go
case "input_json_delta":
    if block.Input == nil {
        block.Input = []byte(`""`)   // ← 初始化为 JSON 空字符串？
    }
    block.Text += delta.Delta.PartialJSON
```

`block.Input` 被初始化为 JSON 字符串 `""`（而不是 `null` 或 `{}`），但随后在 `EventContentBlockStop` 时被 `block.Text` 覆盖（L84）。该初始化值没有实际作用，且容易产生"tool_use input 是 JSON 字符串而不是对象"的误解。建议移除该初始化，或改为 `block.Input = []byte("null")` 并附注释。

---

#### P2-4: `factory.go` 的 `default` 分支静默返回 `directClient`

**文件**: `internal/api/factory.go` L87–93

```go
default:
    return &directClient{ ... }, nil
```

当传入未知的 `Provider` 字符串时，`NewClient` 不返回错误，而是静默降级为 `directClient`。这会掩盖配置错误（例如拼写错误的 provider 名）。建议 `default` 分支返回 `nil, fmt.Errorf("api: unknown provider: %s", cfg.Provider)`。

---

#### P2-5: `bedrockClient` / `vertexClient` 缺少实际的云鉴权

**文件**: `internal/api/factory.go` L96–142

`bedrockClient` 嵌入 `directClient` 但没有 AWS SigV4 签名；`vertexClient` 没有 GCP OAuth2 凭证注入。当前将发送未经签名的请求到云端 endpoint，导致 401/403。注释已说明"out of scope"，但：

1. `NewClient` 不返回任何警告，调用方无法感知；
2. 若上层代码按 provider 类型做分支处理，这些分支将静默失效。

**建议**：在构造函数中返回带有 `errors.ErrUnsupported` 包装的错误，或在 `Stream`/`Complete` 中注入 "not implemented" 前置检查，直到鉴权逻辑完成。

---

#### P2-6: `buildDefaultHeaders()` 与设计文档规定的 User-Agent / x-app 不符

**文件**: `internal/api/factory.go` L41–45

```go
func buildDefaultHeaders(version string) map[string]string {
    return map[string]string{
        "User-Agent": "claude-code-go/" + version,
    }
}
```

设计文档 §3.1 明确规定：
- `User-Agent: ClaudeCode/x.y.z Go/1.22`
- `x-app: cli`

当前实现：
- User-Agent 格式为 `claude-code-go/0.1.0`（前缀不一致，缺少 Go 版本）；
- 完全缺少 `x-app: cli` 头。

服务端可能依赖这些头进行路由/计费/遥测，格式不符会产生意外行为。建议使用 `runtime.Version()` 提供 Go 版本，并补充 `x-app` 头。

---

#### P2-7: `retry.go` 中 401 错误仅标记可重试，但无 token 刷新回调

**文件**: `internal/api/retry.go` L89–94

```go
case http.StatusUnauthorized: // 401
    isRetryable = true
```

设计文档 §3.3 描述：遇到 401 时应先 `triggerTokenRefresh()` 再重试，否则重试请求仍会因旧 token 失败，消耗重试次数徒劳。`WithRetry` 现在没有 `onUnauthorized` 回调参数，调用方无法挂入 `TokenManager.HandleOAuth401Error`。

**建议**：在 `RetryOptions`（或等价配置结构体）中增加 `OnUnauthorized func(ctx context.Context) error` 回调，在 `isRetryable && status==401` 时调用，成功后再执行下一次重试。

---

#### P2-8: `PersistentMaxBackoff` / `PersistentResetCap` 常量已声明但未使用

**文件**: `internal/api/retry.go` L26–27

```go
PersistentMaxBackoff = 5 * time.Minute
PersistentResetCap   = 6 * time.Hour
```

这两个常量在 `WithRetry` 实现中从未被引用，是设计文档持久重试逻辑的遗留骨架。未使用常量在 Go 中不会报错，但会给读者造成"持久重试已实现"的假象。建议：
- 若持久重试不在本期范围，加注释 `// TODO: persistent retry not yet implemented`；
- 若需要，实现对应逻辑。

---

#### P2-9: `oauth.RefreshToken()` 缺少超时控制

**文件**: `internal/oauth/client.go`

`ExchangeCodeForTokens` 正确设置了 15 秒超时（`context.WithTimeout`），但 `RefreshToken` 直接使用传入的 `ctx`，若调用方传入无超时的 background context，请求可能无限挂起。

**建议**：在 `RefreshToken` 内同样添加 `context.WithTimeout(ctx, 15*time.Second)` 或可配置的超时，与 `ExchangeCodeForTokens` 保持对称。

---

#### P2-10: `sseTransport` 的 POST endpoint URL 使用脆弱字符串拼接

**文件**: `internal/mcp/transport.go`（`sseTransport` 初始化）

```go
postURL: strings.TrimSuffix(url, "/sse") + "/message"
```

若服务器的 SSE URL 不以 `/sse` 结尾（如 `/events`、`/stream`），`TrimSuffix` 不生效，拼接结果为 `<url>/message`，导致请求发送到错误 endpoint。设计文档建议 POST URL 应由服务器在 SSE 握手响应头中提供（`endpoint` 事件），而不是客户端猜测。

**建议**：在 SSE 握手阶段从服务器推送的 `endpoint` 事件中读取 POST URL，并存入 `atomic.Value`，与 `sessionID` 并列管理。

---

#### P2-11: `pool.go` 的 `isMCPAuthError` 使用手工字符串扫描

**文件**: `internal/mcp/pool.go` L209–223

```go
for _, keyword := range []string{"401", "403", "unauthorized", "auth", "token"} {
    for i := 0; i < len(msg)-len(keyword)+1; i++ {
        if msg[i:i+len(keyword)] == keyword {
```

等价于 `strings.Contains(msg, keyword)`，手工实现无必要，且可读性低。另外，"token" 过于宽泛——"total token count" 会被误判为 auth 错误。建议替换为 `strings.Contains` 并精简关键词集合。

---

#### P2-12: `store.go` 的 `FileStore` 文档注释与实际加密格式不一致

**文件**: `internal/oauth/store.go` L82–96（文档注释）

```
// Format: [4-byte version][12-byte nonce][ciphertext+tag]
```

但实际格式（`crypto_aes.go` L45–49）为：
```
[4-byte version][32-byte salt][12-byte nonce][ciphertext+tag]
```

注释遗漏了 32-byte salt，与实现不符，会误导维护者。需更新注释。

---

#### P2-13: `DefaultToolTimeout` 类型为裸 `int`，单位混淆风险

**文件**: `internal/mcp/pool.go` L25

```go
const DefaultToolTimeout = 100_000_000 // ms — ~27.8h
```

常量定义为裸整数，意图为毫秒值但未被 `time.Millisecond` 乘以，使用时容易产生单位混淆。建议改为 `time.Duration` 类型（`= 100_000 * time.Second`）或在使用处显式转换。

---

#### P2-14: `json.go` 的 `jsonUnmarshal` 包装无附加值

**文件**: `internal/api/json.go`

```go
func jsonUnmarshal(data json.RawMessage, v any) error {
    return json.Unmarshal(data, v)
}
```

该函数仅是对 `json.Unmarshal` 的透明包装，无任何额外逻辑。在同一包内直接调用 `json.Unmarshal` 更清晰。若无扩展计划，建议删除，以减少认知负担。

---

#### P2-15: 服务层测试覆盖率为零

三个子包（`internal/api`、`internal/oauth`、`internal/mcp`）均无任何 `_test.go` 文件，而其他包（`engine`、`compact`、`permissions` 等）已有测试。关键路径缺乏测试：

| 路径 | 风险 |
|---|---|
| `WithRetry` 的 529/FallbackTriggered/context-cancel 路径 | 回归风险高 |
| `Accumulator.Process` 的 delta 累积（tool_use、thinking） | 正确性难目视验证 |
| `decryptTokenFile` 与 `encryptTokenFile` 往返 | 加密格式变更无保护 |
| `singleflightGroup.Do` 并发行为 | 并发 bug 难排查 |
| `NormalizeToolName` 的边界字符 | 工具名映射错误影响 API 调用 |

建议至少为以上路径补充单测，可使用 `MemoryStore` 避免 I/O 依赖。

---

## 4. Design vs Implementation Delta

| 设计文档规定 | 实现状态 | 差异等级 |
|---|---|---|
| `User-Agent: ClaudeCode/x.y.z Go/1.22` + `x-app: cli` | 实现为 `claude-code-go/0.1.0`，缺少 Go 版本和 `x-app` | P2-6 |
| `golang.org/x/sync/singleflight` | 内联自行实现，缺少 panic recovery | P2（P1-4） |
| 401 retry 调用 `triggerTokenRefresh()` | 仅标记 `isRetryable=true`，无回调 | P2-7 |
| `PersistentMaxBackoff` / `PersistentResetCap` 持久重试逻辑 | 常量声明但逻辑未实现 | P2-8 |
| SSE POST URL 从服务器 `endpoint` 事件获取 | 字符串拼接猜测 | P2-10 |
| `RefreshToken` 超时（设计与 `Exchange` 对称） | 无超时，依赖调用方 ctx | P2-9 |
| `MCPResourceContent` 类型 | 已实现（设计文档未明确定义，可接受扩展） | ✓ |
| 所有接口契约 | 全部实现 | ✓ |

---

## 5. Summary

| 维度 | 结论 |
|---|---|
| **正确性** | 总体正确；`accumulate.go` 的 `input_json_delta` 初始化值（P2-3）存在语义歧义但不影响最终结果 |
| **并发安全** | MCP jsonrpc 单 reader 模式已修复（P0-A）；`stdioTransport.Recv` goroutine 泄漏（P1-1）和 `WaitForCode` expectedState 竞争（P1-3）需修复；`httpTransport` 持久 SSE 连接（P1-5）需重构 |
| **接口契约** | 所有接口均已实现，主要与设计文档匹配；设计偏差见 §4 |
| **错误处理** | 主路径错误传播正确；`KeychainStore.Load` 静默吞错（P1-2）和 `factory.go` default 分支（P2-4）需修复 |
| **安全性** | AES-256-GCM 实现正确，PKCE 正确；**P0-1（token 进程参数暴露）必须修复**；`machinePassword` 熵值偏低（P2-1）；`singleflightGroup` panic 传播缺失（P1-4）是潜在稳定性风险 |
| **测试覆盖** | 服务层测试覆盖为零（P2-15），风险较高 |
| **Go 惯用法** | 整体符合；`isOpusModel`（P2-2）、`isMCPAuthError`（P2-11）手工字符串操作可改用标准库；`jsonUnmarshal` 包装（P2-14）可删除 |

**合并条件**：

- **必须修复（P0）**：`store_darwin.go` token 进程参数暴露是安全漏洞（P0-1），不可 merge。
- **必须修复（P1）**：`stdioTransport` goroutine 泄漏（P1-1）、`KeychainStore.Load` 吞错（P1-2）、`WaitForCode` 竞争（P1-3）、`httpTransport` 持久 SSE 连接（P1-5）；`singleflightGroup` panic 安全（P1-4）可在后续 sprint 修复但应创建 issue。
- **建议跟进（P2）**：其余 P2 项可作为 follow-up issue，不阻塞 merge，但 P2-15（测试覆盖）建议在 sprint 结束前至少补充 `api` 和 `oauth` 的核心路径测试；P2-6（User-Agent）和 P2-7（401 刷新回调）建议尽早修复以符合设计规范。

---

*Report version: v2.0 · 2026-04-03 · Tech Lead*
