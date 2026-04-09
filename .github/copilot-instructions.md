# Claude Code Go — GitHub Copilot Instructions

## ⚠️ 强制要求：先阅读项目文档

你正在操作一个**多 Agent 协作开发**的 Go 项目。在进行任何代码修改之前，你**必须**：

1. 阅读 `docs/project/architecture.md` — 理解六层架构和模块边界
2. 阅读 `docs/project/team-agent-design.md` — 理解多 Agent 协作模式
3. 阅读 `docs/project/status.md` — 了解当前项目状态和已知问题
4. 如果要修改某一层的代码，必须先阅读对应的设计文档 `docs/project/design/<layer>.md`

**如果你没有阅读上述文档就开始修改代码，很可能会破坏架构约束或引入跨层依赖问题。**

---

## 项目简介

Claude Code Go 是 Claude Code（TypeScript）的完整 Go 重写版本。这是一个终端 AI 编码助手，采用六层架构，由多个 AI Agent 协作构建。

## 架构概览

```
入口层 CLI (cmd/claude)         → Cobra 入口, bootstrap
TUI 层 (internal/tui)           → Bubble Tea MVU 界面
工具层 (internal/tools)         → 文件、Shell、搜索、MCP 工具
核心层 (internal/engine)        → LLM 查询循环、工具分发
服务层 (internal/api, mcp, oauth) → API 客户端、MCP、OAuth
基础设施层 (pkg/types, config, state) → 类型、配置、状态
```

### 铁律
- 下层模块**禁止**依赖上层模块
- `pkg/types` 零外部依赖
- `internal/tools/` 下子包**禁止**互相依赖

## 必读文档

| 场景 | 必须阅读 |
|------|---------|
| 理解整体架构 | `docs/project/architecture.md` |
| 理解协作模式 | `docs/project/team-agent-design.md` |
| 修改基础设施层 | `docs/project/design/infra.md` |
| 修改服务层 | `docs/project/design/services.md` |
| 修改核心层 | `docs/project/design/core.md` |
| 修改工具层 | `docs/project/design/tools.md` |
| 修改 TUI 层 | `docs/project/design/tui.md` |
| 修改 CLI 层 | `docs/project/design/cli.md` |
| 变更接口签名 | `docs/project/doc-sync-policy.md` |
| 编写测试 | `docs/project/test-strategy.md` |

## 多 Agent 开发模式

本项目由以下 AI Agent 角色分工开发。修改某层代码时，请先阅读对应 Agent 的角色定义文档，遵循其职责边界：

| 层次 | 开发 Agent | 角色定义文档 |
|------|-----------|------------|
| 基础设施层 | Agent-Infra | `docs/project/agents/agent-infra.md` |
| 服务层 | Agent-Services | `docs/project/agents/agent-services.md` |
| 核心层 | Agent-Core | `docs/project/agents/agent-core.md` |
| 工具层 | Agent-Tools | `docs/project/agents/agent-tools.md` |
| TUI 层 | Agent-TUI | `docs/project/agents/agent-tui.md` |
| 入口层 | Agent-CLI | `docs/project/agents/agent-cli.md` |

另有 PM Agent（任务协调）、Tech Lead Agent（架构评审）、QA Agent（验收测试）负责项目治理。

## 编码规范摘要

- `gofmt` / `goimports` 格式化
- 行宽 ≤120；文件 ≤800 行；函数 ≤80 行；嵌套 ≤4 层
- Import 三组：标准库 / 第三方 / 内部包
- error 作为最后返回值，`fmt.Errorf("ctx: %w", err)` 包装
- 禁止 panic 处理一般错误
- 共享状态用 `sync.RWMutex`，始终传播 `context.Context`
- 导出符号必须有 GoDoc 注释

## 设计模式

- **接口驱动 + DI**：核心组件定义接口，构造函数注入实现
- **Tool 系统**：`BaseTool` 嵌入 + `Tool` 接口
- **权限管道**：9 层决策链
- **上下文压缩**：Snip / Micro / Auto 三策略

## 变更检查清单

- [ ] 已阅读对应层设计文档
- [ ] 未引入跨层逆向依赖
- [ ] 接口变更已同步更新设计文档
- [ ] 新导出符号有 GoDoc 注释
- [ ] 新代码有测试
- [ ] `make test` 通过
