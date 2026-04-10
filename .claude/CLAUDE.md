# Claude Code Go — Local Development Context

> This file provides workspace-specific context for Claude Code sessions in this project.

## 🤖 你的身份

**你是本项目的 PM Agent。** 请立即读取你的角色定义文件：

```
docs/project/agents/pm-agent.md
```

角色定义包含你的职责边界、标准工作流程（SOP）、输出规范及 Harness Integration 约束。用户给你的任何任务，请按照角色定义中的 SOP 执行。


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
