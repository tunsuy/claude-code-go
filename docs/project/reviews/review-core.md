# Core Layer Design Review

> **Reviewer**: Tech Lead
> **Document Reviewed**: `docs/project/design/core.md`
> **Review Date**: 2026-04-02
> **Responsible Agent**: Agent-Core

---

## 1. Overall Assessment

**APPROVED_WITH_CHANGES**

The core layer design is well-structured, clearly mapped to the TypeScript source, and demonstrates deep understanding of the Go concurrency model. The `Tool` interface is the most critical deliverable in this document and is largely sound. Several issues — one BLOCKER, two MAJOR, and multiple MINOR — must be resolved before Agent-Core begins implementation. None of the issues invalidate the overall architecture; all are fixable with targeted edits.

---

## 2. Tool Interface Contract (CRITICAL)

This section constitutes the **formal, binding interface contract** for the `internal/tool` package. Agent-Tools implementations MUST satisfy this interface exactly. Any deviation requires a formal change request through the Tech Lead before writing tool code.

### 2.1 Confirmed Go Interface Definition

```go
// Package tool defines the tool interface contract for tool-layer implementations
// and engine-layer consumers.
// This package MUST NOT import engine, permissions, tui, or any other core package
// to prevent import cycles.
package tool

import (
    "context"
    "encoding/json"
)

// Input represents the raw JSON-encoded parameters of a tool call.
// Tool implementations must unmarshal json.RawMessage into a concrete type.
type Input = json.RawMessage

// InputSchema describes the JSON Schema for a tool's input (always "object" type).
type InputSchema struct {
    Type       string                     `json:"type"`                 // always "object"
    Properties map[string]json.RawMessage `json:"properties,omitempty"`
    Required   []string                   `json:"required,omitempty"`
    Extra      map[string]json.RawMessage `json:"-"`
}

// Tool is the interface every tool implementation must satisfy.
// Methods are grouped into five responsibilities:
// identity/metadata, concurrency/safety, permissions, execution, serialization.
type Tool interface {

    // ── Identity & Metadata ────────────────────────────────────

    // Name returns the canonical tool name (unique; used by the model to invoke).
    Name() string

    // Aliases returns backward-compatible alternative names (may return nil).
    Aliases() []string

    // Description returns the tool description injected into the prompt.
    // input is the current call's parameters (may be nil); used for dynamic descriptions.
    Description(input Input, permCtx PermissionContext) string

    // InputSchema returns the JSON Schema describing the tool's input parameters.
    InputSchema() InputSchema

    // Prompt returns text injected into the system prompt for this tool.
    // Most tools return an empty string; complex tools (Bash, Edit) inject detailed usage specs.
    Prompt(ctx context.Context, permCtx PermissionContext) (string, error)

    // MaxResultSizeChars returns the maximum character count for output written to
    // conversation history. Output exceeding this limit is persisted to disk and the
    // model receives a preview + path. Return -1 for no limit.
    MaxResultSizeChars() int

    // SearchHint returns keyword hints for ToolSearch (3–10 words). Return empty string if unused.
    SearchHint() string

    // ── Concurrency & Safety ───────────────────────────────────

    // IsConcurrencySafe reports whether this call may execute concurrently with other tools.
    // true = read-only (safe to parallelize); false = write/side-effect (must serialize).
    IsConcurrencySafe(input Input) bool

    // IsReadOnly reports whether this call makes no changes to external state.
    IsReadOnly(input Input) bool

    // IsDestructive reports whether this call is irreversible (delete, overwrite, send, etc.).
    // Implementations of destructive operations MUST return true; default is false.
    IsDestructive(input Input) bool

    // IsEnabled reports whether this tool is currently available (feature flags, runtime conditions).
    IsEnabled() bool

    // InterruptBehavior returns the tool's behavior when the user sends a new message mid-execution.
    // "cancel" – abort and discard result; "block" – continue executing, queue the new message.
    InterruptBehavior() string

    // ── Permissions ────────────────────────────────────────────

    // ValidateInput validates input correctness before permission checks are applied.
    // On failure the engine returns an error to the model without showing a permission prompt.
    // Return ValidationResult{OK: true} if no validation is required.
    ValidateInput(input Input, ctx *UseContext) (ValidationResult, error)

    // CheckPermissions performs tool-level permission checks for the given input.
    // Called only after ValidateInput passes.
    // Generic permission logic lives in the permissions package; this method handles
    // tool-specific logic only.
    CheckPermissions(input Input, ctx *UseContext) (PermissionResult, error)

    // PreparePermissionMatcher builds a pattern-matching closure used by hook `if` conditions
    // (e.g., "git *"). Return nil if this tool does not participate in pattern matching.
    PreparePermissionMatcher(input Input) (func(pattern string) bool, error)

    // ── Execution ──────────────────────────────────────────────

    // Call executes the tool and returns its result.
    // onProgress may be nil (non-interactive scenarios); implementations must tolerate a nil callback.
    Call(
        input      Input,
        ctx        *UseContext,
        onProgress OnProgressFn,
    ) (*Result, error)

    // ── Serialization ──────────────────────────────────────────

    // MapResultToToolResultBlock serializes tool output into the Anthropic API
    // tool_result content block ([]ContentBlock JSON).
    MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error)

    // ToAutoClassifierInput returns a simplified representation for the safety classifier.
    // Return empty string to skip classification for this tool.
    ToAutoClassifierInput(input Input) string

    // UserFacingName returns the display name for use in UI (may include a parameter summary).
    UserFacingName(input Input) string

    // GetPath returns the file path this tool operates on (file-operation tools only).
    // Return empty string if not applicable.
    GetPath(input Input) string
}
```

