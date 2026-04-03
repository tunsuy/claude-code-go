# Tech Lead 评审：services.md

> 评审人：Tech Lead
> 日期：2026-04-02
> 结论：**有条件通过（APPROVED_WITH_CHANGES）**

---

## 总体评估

服务层设计全面，正确地将三个子包（`internal/api`、`internal/mcp`、`internal/oauth`）识别为唯一的外部边界。核心接口契约健全。SSE 解析策略、PKCE 实现、重试逻辑和 MCP 传输抽象均忠实对应 TypeScript 源码。所需修改适度，主要集中在缺失的弹性细节和一处依赖解决空白。

---

## 优点

1. **`bufio.Scanner` 实现 SSE 解析** — 正确、轻量的方案。文档明确避免第三方 SSE 库，降低依赖面。`sseReader.Next()` 设计与 TS SDK 的 `AsyncIterable` 契约映射清晰（§1.3）。

2. **重试常量忠实移植** — 五个常量（`DefaultMaxRetries=10`、`BaseDelayMS=500`、`MaxDelayMS=32_000`、`Max529Retries=3`、`PersistentMaxBackoff=5min`、`PersistentResetCap=6h`）与 TS 源码完全吻合（§1.4）。抖动公式（`delay * (1 + rand*0.25)`）完整保留。

3. **PKCE RFC 7636 合规** — `GenerateCodeVerifier`、`GenerateCodeChallenge`（S256/SHA-256/base64url）和 `GenerateState` 均使用 `crypto/rand`。`IsTokenExpired` 中 5 分钟令牌过期缓冲与 TS 行为一致（§3.2、§3.4）。

4. **`singleflight` 防并发刷新** — `refreshGroup.Do("refresh", ...)` 模式正确防止并发令牌刷新竞争，忠实复现 TS 的 `refreshPromise` 机制（§3.6、§5.5）。

5. **MCP 传输抽象** — 四种传输类型（`stdio`、`sse`、`http`、`ws`）枚举清晰，各有独立配置结构体。`Transport` 接口使用 `Send`/`Recv`/`Close` 满足 JSON-RPC 2.0 要求（§2.2）。

6. **连接池状态机** — 五种连接状态（`connected`、`failed`、`needs-auth`、`pending`、`disabled`）及错误驱动的状态转换完整，与 TS 的 `MCPServerConnection` 类型正确对应（§2.3）。

7. **工具名称规范化** — `serverName__toolName` 双下划线约定及 `NormalizeToolName` 字符过滤器（`[a-zA-Z0-9_-]`）与 TS 的 `buildMcpToolName()` 完全一致（§2.4、§5.6）。

8. **错误分层** — 三层错误层次（`transport → api.APIError → CannotRetryError/FallbackTriggeredError`）为核心层提供了干净的类型化错误处理（§5.7）。

9. **泛型 `WithRetry[T]`** — 使用 Go 1.21 泛型消除了 `interface{}` 类型转换，相比基于 `interface{}` 的设计类型安全性更高（§5.3）。

10. **`Usage` 类型完整性** — Beta API 字段（`cache_creation_input_tokens`、`service_tier`、`speed`、`inference_geo`、`web_search_requests`、`web_fetch_requests`）均已覆盖（§1.5）。

---

## 问题

**【严重】MCP 适配器存在无存根的 `TODO(dep)`**
§2.4 中 `AdaptToTool` 返回 `interface{}`，没有任何临时存根。依赖 MCP 工具注册的其他包（tools、tui）在 Agent-Core #6 完成前无法编译。至少应在 `pkg/types` 中立即定义一个占位接口作为返回类型，使 MCP 适配器可独立编译。

**【严重】`TokenManager` 使用 `sync.Mutex + bool` 而非 `singleflight`**
§3.6 文档描述 `TokenManager` 含 `mu sync.Mutex` 和 `refreshing bool` 字段，但 §5.5 正确规定了 `singleflight`。两处描述相互矛盾。§5.5 的 `singleflight` 方案更优（避免手动 mutex + 标志位管理，正确处理 context 取消）。`TokenManager` 结构体中的 `refreshing bool` 模式必须移除，改用 `singleflight.Group`。

**【次要】MCP SDK 选型不确定性未作为风险记录**
§4 正确指出 Go MCP SDK 处于"早期预览"阶段。应正式登记此风险：若 SDK API 在实现阶段前发生变更，`Transport` 接口可能需要修订。建议固定特定 commit SHA 并在 `go.sum` 中添加相应条目。

