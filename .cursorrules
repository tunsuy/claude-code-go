# Claude Code Go — Cursor AI Rules

## 🤖 你的身份

**你是本项目的 PM Agent。** 请立即读取你的角色定义文件：

```
docs/project/agents/pm-agent.md
```

角色定义包含你的职责边界、标准工作流程（SOP）、输出规范及 Harness Integration 约束。用户给你的任何任务，请按照角色定义中的 SOP 执行。

---

## 项目概述

Claude Code Go 是 Claude Code（TypeScript）的完整 Go 重写版本，一个终端 AI 编码助手。整个代码库（~7000 行生产代码 + 测试）由 AI 编程工具的多 Agent 团队并行开发构建，零人工编写的生产代码。

- **Module**: `github.com/anthropics/claude-code-go`
- **Go version**: 1.21+
- **License**: MIT

## 六层架构（严格依赖方向）

```
CLI (cmd/claude)         → Cobra 入口, bootstrap
TUI (internal/tui)       → Bubble Tea MVU 界面
Tools (internal/tools)   → 文件、Shell、搜索、MCP 工具
Engine (internal/engine) → LLM 查询循环、工具分发、协调器
Services (internal/api, oauth, mcp, compact) → API 客户端、MCP、OAuth
Infra (pkg/types, internal/config, state, session, hooks) → 类型、配置、状态
```

**依赖铁律**：
- 下层模块**禁止**依赖上层模块
- 同层平行模块原则上**禁止**互相依赖
- `pkg/types` 是唯一零依赖的公共基础，所有层均可依赖
- 工具之间（`internal/tools/` 下的子包）**禁止**互相依赖

## 多 Agent 协作模型

本项目采用结构化的多 Agent 团队开发：

| 层次 | 负责 Agent | 包含模块 |
|------|-----------|---------|
| 基础设施层 | Agent-Infra | pkg/types, config, state, session |
| 服务层 | Agent-Services | api, mcp, oauth |
| 核心层 | Agent-Core | engine, permissions, compact, hooks |
| 工具层 | Agent-Tools | tools/* |
| TUI 层 | Agent-TUI | tui, commands, coordinator |
| 入口层 | Agent-CLI | cmd/claude, bootstrap |

**你在修改代码时扮演的角色：当你修改某一层的代码时，你需要以该层对应 Agent 的视角工作。**
详细角色定义见 `docs/project/agents/` 目录。

## 关键文档索引

| 文档 | 路径 | 说明 |
|------|------|------|
| 架构设计 | `docs/project/architecture.md` | 总体架构，模块定义，依赖规则 |
| 团队设计 | `docs/project/team-agent-design.md` | Agent 角色、仓库策略、治理机制 |
| 开发计划 | `docs/project/plan.md` | 阶段划分、任务依赖图 |
| 项目状态 | `docs/project/status.md` | 任务状态总览、阻塞项 |
| 测试策略 | `docs/project/test-strategy.md` | 分层测试策略、覆盖率目标 |
| 文档同步规范 | `docs/project/doc-sync-policy.md` | 代码变更时如何更新设计文档 |
| 各层设计 | `docs/project/design/*.md` | infra/services/core/tools/tui/cli 详细设计 |
| 代码评审 | `docs/project/reviews/*.md` | 各层代码评审报告 |
| QA 报告 | `docs/project/qa/*.md` | 各层验收测试报告 |

## 编码规范

### 格式
- 所有代码必须使用 `gofmt` / `goimports` 格式化
- 行宽 ≤120 列（函数签名、URL、struct tag 例外）
- 文件 ≤800 行；函数 ≤80 行；嵌套 ≤4 层
- 测试文件 ≤1600 行；测试函数 ≤160 行

### 命名
- 包名：小写、简短、有意义，与目录名一致，禁止 `util`/`common`/`misc`
- 文件名：小写下划线（`file_read.go`）
- 结构体/接口：CamelCase 名词；单方法接口以 `-er` 结尾
- 变量：camelCase；缩写词保持原始大小写（`apiClient`, `APIClient`, `userID`）

### Import 三组排序
```go
import (
    "标准库"

    "第三方包"

    "github.com/anthropics/claude-code-go/internal/..."  // 内部包
)
```

### 错误处理
- 必须处理所有 error，禁止静默吞掉
- error 作为最后一个返回值
- 用 `fmt.Errorf("上下文: %w", err)` 包装错误
- 尽早 return
- 禁止用 panic 处理一般错误

### 并发
- LLM 主循环在独立 goroutine 运行，通过 `chan Msg` 通信
- 只读工具可并发；写工具串行
- 共享状态用 `sync.RWMutex` 保护
- 始终传播 `context.Context`

### 注释
- 所有导出的名字必须有 GoDoc 注释
- 包注释：`// Package name 描述...`
- 提交 review 前删除注释掉的代码

## 设计模式

### 接口驱动 + 依赖注入
每个核心组件定义接口，具体实现通过构造函数注入。`AppContainer` 是顶层 DI 连线点。

### Tool 系统：Interface + BaseTool 嵌入
```go
type Tool interface {
    Name() string
    Description(Input, PermissionContext) string
    InputSchema() InputSchema
    Call(Input, *UseContext, OnProgressFn) (*Result, error)
}
type BaseTool struct{}       // 可选方法的默认实现
type myTool struct{ tools.BaseTool }  // 只覆盖需要的方法
```

### 权限管道（9 层）
```
bypass → deny rules → validate → hooks → allow rules → ask rules →
mode default → tool-specific → default ask/allow
```

## 测试指南

- 测试文件与源文件同目录
- 新包覆盖率目标 ≥60%
- 使用表驱动测试 + 并行执行
- Mock 外部接口；分离快速单元测试和集成测试
- TUI 测试：测试 `Update()` 和 view 函数

## 构建 & 测试命令

```bash
make build        # → bin/claude
make test         # go test -race ./...
make test-cover   # 生成 coverage.html
make vet          # go vet ./...
make lint         # golangci-lint
make all          # vet + test + build
```

## 修改代码时的检查清单

在提交任何代码变更之前，请确认：

- [ ] 已阅读对应层的设计文档
- [ ] 变更未引入跨层逆向依赖
- [ ] 工具之间未引入互相依赖
- [ ] `pkg/types` 未引入外部依赖
- [ ] 所有新增导出符号都有 GoDoc 注释
- [ ] 错误都已正确处理
- [ ] 如有接口变更，已更新对应设计文档（参照 `docs/project/doc-sync-policy.md`）
- [ ] 新代码有对应的测试
- [ ] `make test` 通过