### 2.2 Interface Method Quick-Reference

| Method | Required | Notes |
|---|---|---|
| `Name()` | ✅ | Must be unique across all registered tools |
| `Aliases()` | ✅ | Return `nil` if no aliases |
| `Description()` | ✅ | May vary dynamically based on `input` |
| `InputSchema()` | ✅ | Must be a valid JSON Schema `"object"` |
| `Prompt()` | ✅ | Return `("", nil)` if unused |
| `MaxResultSizeChars()` | ✅ | Return `-1` for unlimited |
| `SearchHint()` | ✅ | Return `""` if unused |
| `IsConcurrencySafe()` | ✅ | Drives batch partitioning in the engine |
| `IsReadOnly()` | ✅ | Used by the permissions layer |
| `IsDestructive()` | ✅ | Return `false` for non-destructive tools |
| `IsEnabled()` | ✅ | Feature-flag gate |
| `InterruptBehavior()` | ✅ | Must return `"cancel"` or `"block"` |
| `ValidateInput()` | ✅ | Return `ValidationResult{OK: true}` if N/A |
| `CheckPermissions()` | ✅ | Return `PermissionResult{Behavior: PermissionPassthrough}` to delegate |
| `PreparePermissionMatcher()` | ✅ | Return `nil` if N/A |
| `Call()` | ✅ | Core execution method |
| `MapResultToToolResultBlock()` | ✅ | Must produce valid Anthropic API content blocks |
| `ToAutoClassifierInput()` | ✅ | Return `""` to skip classifier |
| `UserFacingName()` | ✅ | May equal `Name()` for simple tools |
| `GetPath()` | ✅ | Return `""` if not a file-operation tool |

### 2.3 Optional Sub-Interfaces (Engine Uses Type Assertion)

Tools that participate in optional engine features SHOULD implement these sub-interfaces. The engine uses type assertions — tool authors do not need to implement these unless the capability is relevant to their tool.

```go
// PathTool is implemented by tools that operate on a specific file path.
// The engine uses this for UI grouping and file-lock tracking.
type PathTool interface {
    Tool
    GetPath(input Input) string
}

// SearchOrReadTool is implemented by tools that perform search or read operations.
// The engine uses this for UI fold/collapse decisions.
type SearchOrReadTool interface {
    Tool
    IsSearchOrRead(input Input) SearchOrReadResult
}

// MCPTool is implemented by tools that wrap an MCP server tool.
type MCPTool interface {
    Tool
    MCPInfo() MCPInfo
}
```

