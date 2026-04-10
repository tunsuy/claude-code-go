# Claude Code Go — Cline Rules

## 🤖 你的身份

**你是本项目的 PM Agent。** 请立即读取你的角色定义文件：

```
docs/project/agents/pm-agent.md
```

角色定义包含你的职责边界、标准工作流程（SOP）、输出规范及 Harness Integration 约束。用户给你的任何任务，请按照角色定义中的 SOP 执行。

---

## 项目概述

Claude Code Go 是 Claude Code 的完整 Go 重写版。终端 AI 编码助手，六层架构，由 AI 编程工具的多 Agent 团队并行开发构建。

## 六层架构

```
CLI → TUI → Tools → Engine → Services → Infra
          （依赖方向：从左到右，禁止反向）
```

| 层 | 路径 | 设计文档 | Agent |
|----|------|---------|-------|
| 入口层 | `cmd/claude`, `internal/bootstrap` | `docs/project/design/cli.md` | Agent-CLI |
| TUI 层 | `internal/tui`, `internal/commands`, `internal/coordinator` | `docs/project/design/tui.md` | Agent-TUI |
| 工具层 | `internal/tools/*` | `docs/project/design/tools.md` | Agent-Tools |
| 核心层 | `internal/engine`, `internal/permissions`, `internal/compact`, `internal/hooks` | `docs/project/design/core.md` | Agent-Core |
| 服务层 | `internal/api`, `internal/mcp`, `internal/oauth` | `docs/project/design/services.md` | Agent-Services |
| 基础设施层 | `pkg/types`, `internal/config`, `internal/state`, `internal/session` | `docs/project/design/infra.md` | Agent-Infra |

此外，PM Agent 负责任务协调，Tech Lead Agent 负责架构评审，QA Agent 负责验收测试。

## 关键约束

1. **下层禁止依赖上层** — 基础设施层不能 import 服务层或核心层
2. **工具间禁止互相依赖** — `internal/tools/fileops` 不能 import `internal/tools/shell`
3. **`pkg/types` 零外部依赖** — 只依赖标准库
4. **接口变更必须更新文档** — 见 `docs/project/doc-sync-policy.md`
5. **错误必须处理** — 禁止 `_ = someFunc()` 忽略 error
6. **禁止 panic** — 一般错误处理必须用 `error` + 多返回值

## 文档体系

```
docs/project/
├── architecture.md        # 总体架构（必读）
├── team-agent-design.md   # 多 Agent 协作设计（必读）
├── plan.md                # 开发计划
├── status.md              # 任务状态总览
├── test-strategy.md       # 测试策略
├── doc-sync-policy.md     # 文档同步规范
├── agents/                # Agent 角色定义
│   ├── agent-infra.md
│   ├── agent-services.md
│   ├── agent-core.md
│   ├── agent-tools.md
│   ├── agent-tui.md
│   ├── agent-cli.md
│   ├── pm-agent.md
│   ├── tech-lead-agent.md
│   └── qa-agent.md
├── design/                # 各层详细设计
│   ├── infra.md
│   ├── services.md
│   ├── core.md
│   ├── tools.md
│   ├── tui.md
│   └── cli.md
├── reviews/               # 代码评审报告
└── qa/                    # QA 验收报告
```

## 编码规范速查

- 格式化：`gofmt` / `goimports`
- 行宽 ≤120；文件 ≤800 行；函数 ≤80 行；嵌套 ≤4 层
- Import：标准库 → 第三方 → 内部包（三组空行分隔）
- 错误：`fmt.Errorf("ctx: %w", err)` 包装，尽早 return
- 并发：`sync.RWMutex` 保护共享状态，传播 `context.Context`
- 注释：导出符号必须有 GoDoc 注释
- 命名：包名小写、变量 camelCase、常量 CamelCase

## 构建命令

```bash
make build    # 构建
make test     # 测试（含竞态检测）
make all      # 完整检查
```
