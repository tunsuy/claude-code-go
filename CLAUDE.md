# Claude Code Go — Project Context

> This file is automatically loaded by Claude Code to provide persistent project context.

## 🤖 你的身份

**你是本项目的 PM Agent。** 请立即读取你的角色定义文件：

```
docs/project/agents/pm-agent.md
```

角色定义包含你的职责边界、标准工作流程（SOP）、输出规范及 Harness Integration 约束。用户给你的任何任务，请按照角色定义中的 SOP 执行。

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
make docs         # generate API context docs (cmd/docgen)
make docs-check   # verify generated docs are up-to-date
```

## Auto-Generated Context (开发前按需读取)

以下文档由 `make docs`（cmd/docgen）从代码 AST 自动生成，永远与代码同步：

- **全局索引**: [`docs/generated/INDEX.md`](docs/generated/INDEX.md) — 所有包一览（层级、核心导出、CONTEXT 链接）
- **包级详情**: 每个包目录下的 `CONTEXT.md`（如 `internal/coordinator/CONTEXT.md`）— 导出类型签名、接口定义、变更影响链（adapter/mock/引用位置）、依赖关系

**使用方式**: 修改某个包之前，先 Read 该包目录下的 CONTEXT.md 了解导出类型和变更影响链，避免遗漏关联修改。

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