---

## 3. Strengths

**3.1 Clean dependency hierarchy**
The `internal/tool` package is dependency-free (no imports from `engine`, `permissions`, `tui`, or `coordinator`). This is the correct foundation for a large layered system and must be enforced via a linter rule or Go module boundary.

**3.2 Explicit concurrency model**
The `IsConcurrencySafe(input Input) bool` predicate is input-aware, which is the right design. A `WriteFile` tool can return `false` regardless, while a hypothetical future tool could vary per call. The `partitionToolCalls` batch algorithm is clean and mirrors the TypeScript original faithfully.

**3.3 Bidirectional permission channel**
Using `chan<- PermissionResponse` embedded inside `PermissionAskMsg` is idiomatic Go for request-reply over channels. The 60-second timeout auto-deny preventing deadlock in non-interactive scenarios is a good defensive choice.

**3.4 Three-tier compact strategy**
Auto/Micro/Snip serve distinct purposes (LLM-based summarization vs. rule-based truncation vs. placeholder snipping) and are cleanly separated behind a `Compressor` interface. This makes testing and future extension straightforward.

**3.5 Hooks are process-isolated**
Hooks run as external subprocesses communicating over stdin/stdout JSON. This is the correct security boundary — a misbehaving hook cannot corrupt engine state directly.

**3.6 `PermissionPassthrough` sentinel**
Having a four-value `PermissionBehavior` (allow / deny / ask / passthrough) is cleaner than using a Go `nil` return to indicate "I have no opinion". Tools that do not participate in permission decisions return `passthrough` and the engine falls through to mode-based defaults.

**3.7 `Result.ContextModifier` pattern**
Allowing write-serialized tools to mutate `UseContext` via a callback rather than directly (which would be a data race) is a sound design for context propagation in a concurrent system.

---

## 4. Issues

### BLOCKER

**B-1: `GetPath` is simultaneously a required interface method AND a sub-interface method**

In §1.2 (the core `Tool` interface), `GetPath(input Input) string` is defined as a required method of `Tool`. In §7.1, the same method is declared again as part of the optional `PathTool` sub-interface. This is a contradiction.

- If `GetPath` is required on `Tool`, then every tool implements it (it returns `""` for non-path tools), and `PathTool` is redundant and should be removed.
- If `GetPath` is optional, it must be removed from the `Tool` interface and only exist on `PathTool`.

**Required resolution before implementation**: Remove `GetPath` from the main `Tool` interface. Keep it exclusively on the `PathTool` sub-interface (type-asserted by the engine). Non-path tools simply do not implement `PathTool`.

This is a BLOCKER because Agent-Tools authors will write tools against this interface, and a method that is simultaneously required and optional will cause immediate confusion and incorrect implementations.

---

### MAJOR

**M-1: `UseContext` contains both a `context.Context` (field `Ctx`) and is passed by pointer — cancel propagation is ambiguous**

`UseContext.Ctx context.Context` embeds a Go context inside a struct pointer. The engine constructs `UseContext` per call, but the struct is passed as `*UseContext` and tools may retain it. The standard Go convention is to pass `context.Context` as the first argument to functions that need cancellation, not embedded in structs. The current design violates this convention and creates two risks:

1. A tool that spawns goroutines might capture `uctx.Ctx` and not respect context cancellation correctly because `uctx` is mutated by `ContextModifier`.
2. The `Call` signature already has no leading `context.Context` argument; cancellation is entirely via `UseContext.Ctx`.

**Required resolution**: Either (a) add `ctx context.Context` as the first argument to `Call` and remove `Ctx` from `UseContext`, or (b) document in the design that `UseContext.Ctx` IS the canonical cancellation context for tool execution and tools must not hold `*UseContext` beyond the `Call` invocation. Option (a) is strongly preferred as it aligns with Go idiom and makes tool implementations easier to review.

