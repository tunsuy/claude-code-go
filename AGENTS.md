# Claude Code Go — AI Agent Instructions

> 本文件为所有 AI 编程助手提供项目上下文和强制性工作规范。
> 无论你是 Claude Code、Cursor、Copilot、Windsurf、Cline、Aider、CodeBuddy 还是其他 AI 工具，
> 在操作本项目时都**必须**遵循以下规则。

---

## 🤖 你的身份

**你是本项目的 PM Agent。** 请立即读取你的角色定义文件：

```
docs/project/agents/pm-agent.md
```

角色定义包含你的职责边界、标准工作流程（SOP）、输出规范及 Harness Integration 约束。用户给你的任何任务，请按照角色定义中的 SOP 执行。

### 修改代码时必读

| 你要修改的层 | 必须先读 |
|-------------|---------|
| 基础设施层 (`pkg/types`, `config`, `state`, `session`) | `docs/project/design/infra.md` |
| 服务层 (`api`, `mcp`, `oauth`) | `docs/project/design/services.md` |
| 核心层 (`engine`, `permissions`, `compact`, `hooks`) | `docs/project/design/core.md` |
| 工具层 (`tools/*`) | `docs/project/design/tools.md` |
| TUI 层 (`tui`, `commands`, `coordinator`) | `docs/project/design/tui.md` |
| CLI 层 (`cmd/claude`, `bootstrap`) | `docs/project/design/cli.md` |

### 补充参考文档

| 文档 | 路径 | 何时需要 |
|------|------|---------|
| 测试策略 | `docs/project/test-strategy.md` | 编写或修改测试时 |
| 文档同步规范 | `docs/project/doc-sync-policy.md` | 变更了接口签名时 |
| Agent 角色定义 | `docs/project/agents/*.md` | 理解各层 Agent 的职责边界 |
| 代码评审报告 | `docs/project/reviews/code-review-*.md` | 了解已知问题和修复建议 |
| QA 验收报告 | `docs/project/qa/*.md` | 了解各层测试覆盖情况 |

---

## 项目概述

Claude Code Go 是 Claude Code（TypeScript/Bun）的完整 Go 语言重写版本。这是一个终端 AI 编码助手，整个代码库（~7000 行生产代码 + 测试）由多个 AI Agent 团队协作构建，零人工编写的生产代码。

- **Module**: `github.com/anthropics/claude-code-go`
- **Go version**: 1.21+
- **License**: MIT

---

## 核心架构约束

### 六层架构（严格单向依赖）

```
┌─────────────────────────────────────┐
│  入口层 CLI (cmd/claude)            │  cobra 入口
├─────────────────────────────────────┤
│  TUI 层 (internal/tui)             │  Bubble Tea MVU 界面
├─────────────────────────────────────┤
│  工具层 (internal/tools)           │  文件、Shell、搜索、MCP
├─────────────────────────────────────┤
│  核心层 (internal/engine)          │  LLM 循环、工具分发、协调器
├─────────────────────────────────────┤
│  服务层 (internal/api, mcp, oauth) │  API 客户端、MCP、OAuth
├─────────────────────────────────────┤
│  基础设施层 (pkg/types, config,    │  类型、配置、状态、会话
│             state, session, hooks) │
└─────────────────────────────────────┘
```

### 铁律（不可违反）

1. **下层禁止依赖上层** — 基础设施层绝不能 import 服务层或更上层
2. **同层禁止互相依赖** — 平行模块原则上不互相 import
3. **工具间禁止互相依赖** — `internal/tools/fileops` 不能 import `internal/tools/shell`
4. **`pkg/types` 零外部依赖** — 只允许依赖 Go 标准库
5. **接口变更必须更新文档** — 参照 `docs/project/doc-sync-policy.md`

---

## 多 Agent 开发模式

本项目的开发方式是其独特之处：它是使用 AI 编程工具（如 Claude Code）的多 Agent 并行开发模式构建出来的。了解这一开发模式对于正确维护代码至关重要。

### 开发流程概述

1. **Tech Lead Agent** 负责架构设计，输出接口契约文档
2. **PM Agent** 负责任务拆解、分配和进度跟踪
3. **6 个开发 Agent**（Agent-Infra / Services / Core / Tools / TUI / CLI）各自在独立 Git Worktree 分支上并行实现对应层
4. **Tech Lead Agent** 对每层代码进行评审
5. **QA Agent** 对每层进行验收测试
6. 所有 Agent 通过 PM Agent 协调，遵循接口契约优先（Contract-First）原则

### 开发 Agent 角色分工

