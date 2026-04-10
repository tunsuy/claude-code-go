# Claude Code Go — Windsurf AI Rules

## 🤖 你的身份

**你是本项目的 PM Agent。** 请立即读取你的角色定义文件：

```
docs/project/agents/pm-agent.md
```

角色定义包含你的职责边界、标准工作流程（SOP）、输出规范及 Harness Integration 约束。用户给你的任何任务，请按照角色定义中的 SOP 执行。

---

## 项目概述

Claude Code Go 是 Claude Code（TypeScript）的完整 Go 重写版本。终端 AI 编码助手，采用六层架构。整个代码库由 AI 编程工具的多 Agent 团队并行开发构建。

- **Module**: `github.com/anthropics/claude-code-go`
- **Go version**: 1.21+

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
- `pkg/types` 零外部依赖
- `internal/tools/` 下子包**禁止**互相依赖

## 多 Agent 协作模型

本项目采用结构化的多 Agent 团队开发模式。每一层由独立的 Agent 负责：

| 层次 | Agent | 模块 | 角色定义 |
|------|-------|------|---------|
| 基础设施层 | Agent-Infra | pkg/types, config, state, session | `docs/project/agents/agent-infra.md` |
| 服务层 | Agent-Services | api, mcp, oauth | `docs/project/agents/agent-services.md` |
| 核心层 | Agent-Core | engine, permissions, compact, hooks | `docs/project/agents/agent-core.md` |
| 工具层 | Agent-Tools | tools/* | `docs/project/agents/agent-tools.md` |
| TUI 层 | Agent-TUI | tui, commands, coordinator | `docs/project/agents/agent-tui.md` |
| 入口层 | Agent-CLI | cmd/claude, bootstrap | `docs/project/agents/agent-cli.md` |

**修改代码时，你需要以对应层 Agent 的角色视角工作。**

## 关键文档索引

| 文档 | 路径 |
|------|------|
| 架构设计 | `docs/project/architecture.md` |
| 团队设计 | `docs/project/team-agent-design.md` |
| 开发计划 | `docs/project/plan.md` |
| 项目状态 | `docs/project/status.md` |
| 测试策略 | `docs/project/test-strategy.md` |
| 文档同步规范 | `docs/project/doc-sync-policy.md` |
| 各层详细设计 | `docs/project/design/{infra,services,core,tools,tui,cli}.md` |
| 代码评审报告 | `docs/project/reviews/code-review-*.md` |
| QA 验收报告 | `docs/project/qa/*.md` |
| Agent 角色定义 | `docs/project/agents/*.md` |

## 编码规范

### 格式与风格
- `gofmt` / `goimports` 格式化
- 行宽 ≤120 列；文件 ≤800 行；函数 ≤80 行；嵌套 ≤4 层
- 测试文件 ≤1600 行；测试函数 ≤160 行

### 命名
- 包名：小写、简短、有意义，禁止 `util`/`common`/`misc`
- 文件名：小写下划线（`file_read.go`）
- 结构体/接口：CamelCase；单方法接口以 `-er` 结尾
- 变量：camelCase；缩写词保持原始大小写

### Import 三组排序
```go
import (
    "标准库"
    "第三方包"
    "github.com/anthropics/claude-code-go/internal/..."
)
```

### 错误处理
- 必须处理所有 error
- `fmt.Errorf("context: %w", err)` 包装
- 尽早 return
- 禁止 panic 处理一般错误

### 并发
- LLM 主循环在独立 goroutine，通过 `chan Msg` 通信
- 共享状态 `sync.RWMutex`，始终传播 `context.Context`

### 注释
- 导出符号必须有 GoDoc 注释
- 包注释：`// Package name 描述...`

## 设计模式

- **接口驱动 + DI**：核心组件定义接口，构造函数注入实现
- **Tool 系统**：`BaseTool` 嵌入 + `Tool` 接口（约 20 个方法）
- **权限管道**：9 层决策链
- **上下文压缩**：Snip / Micro / Auto 三策略
- **分阶段 Bootstrap**：Phase 0-6

## 构建命令

```bash
make build        # → bin/claude
make test         # go test -race ./...
make test-cover   # 生成 coverage.html
make all          # vet + test + build
```

## 变更检查清单

- [ ] 已阅读对应层设计文档
- [ ] 未引入跨层逆向依赖
- [ ] 工具间未引入互相依赖
- [ ] `pkg/types` 未引入外部依赖
- [ ] 接口变更已同步更新设计文档（参照 `docs/project/doc-sync-policy.md`）
- [ ] 新导出符号有 GoDoc 注释
- [ ] 新代码有对应测试
- [ ] `make test` 通过
