# Claude Code Go — Local Development Context

> This file provides workspace-specific context for Claude Code sessions in this project.

## ⚠️ MANDATORY: Read Before Any Changes

**本项目是使用 AI 编程工具的多 Agent 并行开发模式构建出来的。** 每次新对话开始时，必须先阅读以下核心文档：
- `docs/project/architecture.md` — 六层架构和依赖规则
- `docs/project/team-agent-design.md` — 了解本项目的 AI 多 Agent 开发模式（角色分工、开发流程、治理机制）
- 修改某一层时，读 `docs/project/design/<layer>.md` 和 `docs/project/agents/agent-<layer>.md`

## 🤖 Multi-Agent Session Start Checklist

**以 Agent 角色参与本项目时，每次新对话必须执行以下步骤：**

1. **读取项目状态快照**（机器可读，跨会话持久化）：
   ```
   .claude/harness/state.json
   ```
   该文件包含当前里程碑、任务状态、测试通过情况，是恢复上下文的最快入口。

2. **读取你的角色定义**（含 Harness Integration 章节）：
   ```
   docs/project/agents/<your-role>.md
   ```
   例如：`docs/project/agents/pm-agent.md`、`docs/project/agents/agent-cli.md`

3. **查阅任务注册表**（如需了解任务详情或更新任务状态）：
   ```
   docs/project/harness/tasks/task-registry.yaml
   ```

4. **完成任务后按输出协议汇报**：
   ```
   docs/project/harness/protocols/agent-output.md
   ```

> **为何不在 `.claude/` 目录**：Harness 文档（角色定义、任务注册表、输出协议）存放在 `docs/project/` 下，以确保非 Claude Code CLI 工具（如 Cursor、Windsurf）的用户也能访问这些协作规范。只有 Claude Code 运行时文件（`state.json`、`settings.local.json`、hooks）保留在 `.claude/` 下。

## Environment Setup

```bash
# Required
export ANTHROPIC_API_KEY=sk-ant-...  # or use OAuth: claude /config

# Build
make build    # output: bin/claude
make all      # full check: vet + test + build
```

## Quick Reference

### Frequently Modified Paths

| What | Where |
|------|-------|
| CLI entry | `cmd/claude/main.go` |
| Bootstrap / DI | `internal/bootstrap/wire.go` |
| Root command flags | `internal/bootstrap/root.go` |
| Tool registration | `internal/bootstrap/tools.go` |
| Query engine | `internal/engine/engine.go`, `query.go` |
| Tool interface | `internal/tools/tool.go` |
| Tool base class | `internal/tools/base.go` |
| Tool registry | `internal/tools/registry.go` |
| TUI model | `internal/tui/model.go` |
| TUI update loop | `internal/tui/update.go` |
| Permission checker | `internal/permissions/checker.go` |
| Config loader | `internal/config/loader.go` |
| App state | `internal/state/store.go` |
| Shared types | `pkg/types/*.go` |

### Permission Configuration

Permissions are configured in `.claude/settings.local.json`:
```json
{
  "permissions": {
    "allow": [
      "Bash(go build:*)",
      "Bash(go test:*)",
      "Read(/path/to/allowed/**)"
    ]
  }
}
```

### Branch Naming Convention

- `feat/<description>` — new features
- `fix/<description>` — bug fixes
- `docs/<description>` — documentation only
- `refactor/<description>` — code refactoring
- `test/<description>` — adding or improving tests

## Debugging Tips

- Use `--debug` flag to enable debug logging
- Use `--verbose` for verbose output
- Set `CLAUDE_CODE_ENGINE_MSG_BUF_SIZE` env var to adjust the engine message buffer (default: 256)
- Coverage report: `make test-cover` generates `coverage.html`

## Important Invariants

1. **Layer isolation**: `internal/` packages follow strict dependency direction (CLI → TUI → Tools → Engine → Services → Infra)
2. **Zero-dependency types**: `pkg/types` has NO external imports — only stdlib
3. **Tool independence**: Tools in `internal/tools/` MUST NOT import each other
4. **Concurrent safety**: All shared state accessed via `sync.RWMutex`; `AppStateStore` uses copy-on-write for maps/slices
5. **Context propagation**: All I/O functions take `context.Context` as first parameter

## Architecture Documents

- [`docs/project/architecture.md`](docs/project/architecture.md) — detailed architecture design (Chinese)
- [`docs/project/design/`](docs/project/design/) — per-layer design docs (6 files)
- [`docs/project/qa/`](docs/project/qa/) — QA test reports and final sign-off
- [`docs/project/reviews/`](docs/project/reviews/) — code review reports
