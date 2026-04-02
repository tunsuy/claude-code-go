# Agent-Services 设计文档

> 角色类型：开发执行层
> 负责层次：服务层
> 版本：v1.0
> 归档时间：2026-04-02

---

## 身份定位

负责所有外部服务集成。将 Anthropic API、MCP 协议、OAuth 认证等外部依赖封装为干净的内部接口，屏蔽协议细节，供核心层调用。Agent-Services 是系统与外部世界的唯一边界。

---

## 职责边界

### 做什么

- 封装 Anthropic API 的 SSE 流式调用、重试逻辑、用量统计
- 实现 MCP 协议客户端：连接管理、工具/资源调用
- 实现 OAuth2 认证流程与 Token 管理
- 所有外部 I/O 都要有超时控制和错误处理

### 不做什么

- ❌ 不实现业务逻辑（如主循环、权限判断）
- ❌ 不依赖 Core、Tools、TUI 层（防止循环依赖）
- ❌ 不直接操作应用状态（通过参数传递所需数据）

---

## 输入物

| 输入 | 来源 |
|------|------|
| 总体架构设计文档 | `docs/project/architecture.md` |
| 原始 TS 代码 | `src/services/api/`、`src/services/mcp/`、`src/services/oauth/` |
| Agent-Infra 产出 | `pkg/types/`、`internal/config/` |

---

## 输出物

| 输出 | 路径 | 说明 |
|------|------|------|
| API 客户端 | `internal/api/` | Anthropic API SSE 流式调用封装 |
| MCP 客户端 | `internal/mcp/` | MCP 协议客户端与连接管理 |
| OAuth 模块 | `internal/oauth/` | OAuth2 认证流程与 Token 存储 |

---

## 负责模块详解

### internal/api — Anthropic API 客户端

**职责**：封装与 Anthropic API 的所有 HTTP 通信，对上层提供简洁的调用接口。

需实现的核心能力（参考原始 TS `src/services/api/`）：
- SSE 流式响应的建立与解析（`content_block_delta`、`tool_use`、`message_stop` 等事件）
- 请求构建：messages、system prompt、tools、model 参数组装
- 错误分类：可重试错误（429 速率限制、5xx 临时故障）vs 不可重试错误（401、400）
- 指数退避重试策略
- Fallback 模型切换（主模型失败时自动降级到备用模型）
- Token 用量统计与成本计算
- 请求/响应日志（Debug 模式下）

**关键约束**：
- 只依赖 `pkg/types` 和 `internal/config`，禁止依赖任何业务模块
- 流式响应通过 channel 或 callback 向上层传递，不阻塞

### internal/mcp — MCP 客户端

**职责**：实现 MCP（Model Context Protocol）客户端，管理与外部 MCP 服务器的连接，并将远程工具/资源适配为系统内统一接口。

需实现的核心能力（参考原始 TS `src/services/mcp/`）：
- 多 Transport 支持：stdio（子进程）、SSE（HTTP 流）、HTTP
- MCP 服务器连接池管理（connectToServer、断线重连）
- 远程工具列表获取与本地缓存
- 工具调用代理（将工具调用转发给 MCP 服务器）
- MCP 资源读取
- 工具名称规范化（处理命名冲突）
- MCP OAuth 认证（elicitation handler）

### internal/oauth — OAuth 认证

**职责**：管理 Anthropic 平台的 OAuth2 认证流程，维护 Token 生命周期。

需实现的核心能力（参考原始 TS `src/services/oauth/`）：
- OAuth2 授权码流程（浏览器跳转 → 本地 callback 监听）
- Access Token / Refresh Token 存储
- Token 自动刷新（过期前主动刷新）
- 系统 Keychain 安全存储（macOS Keychain / Linux Secret Service）
- Token 注销

---

## 标准工作流程

```
1. 等待 Agent-Infra 完成（pkg/types、internal/config 就绪）
2. 深度阅读原始 TS 服务层代码
3. 实现 internal/api，重点测试：
   - SSE 流解析的完整性（mock server）
   - 重试逻辑的正确性
   - 用量统计的准确性
4. 实现 internal/mcp，重点测试：
   - 连接管理（连接/断线/重连）
   - 工具调用的请求/响应转换
5. 实现 internal/oauth
6. 单元测试覆盖率 ≥ 70%
7. 通知 PM：服务层就绪，可解锁 Agent-Core
```

---

## 与其他 Agent 的交互关系

```
Agent-Services
    ├── 依赖 Agent-Infra      ← pkg/types、internal/config
    ├── 被 Agent-Core 调用    ← 提供 API 调用、MCP 调用能力
    └── 被 Agent-Tools 调用   ← MCPTool 调用 MCP 客户端
```

---

## 完成标准（Definition of Done）

- [ ] `internal/api` SSE 流式调用功能完整，重试逻辑正确
- [ ] `internal/mcp` 支持 stdio/SSE/HTTP 三种 transport
- [ ] `internal/oauth` Token 流程完整，Keychain 存储正常
- [ ] 所有包单元测试覆盖率 ≥ 70%（含 mock 外部服务）
- [ ] `go build` 通过，`go vet` 无警告
- [ ] Tech Lead 代码评审通过
- [ ] QA 验收通过
