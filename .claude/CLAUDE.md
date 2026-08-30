# Claude Code Go — Local Development Context

> This file provides workspace-specific context for Claude Code sessions in this project.

## 项目当前状态（维护期，2026-08-29 起）

本项目由多 Agent 团队在 2026-04-02 ~ 04-10 完成初始构建（M0–M3），此后进入**差距修复维护模式**。构建期的多 Agent 角色体系（PM/Tech Lead/6 个分层开发 Agent/QA）已解散，相关文档均为历史归档，**不再对当前工作构成流程约束**。

当前工作方式：**一个人类负责人 + 一个主会话**，以 gap 分析文档驱动，走 feat 分支 + PR。

- 流程复盘（为何如此，含全部证据）：[`docs/project/discussions/2026-08-29-process-retrospective.md`](docs/project/discussions/2026-08-29-process-retrospective.md)
- 构建期历史归档入口：[`docs/project/agents/`](docs/project/agents/)、[`docs/project/harness/`](docs/project/harness/)

### 已知欠账（改代码前先知道）

| 欠账 | 数量 | 红线 |
|------|------|------|
| `TODO(dep)` 占位（非测试代码） | 56 | 只降不升，CI 强制（`make debt-check`） |
| 用户可见 `not yet implemented` 桩 | 42 | 只降不升，CI 强制 |
| 相对原版 TS 的 LOC 覆盖 | ≈5.3% | 行为对比测试见 `test/parity/` |

关闭欠账后运行 `scripts/debt-check.sh --update` 刷新基线并随 PR 提交。

## Environment Setup

```bash
# Required
export ANTHROPIC_API_KEY=sk-ant-...  # or use OAuth: claude /config

# Build
make build    # output: bin/claude
make all      # full check: vet + test + build
make docs     # regenerate API context docs
make debt-check  # 欠账红线（TODO(dep) / not-yet-implemented）
```

## Quick Reference

### Auto-Generated Context (开发前先看)

修改某个包之前，先 Read 该包目录下的 CONTEXT.md：
- **索引**: `docs/generated/INDEX.md`
- **包详情**: `<package>/CONTEXT.md`（如 `internal/coordinator/CONTEXT.md`）

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
- [`docs/project/discussions/`](docs/project/discussions/) — 讨论与复盘（维护期流程的出处）