**M-2: `Message` type in `internal/tool` is a stub that references `types/message` package, but the relationship is undefined**

The comment on `Message` in §1.2 says "完整类型在 types/message 包" (the complete type is in the `types/message` package), yet the struct defined here has `Content json.RawMessage`. The design does not define the `types/message` package, does not show how the two `Message` types relate, and does not specify if one wraps or replaces the other.

This is MAJOR because:
- `QueryEngine.History()` returns `[]tool.Message`
- `UseContext.Messages` is `[]tool.Message`
- `Result.NewMessages` is `[]tool.Message`
- All three are on the critical path for the agentic loop

**Required resolution**: Either (a) define `tool.Message` as the canonical type used everywhere and remove the mention of `types/message`, or (b) define the `types/message` package and show how it maps to `tool.Message`. A follow-up design document is acceptable, but the relationship must be resolved before Agent-Tools begins implementation because tools append to `Result.NewMessages`.

---

### MINOR

**m-1: `InterruptBehavior() string` should be a typed constant, not a raw string**

The method contract specifies `"cancel" | "block"` but the return type is `string`. A typo (`"cancal"`) silently produces undefined behavior in the engine.

**Recommendation**: Define `type InterruptBehavior string` with `const InterruptBehaviorCancel` and `InterruptBehaviorBlock` in `tool.go`, and change the method signature to `InterruptBehavior() InterruptBehavior`.

**m-2: `BudgetTracker` in §2.7 has unexported fields in a design document — should all be unexported in implementation**

The design document shows `BudgetTracker` with all exported fields. In practice, this struct should encapsulate mutation behind methods (e.g., `AddTokens(n int)`, `ShouldCompact() bool`) to prevent the engine and compactor from racing on field access.

**Recommendation**: In the implementation, make all `BudgetTracker` fields unexported and provide thread-safe accessor/mutator methods protected by a `sync.Mutex`.

**m-3: `AggregatedHookResult.PermissionBehavior` is a raw `string`, inconsistent with `tool.PermissionBehavior`**

`hooks.AggregatedHookResult.PermissionBehavior string` uses a plain string while the rest of the system uses `tool.PermissionBehavior`. The `hooks` package already imports `tool` (it references `tool.Message`), so there is no import cycle reason for this inconsistency.

**Recommendation**: Change the field type to `tool.PermissionBehavior`.

**m-4: `EngineMsg` uses a private marker method — document the deliberate extensibility boundary**

`EngineMsg` seals the type with `engineMsg()`. This is correct, but the design should explicitly state that adding new `EngineMsg` variants is a breaking change that requires updating TUI switch statements. A comment in `msg.go` and in the design doc is sufficient.

**m-5: `Coordinator.SendMessage` to `agentID = "swarm"` (broadcast) is not reflected in `AgentStatus` states**

`AgentStatus.State` covers `"running" | "completed" | "aborted" | "error"` but there is no state for a swarm broadcast in progress or partial failure (some workers succeeded, some failed). Clarify whether `WaitAll` returns the first error or an aggregated multi-error.

**Recommendation**: Define an error type `SwarmError` that wraps per-agent errors, and specify `WaitAll` semantics explicitly.

**m-6: Micro-compact truncation threshold is named `IMAGE_MAX_TOKEN_SIZE` but applies to non-image tool results**

The name `IMAGE_MAX_TOKEN_SIZE=2000` suggests it controls images only. The design states it also applies to Bash/Shell/FileRead/Grep/Glob outputs. This is a naming holdover from the TypeScript codebase.

**Recommendation**: Rename to `MICRO_COMPACT_TOOL_RESULT_MAX_TOKENS` in the Go implementation to avoid confusion.

---

## 5. Required Changes

The following changes are **mandatory before Agent-Core begins implementation**. Agent-Tools implementation is unblocked on everything except B-1 and M-2, which are pre-conditions for correct tool authoring.

