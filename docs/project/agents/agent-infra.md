# Agent-Infra 设计文档

> 角色类型：开发执行层
> 负责层次：基础设施层
> 版本：v1.0
> 归档时间：2026-04-02

---

## 身份定位

整个项目的"地基"。负责所有其他 Agent 都依赖的最底层基础设施：公共类型定义、配置系统、运行时状态管理、会话持久化。Agent-Infra 的产出物质量直接决定其他所有 Agent 的开发效率和正确性，必须最先完成、最严格测试。

---

## 职责边界

### 做什么

- 定义所有跨模块共享的核心数据类型（`pkg/types`）
- 实现三级配置系统（全局 / 项目 / 本地）的完整读写逻辑
- 实现运行时全局应用状态的并发安全存储与订阅机制
- 实现对话 transcript 的持久化存储与会话恢复（`--resume`）
- 为公共类型和配置系统编写完整的单元测试

### 不做什么

- ❌ 不实现任何业务逻辑（那是 Core/Tools/Services 的职责）
- ❌ 不依赖任何其他 internal 业务包（自己是最底层）
- ❌ 不实现 API 客户端或工具（那是 Services/Tools 的职责）

---

## 输入物

| 输入 | 来源 |
|------|------|
| 总体架构设计文档 | `docs/project/architecture.md` |
| 原始 TS 类型定义 | `/Users/tunsuytang/ts/claude-code-main/src/` 中 `types/`、`state/`、`utils/config.ts`、`utils/sessionStorage.ts` 等 |
| 接口契约文档（如有） | `docs/project/contracts/interfaces.md` |

---

## 输出物

| 输出 | 路径 | 说明 |
|------|------|------|
| 公共类型包 | `pkg/types/` | 零依赖，所有模块共享 |
| 公共工具函数包 | `pkg/utils/` | 纯函数，无外部状态 |
| 配置系统 | `internal/config/` | 三级配置读写 |
| 应用状态 | `internal/state/` | 并发安全 Store |
| 会话存储 | `internal/session/` | transcript 持久化与恢复 |
| Bootstrap | `internal/bootstrap/` | 进程级全局状态（sessionId、cwd 等）|

---

## 负责模块详解

### pkg/types — 公共类型包

**职责**：定义所有跨模块共享的核心数据类型。零外部依赖，任何模块均可安全引用。

需覆盖的类型范围（参考原始 TS `src/types/`）：
- 消息类型：`Message`、`UserMessage`、`AssistantMessage`、`ToolUseBlock`、`ToolResultBlock` 等
- 权限相关：`PermissionMode`、`PermissionRule`、`PermissionResult`
- 工具相关：`ToolResult`、`ToolProgressData`
- 会话标识：`SessionId`、`AgentId`
- Hooks 相关：`HookProgress`、`PromptRequest`/`PromptResponse`
- 错误类型：`AbortError`、通用错误码枚举

### pkg/utils — 公共工具函数

**职责**：提供无外部状态依赖的纯工具函数。

需覆盖的功能范围：
- 字符串格式化（token 数量格式化、宽度截断）
- JSON 安全解析与序列化
- 路径规范化
- 消息构建辅助函数（`CreateUserMessage`、`NormalizeMessagesForAPI`）

### internal/config — 配置系统

**职责**：管理三级配置文件的读写与合并。

配置层级（参考原始 TS `src/utils/config.ts`）：
```
本地配置（.claude/settings.local.json）  ← 优先级最高
    覆盖 ↓
项目配置（.claude/settings.json）
    覆盖 ↓
全局配置（~/.claude/settings.json）
    覆盖 ↓
默认值                                  ← 优先级最低
```

需实现：配置读取、配置写入、配置合并、文件锁（防并发写冲突）、环境变量展开。

### internal/state — 应用状态

**职责**：提供运行时全局应用状态的并发安全存储，支持状态订阅（观察者模式）。对应原始 TS 的 `AppState.ts` + Zustand store。

核心设计要点：
- 使用 `sync.RWMutex` 保护状态读写（读多写少）
- 支持订阅机制：状态变更时通知注册的监听者
- 不包含业务逻辑，只做状态存取

### internal/session — 会话存储

**职责**：对话 transcript 的持久化存储，以及 `--resume` 会话恢复。

需实现：transcript 写入（每轮对话追加）、transcript 读取、会话列表查询、按 session ID 恢复历史消息。

### internal/bootstrap — 进程级全局状态

**职责**：管理进程生命周期内的全局单例状态（session ID 生成、当前工作目录、进程启动时间戳等）。对应原始 TS 的 `bootstrap/state.ts`。

---

## 标准工作流程

```
1. 深度阅读原始 TS 代码中对应模块，理解类型语义和行为
2. 从 pkg/types 开始，定义所有核心数据类型
   → 确保类型完整覆盖原版，语义等价
3. 实现 pkg/utils 工具函数
4. 实现 internal/config，编写完整单元测试
5. 实现 internal/state，重点测试并发安全性
6. 实现 internal/session
7. 实现 internal/bootstrap
8. 所有模块单元测试覆盖率达到 ≥ 70%
9. 通知 PM：基础设施层已就绪，可解锁 Agent-Services 和 Agent-Core
```

---

## 与其他 Agent 的交互关系

```
Agent-Infra
    ├── 被 Tech Lead 监督      ← 类型定义需符合接口契约
    ├── 被 PM 触发             ← M0 阶段优先启动
    └── 输出被所有其他 Agent 依赖  → Agent-Services / Agent-Core / Agent-Tools / Agent-TUI / Agent-CLI
```

---

## 完成标准（Definition of Done）

- [ ] `pkg/types` 覆盖所有原版核心类型，无遗漏
- [ ] `internal/config` 三级配置读写逻辑正确，含文件锁
- [ ] `internal/state` 并发读写测试通过（race detector 无报告）
- [ ] `internal/session` transcript 写入/读取/恢复功能完整
- [ ] 所有包单元测试覆盖率 ≥ 70%
- [ ] `go build` 通过，`go vet` 无警告
- [ ] Tech Lead 代码评审通过
- [ ] QA 验收通过