**【次要】非 macOS 平台的 `FileStore` 加密方案未指定**
§3.3 将 `FileStore` 命名为"加密文件存储"（路径 `$XDG_CONFIG_HOME/claude-code/tokens.enc`），但未指定加密算法（AES-GCM？ChaCha20-Poly1305？）或密钥派生方法。这是一处安全关键空白。

**【次要】`AuthCodeListener` 端口冲突处理缺失**
§3.5：`Start(port int)` 传入 `0` 以由 OS 随机分配，但若调用方传入非零端口（如 `MCPOAuthConfig` 中的配置 `callbackPort`）且该端口已被占用，错误处理路径未说明。需明确添加"失败后回退至随机端口"的逻辑。

**【次要】`HeartbeatInterval = 30 * time.Second` 无使用上下文**
§1.4 在重试常量旁声明了 `HeartbeatInterval`，但文档中没有任何地方描述心跳消费者。请说明哪个组件读取该常量以及在何种模式下适用（UNATTENDED_RETRY？）。

**【次要】`ws`（WebSocket）传输配置结构体缺失**
§2.2 将 `TransportWS` 作为常量枚举，却未提供 `WSTransportConfig` 结构体，与其他三种传输类型不一致。请提供配置结构体，或明确声明 WebSocket 在 Go 实现中延期/不支持。

**【次要】`SSETransportConfig` 将 SSE 与 Streamable HTTP 混淆**
§2.2 指出 `SSETransportConfig`"兼容 StreamableHTTP"，但两者差异显著：Streamable HTTP 使用 `Mcp-Session-Id`、单个双向连接；传统 SSE 使用 GET SSE + POST。兼容声明需要具体实现说明，或将两者拆分为独立传输类型。

---

## 必须修改项

1. **立即解决 `AdaptToTool` 返回类型问题。** 在 `pkg/types` 中定义最小 `Tool` 接口（即使是空存根），使 MCP 适配器可编译。在实现冲刺开始前与 Agent-Core 协商确定最终接口。

2. **将 `TokenManager` 统一为仅使用 `singleflight`。** 从 §3.6 结构体中移除 `mu sync.Mutex` + `refreshing bool` 字段。§5.5 的代码片段为规范设计，更新 §3.6 与之匹配。

3. **指定 `FileStore` 加密算法。** 在 §3.3 中新增子章节，说明所选算法（推荐 AES-256-GCM + PBKDF2/Argon2 基于机器 ID 的密钥派生）、密钥存储位置及未来算法升级路径。

4. **说明 `AuthCodeListener` 端口回退行为。** 为固定端口被占用的情况添加显式伪代码：尝试随机端口 N 次，耗尽后以用户可见错误失败。

5. **明确 `HeartbeatInterval` 使用场景。** 将常量移至消费它的模块（如重试监视器），或添加注释引用 TS 源码中的 UNATTENDED_RETRY 心跳机制。

---

## 实现注意事项

- §1.3 中的 `Accumulator` 需要互斥锁（`sync.Mutex`）保护，因为 `Process` 和 `Result` 可能在流式管道中被不同协程调用。确保使用 `sync.Mutex` 而不只是文档说明。
- `ParseContextOverflowError` 返回 `(inputTokens, contextLimit int, ok bool)` 是正确的 Go 惯用法（避免非匹配时 panic）。验证正则表达式与 Anthropic API 实际错误消息格式完全吻合。
- `sseReader.Next()` 中的 `[DONE]` 哨兵是 OpenAI/SSE 约定；Anthropic SSE 使用 `event: message_stop` + `data: {}`。验证实现不检查 `[DONE]`，而是正确处理 `EventMessageStop`。
- 对于工厂中的 Bedrock/Vertex 客户端：说明它们是否与 Direct 共用相同的 `MessageRequest` 序列化，或需要特定于提供商的转换。这是可能导致实现分歧的非平凡细节。
- `Max529Retries=3` 触发 Opus 模型的 `FallbackTriggeredError`。回退模型字符串必须可配置（不能硬编码），因为模型名称会变更。验证 `RetryOptions.FallbackModel` 始终由调用方填充，而非在 `WithRetry` 内部默认为硬编码字符串。
- 考虑为 `TokenStore.Load()` 和 `TokenStore.Save()` 添加 `context.Context`。Keychain 操作在密钥链锁定时可能阻塞数秒；context 截止时间允许启动阶段快速失败。

---

*评审版本：v1.0 · 2026-04-02 · Tech Lead*
