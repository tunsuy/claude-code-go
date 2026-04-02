# Agent-Core 设计文档

> 角色类型：开发执行层
> 负责层次：核心层
> 版本：v1.0
> 归档时间：2026-04-02

---

## 身份定位

整个系统的"大脑"。负责驱动 LLM 对话的核心机制：查询引擎、工具编排、权限系统、上下文压缩、Hooks 系统。Agent-Core 的产出物是所有上层（Tools、TUI、CLI）赖以运转的核心引擎，复杂度最高，需要最深入地理解原始 TS 代码。

---

## 职责边界

### 做什么

- 实现 LLM 会话管理与主循环驱动（QueryEngine + query loop）
- 实现工具并发编排（并发只读工具 / 串行写操作工具）
- 实现工具调用权限检查（allow / deny / ask-user 决策）
- 实现对话历史上下文压缩（Auto-compact / Micro-compact / Snip-compact）
- 实现 Hooks 系统（Pre/Post tool hooks、session hooks、sampling hooks）
- 定义工具接口规范（供 Agent-Tools 实现）

### 不做什么

- ❌ 不实现具体工具（那是 Agent-Tools 的职责）
- ❌ 不实现 TUI 渲染（那是 Agent-TUI 的职责）
- ❌ 不直接处理 CLI 参数（那是 Agent-CLI 的职责）

---

## 输入物

| 输入 | 来源 |
|------|------|
| 总体架构设计文档 | `docs/project/architecture.md` |
| 原始 TS 代码 | `src/QueryEngine.ts`、`src/query.ts`、`src/utils/permissions/`、`src/services/compact/`、`src/utils/hooks/`、`src/Tool.ts` |
| Agent-Infra 产出 | `pkg/types/`、`internal/config/`、`internal/state/` |
| Agent-Services 产出 | `internal/api/`、`internal/mcp/` |

---

## 输出物

| 输出 | 路径 | 说明 |
|------|------|------|
| 工具接口定义 | `internal/tool/` | Tool interface，供 Agent-Tools 实现 |
| 查询引擎 | `internal/engine/` | 会话管理与主循环 |
| 权限系统 | `internal/permissions/` | 工具调用权限决策 |
| 上下文压缩 | `internal/compact/` | token 窗口管理 |
| Hooks 系统 | `internal/hooks/` | Pre/Post hooks 执行 |

---

## 负责模块详解

### internal/tool — 工具接口定义

**职责**：定义统一的 Tool 接口规范，是查询引擎与所有具体工具实现之间的契约。Agent-Tools 必须实现此接口。

需定义的内容（参考原始 TS `src/Tool.ts`）：
- `Tool` interface：工具名称、描述、输入 Schema、权限类型声明、执行函数签名
- `ToolUseContext`：工具执行上下文（session 信息、权限检查回调等）
- `ToolResult`：工具执行结果（数据、新消息、错误信息）
- `ToolProgress`：流式进度上报类型
- 工具注册表（Registry）：全局工具集合，支持按名查找

**关键约束**：此包是 Core 与 Tools 之间的边界，定义后不得随意修改（变更需走接口契约流程）。

### internal/engine — 查询引擎

**职责**：管理单次 Claude 会话的完整生命周期，驱动 LLM 请求-响应主循环。这是整个系统复杂度最高的模块。

需实现的核心能力（参考原始 TS `src/QueryEngine.ts` + `src/query.ts`）：

**会话管理**：
- 消息历史维护（追加用户消息、助手消息、工具结果）
- Cost 累计与 token 用量跟踪
- Turn 计数与最大轮数限制

**主循环（query loop）**：
- 系统提示构建（内存文件注入、coordinator 上下文注入、token 预算注入）
- 调用 API 客户端，接收 SSE 流
- 流事件处理：文字内容推送到 TUI、tool_use 块收集
- stop_reason 判断：`end_turn` 退出循环，`tool_use` 进入工具编排
- 工具编排：划分可并发批次（只读）和串行批次（写操作），调用各工具
- 追加 tool_result，继续下一轮循环
- 每轮循环前触发压缩检查