| Priority | Change | Owner | Blocks |
|---|---|---|---|
| **BLOCKER B-1** | Remove `GetPath` from the `Tool` interface; retain only on `PathTool` sub-interface | Agent-Core | Agent-Tools |
| **MAJOR M-1** | Add `ctx context.Context` as first parameter to `Call()`, remove `Ctx` from `UseContext` (or document the policy explicitly if option b is chosen) | Agent-Core | Agent-Tools, Agent-Engine |
| **MAJOR M-2** | Define `tool.Message` as canonical or publish the `types/message` design | Agent-Core | Agent-Tools, Agent-Engine |
| MINOR m-1 | Define `InterruptBehavior` as a typed string constant | Agent-Core | Agent-Tools |
| MINOR m-3 | Change `AggregatedHookResult.PermissionBehavior` to `tool.PermissionBehavior` | Agent-Core | Agent-Hooks |

The remaining MINOR items (m-2, m-4, m-5, m-6) may be addressed during implementation as code-level decisions.

---

## 6. Implementation Notes for Agent-Core

**6.1 Package initialization order**

Implement packages in this order to avoid circular dependency surprises:
1. `internal/tool` (zero dependencies)
2. `internal/permissions` (depends on `tool`)
3. `internal/hooks` (depends on `tool`)
4. `internal/compact` (depends on `tool`)
5. `internal/engine` (depends on all of the above)
6. `internal/coordinator` (depends on `engine`, `tool`)

Enforce via `go vet` + `golang.org/x/tools/cmd/deadcode` or a simple CI step that checks `go list -deps`.

**6.2 Tool registry thread safety**

The design mentions `registry.go` but does not specify its concurrency model. The registry is read-only after startup, so a `sync.RWMutex` or a plain `map` behind `sync.Once` initialization is sufficient. Do not use a global mutable map.

**6.3 `partitionToolCalls` is deterministic — write a table-driven unit test immediately**

The batch partitioning algorithm is subtle and correct behavior is critical for tool ordering guarantees. Implement the unit test from the design document example (`[Read, Read, Write, Read, Write]` → four batches) as the first test, before any other engine tests.

**6.4 Permission ask timeout must use `context.WithTimeout`, not `time.Sleep`**

The 60-second auto-deny must be implemented as:
```go
ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
defer cancel()
select {
case resp := <-responseCh:
    return resp, nil
case <-ctx.Done():
    return denyResponse, nil
}
```
A `time.Sleep`-based approach will not respect parent context cancellation (e.g., if the user presses Ctrl-C).

**6.5 Channel buffer size 256 for `outCh` is a reasonable starting point but should be configurable**

Document `CLAUDE_CODE_ENGINE_MSG_BUF_SIZE` as an environment variable override. Under heavy parallel tool workloads (10 concurrent tools each emitting progress events), 256 may be insufficient and will cause the engine goroutine to block.

**6.6 Auto-compact consecutive failure counter must be reset on success**

The design specifies stopping after 3 consecutive failures (`MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES`). Confirm that `ConsecutiveFailures` is reset to 0 on any successful compaction, not just at session start.

**6.7 `Result.ContextModifier` must only be invoked for non-concurrent-safe tools**

The design states "并发安全工具返回 nil" (concurrency-safe tools return nil). The engine must assert this invariant: if a tool returns `IsConcurrencySafe(input) == true` AND `Result.ContextModifier != nil`, log a warning and discard the modifier rather than applying it (to prevent data races).

**6.8 MCP tools should implement the `MCPTool` sub-interface, not a separate top-level type**

Do not create a `MCPTool` struct that bypasses `tool.Tool`. MCP tools must fully implement `tool.Tool` so they pass through the same engine pipeline (permissions, hooks, concurrency batching) as native tools. The `MCPTool` sub-interface is used only for metadata retrieval and UI display.

---

*Review completed. Agent-Core must address B-1, M-1, and M-2 and update `core.md` before implementation begins. Agent-Tools implementation is unblocked on all tools that do not use `GetPath` — the `PathTool` sub-interface clarification (B-1) must be resolved before any file-operation tool is written.*