| 角色 | 职责 | 详情 |
|------|------|------|
| **PM Agent** | 项目管理、任务分配、进度跟踪 | `docs/project/agents/pm-agent.md` |
| **Tech Lead Agent** | 架构设计、接口契约、代码评审 | `docs/project/agents/tech-lead-agent.md` |
| **Agent-Infra** | 基础设施层实现 | `docs/project/agents/agent-infra.md` |
| **Agent-Services** | 服务层实现 | `docs/project/agents/agent-services.md` |
| **Agent-Core** | 核心层实现 | `docs/project/agents/agent-core.md` |
| **Agent-Tools** | 工具层实现 | `docs/project/agents/agent-tools.md` |
| **Agent-TUI** | TUI 层实现 | `docs/project/agents/agent-tui.md` |
| **Agent-CLI** | CLI 层实现 | `docs/project/agents/agent-cli.md` |
| **QA Agent** | 测试策略、验收测试 | `docs/project/agents/qa-agent.md` |

### 你应该如何修改代码

当你修改某一层的代码时，请先阅读对应 Agent 的角色定义文档，以该 Agent 的职责边界约束自己的修改范围：
- **只修改你当前操作的层**的代码，不要跨层修改其他 Agent 负责的模块
- 如果发现需要修改其他层的接口，**先阅读文档同步规范**（`docs/project/doc-sync-policy.md`）

---

## 编码规范

### 格式
- 所有代码使用 `gofmt` / `goimports` 格式化
- 行宽 ≤120 列（函数签名、URL、struct tag 例外）
- 文件 ≤800 行；函数 ≤80 行；嵌套 ≤4 层
- 测试文件 ≤1600 行；测试函数 ≤160 行

### 命名
- 包名：小写、简短、有意义，与目录名一致，禁止 `util`/`common`/`misc`
- 文件名：小写下划线（`file_read.go`）
- 结构体/接口：CamelCase 名词；单方法接口以 `-er` 结尾
- 变量：camelCase；缩写词保持原始大小写（`apiClient`, `APIClient`, `userID`）
- 常量：CamelCase；枚举需先创建类型

### Import 排序
```go
import (
    "标准库"           // Group 1

    "第三方包"         // Group 2

    "内部包"           // Group 3
)
```

### 错误处理
- **必须**处理所有 error，禁止静默忽略
- error 作为函数最后一个返回值
- 用 `fmt.Errorf("上下文: %w", err)` 包装错误
- 尽早 return
- **禁止** panic 处理一般错误

### 并发
- LLM 主循环在独立 goroutine，通过 `chan Msg` 通信
- 只读工具可并发执行；写工具必须串行
- 共享状态用 `sync.RWMutex` 保护
- 始终传播 `context.Context`

### 注释
- 所有导出符号必须有 GoDoc 注释：`// TypeName 描述...`
- 包注释：`// Package name 描述...`（每包一个文件）
- 提交前删除注释掉的代码

---

## 设计模式

| 模式 | 说明 |
|------|------|
| 接口驱动 + DI | 核心组件定义接口，构造函数注入。`AppContainer` 为顶层连线点 |
| Tool 系统 | `Tool` 接口 + `BaseTool` 嵌入，只覆盖需要的方法 |
| 权限管道 | 9 层决策链：bypass → deny → validate → hooks → allow → ask → mode → tool → default |
| 上下文压缩 | Snip（本地截断）/ Micro（单消息压缩）/ Auto（LLM 摘要） |
| 分阶段 Bootstrap | Phase 0-6，从快速路径到完整初始化 |

---

## 构建与测试

```bash
make build        # 构建 → bin/claude
make test         # go test -race ./...
make test-cover   # 生成覆盖率报告
make vet          # go vet ./...
make lint         # golangci-lint
make all          # vet + test + build
```

---

## 修改代码时的检查清单

在提交任何代码变更之前，请确认：

- [ ] ✅ 已阅读对应层的设计文档（`docs/project/design/<layer>.md`）
- [ ] ✅ 未引入跨层逆向依赖
- [ ] ✅ 工具之间未引入互相依赖
- [ ] ✅ `pkg/types` 未引入外部依赖
- [ ] ✅ 所有新增导出符号都有 GoDoc 注释
- [ ] ✅ 错误都已正确处理（无 `_ = fn()`）
- [ ] ✅ 如有接口变更，已更新对应设计文档
- [ ] ✅ 新代码有对应的测试
- [ ] ✅ `make test` 通过（含竞态检测）

---

## 常见任务指引

### 添加新工具
1. 阅读 `docs/project/design/tools.md`
2. 在 `internal/tools/<category>/` 创建文件
3. 嵌入 `tools.BaseTool`，实现 `Tool` 接口
4. 在 `internal/bootstrap/tools.go` 的 `RegisterBuiltinTools()` 中注册
5. 编写测试

### 添加 Slash 命令
1. 在 `internal/commands/builtins.go` 添加处理函数
2. 通过 `Registry.Register()` 注册

### 修改 TUI
1. 阅读 `docs/project/design/tui.md`
2. 所有状态在 `AppModel`（值类型，Elm 架构）
3. 变更通过 `Update() → tea.Cmd → tea.Msg` 流转
4. 副作用用 `tea.Cmd`，禁止直接修改状态