**工具并发编排**：
- 根据工具的 `readOnly` 属性划分并发 / 串行批次
- 并发批次使用 `errgroup` 执行
- 串行批次顺序执行
- 每个工具执行前调用权限系统检查

### internal/permissions — 权限系统

**职责**：在工具执行前进行权限决策，给出 allow / deny / ask-user 三种结论。

需实现的核心能力（参考原始 TS `src/utils/permissions/`）：
- 多级规则匹配：全局 → 项目 → 本地配置叠加，优先级从低到高
- 文件路径权限校验：检查工具访问的路径是否在允许范围内
- Shell 命令规则匹配：精确匹配 + glob 通配符
- ask-user 决策：通过 channel 向 TUI 发送 `PermissionRequest`，阻塞等待用户响应
- 拒绝原因跟踪：记录权限拒绝历史，注入到系统提示中（告知 LLM 哪些操作被禁止）

### internal/compact — 上下文压缩

**职责**：当对话 token 数接近上下文窗口上限时，触发压缩策略，保证对话可持续进行。

需实现的压缩策略（参考原始 TS `src/services/compact/`）：
- **Auto-compact**：token 超过阈值时，调用 LLM 对历史消息做摘要，替换原始消息
- **Micro-compact**：针对单条超长消息（如大文件读取结果）的局部截断
- **Snip-compact**：移除历史消息并标记，保留关键上下文

### internal/hooks — Hooks 系统

**职责**：在工具调用前后、采样前后、会话开始/结束时，执行用户配置的自定义 hook。

需实现的 hook 类型（参考原始 TS `src/utils/hooks/`）：
- **PreToolUse**：工具执行前 hook，可返回 block（阻止工具执行）
- **PostToolUse**：工具执行后 hook
- **PostSampling**：LLM 采样完成后 hook
- **SessionStart / SessionStop**：会话生命周期 hook

---

## 标准工作流程

```
1. 等待 Agent-Infra 和 Agent-Services 完成
2. 深度阅读原始 TS 核心模块代码（QueryEngine.ts 最复杂，需反复阅读）
3. 先定义 internal/tool 接口（其他模块依赖此接口）
   → 与 Tech Lead 确认接口契约后再开始其他模块
4. 实现 internal/permissions（相对独立，可早期完成）
5. 实现 internal/compact
6. 实现 internal/hooks
7. 最后实现 internal/engine（依赖上述所有模块）
   → 主循环是最核心最复杂的部分，需大量测试
8. 单元测试覆盖率 ≥ 70%
9. 通知 PM：核心层就绪，可解锁 Agent-Tools 和 Agent-TUI
```

---

## 与其他 Agent 的交互关系

```
Agent-Core
    ├── 依赖 Agent-Infra        ← pkg/types、internal/config、internal/state
    ├── 依赖 Agent-Services     ← internal/api（LLM 调用）、internal/mcp
    ├── 输出 Tool 接口           → Agent-Tools 实现此接口
    ├── 输出查询引擎             → Agent-TUI 调用 QueryEngine
    └── 输出权限系统接口          → Agent-TUI 传递权限确认 channel
```

---

## 完成标准（Definition of Done）

- [ ] `internal/tool` 接口定义完整，Tech Lead 确认契约
- [ ] `internal/engine` 主循环功能完整（含工具编排、并发控制）
- [ ] `internal/permissions` 多级规则匹配正确，ask-user 流程完整
- [ ] `internal/compact` 三种压缩策略均已实现
- [ ] `internal/hooks` 四种 hook 类型均已实现
- [ ] 主循环集成测试通过（含 mock LLM 响应）
- [ ] 所有包单元测试覆盖率 ≥ 70%
- [ ] `go build` 通过，`go vet` 无警告，race detector 无报告
- [ ] Tech Lead 代码评审通过
- [ ] QA 验收通过
