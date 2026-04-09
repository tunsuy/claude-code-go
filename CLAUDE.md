# Claude Code Go — Project Context

> This file is automatically loaded by Claude Code to provide persistent project context.

## ⚠️ MANDATORY: Read Project Documentation First

**本项目采用 AI 编程工具的多 Agent 并行开发模式构建。** 项目分为六层架构，每层由一个专职 AI Agent（如 Agent-Infra、Agent-Core 等）在独立的 Git Worktree 分支上并行实现，通过 PM Agent 协调、Tech Lead Agent 评审、QA Agent 验收。详细的开发模式说明见 `docs/project/team-agent-design.md`。

**在对本项目进行任何修改之前，你必须先阅读以下文档：**

1. **架构设计**：`docs/project/architecture.md` — 六层架构定义、模块边界、依赖规则
2. **多 Agent 开发模式**：`docs/project/team-agent-design.md` — 项目的 AI Agent 团队分工、开发流程、治理机制
3. **项目状态**：`docs/project/status.md` — 当前任务状态、已知问题
4. **开发计划**：`docs/project/plan.md` — 阶段划分、任务依赖关系
5. **对应层设计文档**：`docs/project/design/<layer>.md` — 你要修改的层的详细设计

**关键约束：**
- 本项目是**使用 AI 编程工具的多 Agent 模式开发出来的**，每一层由一个独立的 AI Agent 负责实现
- 修改任何代码前，必须先确认该代码属于哪一层，阅读对应的设计文档和 Agent 角色定义（`docs/project/agents/`）
- 严格遵守层间依赖规则：下层模块**禁止**依赖上层模块
- 代码变更若导致接口签名变化，**必须**同步更新对应的设计文档（见 `docs/project/doc-sync-policy.md`）
- 所有代码评审报告在 `docs/project/reviews/`，QA 报告在 `docs/project/qa/`

## Project Overview

Claude Code Go is a **full Go rewrite** of Claude Code (originally TypeScript/Bun). It is an agentic AI coding assistant that runs in the terminal. The entire codebase (~7,000 lines of production code + tests) was built using **AI coding tool's multi-agent parallel development mode** — each architectural layer was implemented by a dedicated AI Agent (e.g., Agent-Infra, Agent-Core) on isolated Git Worktree branches, coordinated by PM Agent, reviewed by Tech Lead Agent, and tested by QA Agent. **Zero human-written production code**.

- **Module**: `github.com/anthropics/claude-code-go`
- **Go version**: 1.21+
- **License**: MIT

## Architecture (Six-Layer)

```
CLI (cmd/claude)         → Cobra entry point, bootstrap
TUI (internal/tui)       → Bubble Tea MVU interface
Tools (internal/tools)   → File, shell, search, MCP tools
Engine (internal/engine) → LLM query loop, tool dispatch, coordinator
Services (internal/api, oauth, mcp, compact) → API client, MCP, OAuth
Infra (pkg/types, internal/config, state, session, hooks) → Types, config, state
```

**Dependency rule**: Lower layers MUST NOT import from higher layers. `pkg/types` is zero-dependency — all layers may depend on it.

## Key Dependencies

| Library | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI command framework |
| `github.com/charmbracelet/bubbletea` | TUI framework (Elm-style MVU) |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/charmbracelet/glamour` | Markdown terminal rendering |
| `github.com/yuin/goldmark` | Markdown parsing |

## Build & Test

```bash
make build        # → bin/claude
make test         # go test -race ./...
make test-cover   # generates coverage.html
make vet          # go vet ./...
make lint         # golangci-lint (requires install)
make all          # vet + test + build
```

## Coding Conventions

### Formatting & Style
- **All code** must be formatted with `gofmt` / `goimports`
- Line length: ≤120 columns (soft limit; exceptions for function signatures, URLs, struct tags)
- File length: ≤800 lines; function length: ≤80 lines
- Test file: ≤1600 lines; test function: ≤160 lines
- Nesting depth: ≤4 levels

### Naming
- **Packages**: lowercase, short, meaningful; match directory name; no `util`/`common`/`misc`
- **Files**: lowercase with underscores (`file_read.go`)
- **Structs/Interfaces**: CamelCase nouns; single-method interfaces end in `-er`
- **Variables**: camelCase; acronyms preserve casing (`apiClient`, `APIClient`, `userID`)
- **Constants**: CamelCase; enums need a type declaration first

### Imports (3-group ordering)
```go
import (
    "stdlib/packages"          // Group 1: Standard library

    "external/third-party/pkg" // Group 2: Third-party packages

    "github.com/anthropics/claude-code-go/internal/..." // Group 3: Internal packages
)
```

### Error Handling
- **Always** handle errors — never silently swallow them
- Error as last return value: `func Foo() (T, error)`
- Wrap with context: `fmt.Errorf("module context: %w", err)`
- Use `errors.New("...")` for simple errors
- Return errors immediately (early return pattern)
- **No panic** for general errors — use `error` + multi-return
- `panic` only for init-time invariants or `MustXXX` patterns

### Concurrency
- LLM main loop runs in a dedicated goroutine, communicates via `chan Msg`
- Read-only tools may run concurrently; write tools run serially
- Shared state protected by `sync.RWMutex`
- Always propagate `context.Context` for cancellation
- Prefer channels over shared memory; document guarded fields

### Comments (Required for Exports)
- All exported names must have GoDoc comments: `// TypeName description...`
- Package comment: `// Package name description...` (in one file per package)
- Delete dead/commented-out code before review

