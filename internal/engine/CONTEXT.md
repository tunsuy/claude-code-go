---
package: engine
import_path: internal/engine
layer: core
generated_at: 2026-04-29T02:31:52Z
source_files: [budget.go, cache_params.go, engine.go, forked_agent.go, msg.go, orchestration.go, query.go, stop_hooks.go]
---

# internal/engine

> Layer: **Core** · Files: 8 · Interfaces: 1 · Structs: 14 · Functions: 9

## Interfaces

### QueryEngine (5 methods)
> QueryEngine is the top-level interface exposed to the TUI/SDK layer.

```go
type QueryEngine interface {
    Query(ctx context.Context, params QueryParams) (<-chan Msg, error)
    Interrupt(ctx context.Context)
    GetMessages() []types.Message
    SetMessages(messages []types.Message)
    SetModel(model string)
}
```

## Structs

- **BudgetTracker** — 4 fields: ContinuationCount, LastDeltaTokens, LastGlobalTurnTokens, StartedAt
- **CacheSafeParams** — 3 fields: SystemPrompt, ContextMessages, ToolUseContext
- **Config** — 7 fields: Client, Registry, Model, MaxTokens, PermissionChecker, StopHooks, MsgQueue
- **ForkedAgentConfig** — 6 fields: PromptMessages, CacheSafeParams, QuerySource, MaxTurns, AllowedTools, OnMessage
- **Msg** — 21 fields: Type, TextDelta, ToolUseID, ToolName, InputDelta, ToolInput, ToolResult, AssistantMsg, ...
- **PermissionRequestMsg** — 6 fields: RequestID, ToolUseID, ToolName, Message, Input, RespFn
- **QueryParams** — 14 fields: Messages, SystemPrompt, UserContext, SystemContext, ToolUseContext, QuerySource, MaxOutputTokensOverride, MaxTurns, ...
- **StopHookContext** — 6 fields: Messages, ToolUseContext, QuerySource, IsBareMode, Engine, CacheParams
- **StopHookRegistry** — 2 fields
- **SystemPrompt** — 1 fields: Parts
- **SystemPromptPart** — 2 fields: Text, CacheControl
- **TaskBudget** — 1 fields: Total
- **TokenBudgetDecision** — 8 fields: Action, NudgeMessage, ContinuationCount, Pct, TurnTokens, Budget, DiminishingReturns, DurationMs
- **ToolResultMsg** — 3 fields: ToolUseID, Content, IsError

## Function Types

- `StopHookFn` — `func(ctx context.Context, hookCtx *StopHookContext)`

## Functions

- `CreateCacheSafeParams(params QueryParams, messages []types.Message) *CacheSafeParams`
- `LoadCacheSafeParams() (*CacheSafeParams, error)`
- `LoadCacheSafeParamsFrom(filePath string) (*CacheSafeParams, error)`
- `New(cfg Config) QueryEngine`
- `NewBudgetTracker() *BudgetTracker`
- `NewStopHookRegistry() *StopHookRegistry`
- `RunForkedAgent(ctx context.Context, eng QueryEngine, cfg ForkedAgentConfig) ([]types.Message, error)`
- `SaveCacheSafeParams(params *CacheSafeParams) error`
- `SaveCacheSafeParamsTo(params *CacheSafeParams, filePath string) error`

## Constants

- `CompletionThreshold`
- `DefaultMaxToolUseConcurrency`
- `DiminishingThreshold`
- `MaxContinuations`
- `MsgTypeAssistantMessage`
- `MsgTypeCompactEnd`
- `MsgTypeCompactStart`
- `MsgTypeError`
- `MsgTypePermissionRequest`
- `MsgTypePermissionResponse`
- `MsgTypeProgress`
- `MsgTypeRequestStart`
- `MsgTypeStreamRequestStart`
- `MsgTypeStreamText`
- `MsgTypeSystemMessage`
- `MsgTypeThinkingDelta`
- `MsgTypeTombstone`
- `MsgTypeToolResult`
- `MsgTypeToolUseComplete`
- `MsgTypeToolUseInputDelta`
- `MsgTypeToolUseStart`
- `MsgTypeTurnComplete`
- `MsgTypeUserMessage`

## Change Impact

**Exported type references (files that use types from this package):**
- `Config` → `internal/bootstrap/wire.go`
- `ForkedAgentConfig` → `internal/memdir/extract.go`
- `Msg` → `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/tui/cmds.go`, `internal/tui/model.go`, `internal/tui/tui_test.go` (test)
- `QueryEngine` → `internal/bootstrap/wire.go`, `internal/tui/cmds.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `QueryParams` → `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/tui/cmds.go`, `internal/tui/tui_test.go` (test)
- `StopHookContext` → `internal/bootstrap/wire.go`, `internal/memdir/extract.go`, `internal/memdir/extract_test.go` (test)
- `SystemPrompt` → `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/tui/cmds.go`
- `SystemPromptPart` → `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/tui/cmds.go`
- `ToolResultMsg` → `internal/tui/tui_test.go` (test)

## Dependencies

**Imports:** `internal/api`, `internal/compact`, `internal/msgqueue`, `internal/permissions`, `internal/tools`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/memdir`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