## Project Layout

```
cmd/claude/          CLI entry point (main.go)
internal/
  api/               Anthropic API client (SSE streaming, retry, usage)
  bootstrap/         App init, dependency injection, Cobra root command
  commands/          Slash command registry & built-in handlers
  compact/           Context compaction (auto/micro/snip strategies)
  config/            Three-level config (global/project/local)
  coordinator/       Multi-agent coordination (spawn, message routing)
  engine/            Core query engine (LLM loop, tool orchestration)
  hooks/             Pre/post tool hooks
  mcp/               MCP protocol client (connection pool, JSON-RPC)
  memdir/            CLAUDE.md discovery & loading
  oauth/             OAuth2 auth flow (macOS Keychain support)
  permissions/       Tool permission decision pipeline (9-layer chain)
  plugin/            Plugin system
  session/           Session persistence
  state/             Generic state store + AppState
  tools/             Tool interface, registry, BaseTool
    agent/           Sub-agent & SendMessage tools
    fileops/         FileRead, FileWrite, FileEdit, Glob, Grep, NotebookEdit
    interact/        AskUserQuestion, Worktree tools
    mcp/             MCP tool adapter
    misc/            Miscellaneous tools
    shell/           Bash execution tool
    tasks/           Task CRUD tools
    web/             WebFetch, WebSearch tools
  tui/               Bubble Tea UI components
pkg/
  types/             Zero-dependency shared types (Message, ContentBlock, etc.)
  utils/             Utility packages (env, fs, ids, jsonutil, permission matcher)
  testutil/          Test helpers
docs/project/        Architecture docs, design docs, QA reports
test/integration/    Integration tests
```

## Design Patterns

### Interface-Driven + Dependency Injection
Every core component defines an interface (`QueryEngine`, `Client`, `Checker`, `Coordinator`, `Tool`). Concrete implementations are injected via constructors. `AppContainer` is the top-level DI wiring point.

### Tool System: Interface + BaseTool Embedding
```go
type Tool interface {
    Name() string
    Description(Input, PermissionContext) string
    InputSchema() InputSchema
    Call(Input, *UseContext, OnProgressFn) (*Result, error)
    // ... ~20 methods total
}

type BaseTool struct{} // default implementations for optional methods
type myTool struct{ tools.BaseTool } // only override what's needed
```

### Permission Pipeline (9 Layers)
```
bypass → deny rules → validate → hooks → allow rules → ask rules →
mode default → tool-specific → default ask/allow
```

### Context Compaction (3 Strategies)
1. **Snip** — local truncation of old tool results (no LLM call)
2. **Micro** — compress oversized single messages (no LLM call)
3. **Auto** — LLM-driven history summarization (near context limit)

### Phased Bootstrap
```
Phase 0: Fast path (--version)
Phase 1: Config loading
Phase 2: Runtime safety
Phase 3: OAuth pre-warm
Phase 4: API client
Phase 5: Tool registration
Phase 6: Engine + AppState
```

## Testing Guidelines

- Test files: `*_test.go` alongside source
- Coverage target: ≥60% for new packages
- Use **table-driven tests** with parallel execution
- TUI testing: test `Update()` and view functions; use word-based assertions (ANSI codes may intervene)
- Mock external interfaces; separate fast unit tests from integration tests

## Security Notes

- **SQL injection**: Always use parameterized queries (though no SQL in this project)
- **Input validation**: Validate all tool inputs before execution
- **Credentials**: Never hardcode API keys; use env vars or OAuth
- **Sandbox**: Bash tool has sandbox awareness (Phase 2 implementation)

## Common Tasks

### Adding a New Built-in Tool
1. Create a new file in `internal/tools/<category>/`
2. Embed `tools.BaseTool` and implement required `Tool` interface methods
3. Register in `internal/bootstrap/tools.go` via `RegisterBuiltinTools()`
4. Add tests in the same package

### Adding a Slash Command
1. Add handler in `internal/commands/builtins.go`
2. Register via `Registry.Register()` in the init flow

### Modifying the TUI
- All state lives in `AppModel` (value type, Elm architecture)
- Changes flow through `Update() → tea.Cmd → tea.Msg`
- Side effects via `tea.Cmd`; never mutate state directly
