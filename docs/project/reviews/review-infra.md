# Infrastructure Layer Design Review

> **Reviewer**: Tech Lead
> **Date**: 2026-04-02
> **Subject**: `docs/project/design/infra.md` (v1.0, Author: Agent-Infra)
> **Verdict**: **APPROVED_WITH_CHANGES**

---

## 1. Overall Assessment

The infrastructure layer design is structurally sound. The TypeScript → Go type mapping is thoughtful, the dependency graph is correct (`pkg/types` as zero-dependency anchor), and the layered responsibility split (types / utils / config / state / session / bootstrap) maps cleanly onto Go conventions. The design is implementable as written — with three blocking defects that **must** be resolved before any code is committed, and several important issues to address during implementation.

---

## 2. Strengths

### 2.1 Architecture

- **`pkg/types` zero-dependency anchor.** Every other package depends on it unidirectionally. This is the correct architectural decision and prevents import cycles by construction.
- **Faithful TS → Go type mapping.** Appendix A and Appendix B provide exhaustive mapping tables that will prevent semantic drift during implementation. Branded strings as custom string types, discriminated unions as struct + `Type` field, and `Map<K,V>` → `map[K]V` are all idiomatic choices.
- **Generic `Store[T any]`.** The Go 1.18+ generic store mirrors the TypeScript `Store<T>` faithfully and provides type-safe reuse for future `TaskStore`, `PluginStore`, etc. without `interface{}` casts.
- **Correct lock-release-then-notify pattern.** `Store[T]` releases the write lock before calling `notifyListeners`, correctly avoiding deadlock when a subscriber calls back into the store.

### 2.2 Persistence & Config

- **`AtomicWriteFile` via `os.CreateTemp` + `os.Rename`.** POSIX-compliant crash-safe writes with correct temp-file cleanup on error.
- **`EntryEnvelope` + `json.RawMessage` lazy decoding.** Idiomatic Go for a 20+ variant Entry discriminated union — avoids a large type-switch at the persistence boundary.
- **JSONL format choice.** Correct for Phase 1: maintains compatibility with the TypeScript session file format and is trivially debuggable.
- **Four-tier config model (Policy > Local > Project > User).** Layered semantics match the TypeScript original; `uniqueAppend` for permission arrays and scalar override for primitive fields are the right merge strategies.

---

## 3. Issues

### 3.1 BLOCKERs — Must be resolved before implementation begins

#### B-1: Policy merge order is inverted — enterprise controls are overridable by users

**Location**: §3.1 priority table, §3.3 `Loader.Load()`, `LayeredSettings.Merged` comment.

§3.1 states that `managed-settings.json` is "policy tier, highest priority, for enterprise control." However the `mergeSettings` call in `Loader.Load()` is:

```go
ls.Merged = mergeSettings(ls.Policy, ls.User, ls.Project, ls.Local)
```

Because `mergeSettings` applies layers left-to-right (each layer overrides the previous), the actual effective priority is:

**Policy (lowest) → User → Project → Local (highest)**

Enterprise security policy can be overridden by any user-level or local config. The `LayeredSettings.Merged` field comment compounds the problem by writing "Local > Project > User > Policy", placing Policy last (lowest priority) — directly contradicting §3.1.

**Fix**: Apply Policy as a mandatory post-merge override pass:

```go
// Step 1: merge user-controllable layers in ascending priority
base := mergeSettings(ls.User, ls.Project, ls.Local)
// Step 2: Policy locked fields unconditionally override everything
ls.Merged = applyPolicyOverrides(base, ls.Policy)
```

Consult the TypeScript source for `managed-settings.json` handling to determine which fields are "locked" (unconditional override) vs. "policy defaults" (can be overridden). Code and documentation must be consistent.

**Acceptance criteria**: A Local or User config setting cannot override a field locked by `managed-settings.json`; `LayeredSettings.Merged` comment matches actual merge semantics.

---

#### B-2: `SetState` updater shares map reference — inevitable data race in Phase 1

**Location**: §4.3 `AppState`, §4.4 state update contract, §7.3 deep-copy strategy.

The design acknowledges the risk in §4.4 but defers the fix to "Phase 2":

> "Since Go maps and slices are reference types, direct mutation of map/slice in an updater function may cause races."

`Store[T].SetState` acquires the write lock, captures `prev`, runs `updater(prev)`, stores `next`, releases the lock, then calls `notifyListeners(next, prev)`. The `prev` snapshot passed to listeners shares the same underlying `map[string]TaskState` as `next` — because the updater returns a struct value copy, but map fields still point to the same backing store. Any subscriber that reads `prev.Tasks` while another `SetState` runs concurrently triggers a data race.

The example code in §4.4 is the dangerous pattern:

```go
prev.Tasks[taskId] = TaskState{...}  // mutates shared map — DATA RACE
return prev
```

Shipping Phase 1 with this design causes `go test -race` to fail in any concurrent test. "Phase 2 fix" is not acceptable.

**Fix** (choose one; document the choice explicitly in §4.4):

- **Option A**: `SetState` automatically shallow-copies all map fields before passing `prev` to the updater, using `maps.Clone` (Go 1.21+).
- **Option B**: Establish a copy-on-write convention enforced by code review, with a canonical template in §4.4:

```go
store.SetState(func(prev AppState) AppState {
    newTasks := maps.Clone(prev.Tasks) // Go 1.21+, or manual loop for older Go
    newTasks[taskId] = TaskState{AgentId: agentId, Status: "running"}
    prev.Tasks = newTasks
    return prev
})
```

- **Option C**: Replace mutable maps in `AppState` with immutable persistent data structures.

**Acceptance criteria**: `go test -race ./internal/state/...` passes with tests containing concurrent goroutines calling `SetState` and `GetState`.

---

#### B-3: No environment variable override layer — `ANTHROPIC_API_KEY` and similar vars are silently ignored

**Location**: §3.1 config priority table, §3.3 `Loader.Load()`.

The design documents config priority as "env > file > default" in §3.1 but the implementation contains no environment variable layer at all. `Loader.Load()` reads only JSON files; there is no post-merge pass that applies `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL`, `ANTHROPIC_BASE_URL`, or any other environment variable. Any environment configured by CI systems, container orchestrators, or shell profiles will be silently ignored — the stated "env > file" priority is a fiction.

**Fix**: Add an explicit env-var override pass after `mergeSettings`:

```go
func applyEnvOverrides(s *SettingsJson) {
    if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
        s.APIKey = v
    }
    if v := os.Getenv("ANTHROPIC_MODEL"); v == "" {
        v = os.Getenv("CLAUDE_MODEL")
    }
    if v != "" {
        s.Model = v
    }
    if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
        s.BaseURL = v
    }
    // ... remaining mappings
}
```

Document the complete list of supported environment variables and their target `SettingsJson` fields in the design doc.

**Acceptance criteria**: Setting `ANTHROPIC_API_KEY` in the environment overrides the value from any config file; the env-var → field mapping table is present in the design doc.

---

### 3.2 MAJOR Issues — Should be resolved before or during implementation

#### M-1: `scanner.Bytes()` buffer reuse corrupts all-but-last entries in `ReadAll`

**Location**: §5.3 `SessionStore.ReadAll`.

```go
env.Raw = json.RawMessage(line)  // line = scanner.Bytes()
```

`bufio.Scanner.Bytes()` returns a slice into the scanner's internal buffer. Each call to `Scan()` **reuses and overwrites** that buffer. After the loop, every `env.Raw` in the returned slice points to the same memory — the last line's content. All earlier entries hold corrupted data.

**Fix**:

```go
raw := make([]byte, len(line))
copy(raw, line)
env.Raw = json.RawMessage(raw)
```

---

#### M-2: `Tool`, `ToolCall`, and `ToolResult` types are absent from `pkg/types`

**Location**: §1.5 `message.go`, Appendix A type mapping table.

`Message` embeds `ToolUseBlock` and `ToolResultBlock`, but `ToolCall` and `ToolResult` as standalone types do not appear in `pkg/types`. The core loop, tool executor, and session store all need these types. Without them, Agent-Core either defines its own (causing duplication) or uses `interface{}` (losing type safety).

**Fix**: Add to `pkg/types/message.go` (or a new `pkg/types/tools.go`):

```go
type ToolCall struct {
    ID    ToolUseId `json:"id"`
    Name  string    `json:"name"`
    Input any       `json:"input"`
}

type ToolResult struct {
    ToolUseId ToolUseId `json:"tool_use_id"`
    Content   []ContentBlock `json:"content"`
    IsError   bool      `json:"is_error,omitempty"`
}
```

---

#### M-3: `hooks.go` and `plugin.go` are completely empty — downstream layers cannot implement the hooks system

**Location**: §1.2 file structure, §3.2 `SettingsJson.Hooks`.

Both files contain zero type definitions. `SettingsJson.Hooks` is typed `map[string]any`, discarding all type safety. Hooks are a core architectural feature (pre/post tool-use, session start, sampling); without typed definitions Agent-Core cannot implement hook dispatch.

**Fix**: Provide at minimum a core type skeleton in `hooks.go`:

```go
type HookType string

const (
    HookPreToolUse  HookType = "PreToolUse"
    HookPostToolUse HookType = "PostToolUse"
    HookStop        HookType = "Stop"
    // ... mirror TS original
)

type HookDefinition struct {
    Command string `json:"command"`
    Matcher string `json:"matcher,omitempty"`
    Timeout int    `json:"timeout_ms,omitempty"`
}

type HookResult struct {
    Decision string `json:"decision"` // "block" | "approve" | "modify"
    Reason   string `json:"reason,omitempty"`
}
```

Change `SettingsJson.Hooks` from `map[string]any` to `map[HookType][]HookDefinition`. Provide `LoadedPlugin` and `PluginConfig` skeletons in `plugin.go`.

---

#### M-4: Exported `NewAgentId` in `pkg/types` panics unconditionally

**Location**: §1.3 `ids.go`.

```go
func NewAgentId(prefix string) AgentId {
    panic("see internal/bootstrap")
}
```

An exported function in `pkg/types` that panics by design violates the API contract. `pkg/types` is imported by every package in the codebase; any code that reasonably calls `types.NewAgentId` crashes at runtime with no compile-time warning.

**Fix**: Delete `NewAgentId` from `pkg/types/ids.go` entirely. Keep only the pure type-conversion helpers `AsAgentId()` and `AsSessionId()`. ID generation belongs exclusively in `pkg/utils/ids/ids.go` (which already has the correct implementation in §2.3).

---

#### M-5: `Subscribe` listener slice never shrinks — memory leak in mount/unmount cycles

**Location**: §4.2 `Store[T].Subscribe`.

The unsubscribe closure sets `s.listeners[idx]` to nil but never removes the slot. The slice only grows. In TUI component mount/unmount cycles this causes unbounded memory allocation, and `notifyListeners` traversal cost grows without bound.

**Fix**: Replace the nil-slot slice with `map[uint64]Listener[T]` keyed by a monotonically increasing ID:

```go
type Store[T any] struct {
    // ...
    listenerMu sync.Mutex
    listeners  map[uint64]Listener[T]
    nextID     uint64
}

func (s *Store[T]) Subscribe(l Listener[T]) func() {
    s.listenerMu.Lock()
    id := s.nextID
    s.nextID++
    s.listeners[id] = l
    s.listenerMu.Unlock()
    return func() {
        s.listenerMu.Lock()
        delete(s.listeners, id)
        s.listenerMu.Unlock()
    }
}
```

---

#### M-6: `applyLayer` is incomplete — 15+ `SettingsJson` fields are silently discarded

**Location**: §3.3 `applyLayer`.

`applyLayer` handles only ~7 fields (`Model`, `APIKeyHelper`, `DefaultShell`, `RespectGitignore`, `CleanupPeriodDays`, `Env`, `Permissions`). The comment `// ... 其余字段类似处理` ("remaining fields handled similarly") is not acceptable. Fields including `AWSCredentialExport`, `GCPAuthRefresh`, `Hooks`, `Worktree`, `EnableAllProjectMCP`, `EnabledMCPServers`, `DisabledMCPServers`, and 13 others are silently discarded when set in any higher-priority layer.

**Fix**: Enumerate all `SettingsJson` fields with explicit merge semantics (override / unique-append / ignore) and implement all of them. If the boilerplate is excessive, use `go:generate` code generation.

---

#### M-7: MCP/Plugin fields in `AppState` are `[]any` — no type safety

**Location**: §4.3 `AppState`.

```go
MCPClients  []any
MCPTools    []any
MCPServers  []any
```

These fields have no type safety. Agent-Core and Agent-MCP will be forced to type-assert against `any` at every access site, making bugs invisible until runtime.

**Fix**: Define forward-declaration interfaces in `pkg/types` or `internal/state`:

```go
type MCPConnection interface {
    ID() string
    IsConnected() bool
}
```

Use these interfaces (or concrete stub types) as the element type instead of `any`.

---

#### M-8: `ConfigLoader` and `SessionStorer` interfaces are absent — upper layers cannot mock for tests

**Location**: `internal/config/`, `internal/session/`.

`Loader`, `SessionStore`, and `SessionManager` are all concrete structs with no corresponding interfaces. Packages that depend on them (core loop, tool executor) are forced to use real filesystem operations in unit tests — making tests slow, stateful, and platform-dependent.

**Fix**: Define interfaces for each component:

```go
// internal/config
type ConfigLoader interface {
    Load() (*LayeredSettings, error)
}

// internal/session
type SessionStorer interface {
    AppendEntry(entry any) error
    ReadAll() ([]types.EntryEnvelope, error)
    Close() error
}
```

Concrete structs implement these interfaces; all callers outside `internal/` depend on interfaces, not concrete types.

---

### 3.3 MINOR Issues — Address in subsequent iterations

| # | Location | Issue | Recommendation |
|---|----------|-------|----------------|
| N-1 | `pkg/types/logs.go` | `SerializedMessage` embeds `Message` (which has `SessionId`, `Timestamp`) then re-declares identical field names at the outer level — outer fields shadow embedded fields silently | Remove the duplicate outer fields and rely on embedding promotion, or switch to explicit composition: `Msg Message \`json:"message"\`` |
| N-2 | `pkg/utils/json/json.go` | Package name `json` collides with stdlib `encoding/json`; every file importing both must alias one | Rename the package to `jsonutil` |
| N-3 | `pkg/types/permissions.go` | `PermissionUpdate.Type` is a bare `string`; valid values are only in a comment, making typos invisible at compile time | Define `type PermissionUpdateType string` with named constants |
| N-4 | `pkg/types/logs.go` | `EntryType` defines only 5 constants; the TypeScript original has 20+ variants (`debug`, `tool_result`, `thinking`, `bash`, `image`, etc.) | Complete the `EntryType` enum to mirror all TS variants |
| N-5 | `pkg/types/logs.go` | `EntryEnvelope.Raw` has no JSON tag; callers may assume it is populated by `json.Unmarshal` | Add `json:"-"` tag and a comment explaining it must be populated manually by the caller |
| N-6 | `internal/config/loader.go` | `os.IsNotExist(err)` does not unwrap errors (deprecated since Go 1.13) | Replace globally with `errors.Is(err, fs.ErrNotExist)` |
| N-7 | `pkg/types/permissions.go` | `ToolPermissionContext` fields have no JSON tags; they serialize as PascalCase, which is incompatible with the TS source's camelCase | Add JSON tags matching the TS field names for all fields |
| N-8 | `pkg/utils/ids/ids.go` | `_, _ = rand.Read(b)` silently discards error; if entropy source is unavailable, ID collisions occur without any signal | Check the error and panic — unavailable entropy source is a non-recoverable condition |
| N-9 | `internal/session/store.go` | `ReadAll` silently skips corrupt lines with no observability | Log a WARN via `slog.Default()` with the line number and parse error |
| N-10 | `internal/bootstrap/bootstrap.go` | `homeDir()` is referenced but not defined; `$HOME` may be unset in container environments | Define `homeDir()` explicitly: prefer `os.UserHomeDir()`, fall back to `$HOME`, return error if both fail |
| N-11 | `internal/session/store.go` | `AtomicWriteFile` does not call `file.Sync()` before rename; on power loss the temp file may be empty | Call `f.Sync()` before `os.Rename` for durability |
| N-12 | `internal/session/store.go` | No cross-process file locking on the JSONL session file; concurrent processes (e.g. two claude-code-go instances for the same session) can corrupt the file | Use `flock(2)` via `golang.org/x/sys/unix` or document the single-writer contract explicitly |
| N-13 | `internal/bootstrap/bootstrap.go` | Project hash algorithm is undocumented; if Agent-Session and Agent-Core use different algorithms the session directory cannot be found | Document the exact algorithm (e.g. `sha256(abs(projectRoot))[0:8]`) in the design doc |
| N-14 | `pkg/types/logs.go` | `LogOption` accumulates a `[]string` for each option; if options are read-heavy and write-rare, allocating a slice per option wastes memory | Use a plain `string` for single-value options; only use `[]string` for multi-value options |

---

## 4. Required Changes

The following changes **must** be completed before implementation begins (task #13). Items marked BLOCKER must be resolved in the design document; items marked MAJOR may be addressed directly in code during implementation.

| ID | Priority | Change |
|----|----------|--------|
| RC-1 | BLOCKER | Fix Policy merge order: Policy must unconditionally override Local/Project/User locked fields; update §3.1, §3.3, and `LayeredSettings.Merged` comment to be consistent |
| RC-2 | BLOCKER | Define and enforce copy-on-write convention for all map/slice fields in `AppState`; rewrite §4.4 example to use `maps.Clone`; add `-race` CI gate |
| RC-3 | BLOCKER | Add environment variable override pass in `Loader.Load()` or as a post-merge step; document the complete env-var → field mapping table in §3.1 |
| RC-4 | MAJOR | Remove `NewAgentId` panic stub from `pkg/types/ids.go`; all ID generation must go through `pkg/utils/ids` |
| RC-5 | MAJOR | Fix `scanner.Bytes()` buffer reuse bug in `SessionStore.ReadAll` — copy bytes before storing as `json.RawMessage` |
| RC-6 | MAJOR | Add `ToolCall` and `ToolResult` as first-class types in `pkg/types` |
| RC-7 | MAJOR | Provide core type skeletons in `hooks.go` (`HookType`, `HookDefinition`, `HookResult`) and `plugin.go` (`LoadedPlugin`, `PluginConfig`); change `SettingsJson.Hooks` to `map[HookType][]HookDefinition` |
| RC-8 | MAJOR | Implement `Subscribe`/unsubscribe using `map[uint64]Listener[T]` instead of nil-slot slice |
| RC-9 | MAJOR | Complete `applyLayer` to cover all `SettingsJson` fields with explicit merge semantics per field |
| RC-10 | MAJOR | Replace `[]any` for `MCPClients`, `MCPTools`, `MCPServers` in `AppState` with typed interfaces or concrete stubs |
| RC-11 | MAJOR | Define `ConfigLoader` interface in `internal/config` and `SessionStorer` interface in `internal/session` |
| RC-12 | MAJOR | Add `context.Context` as the first parameter to `LocalCommand.Call` |
| RC-13 | MINOR | Fix `SerializedMessage` field shadowing — remove duplicate `SessionId`/`Timestamp` fields at the outer level |
| RC-14 | MINOR | Rename `pkg/utils/json` package to `jsonutil` to avoid collision with `encoding/json` |
| RC-15 | MINOR | Define `PermissionUpdateType` string type with named constants; replace bare `string` in `PermissionUpdate.Type` |
| RC-16 | MINOR | Complete `EntryType` constants to cover all 20+ TypeScript variants |
| RC-17 | MINOR | Add `json:"-"` tag to `EntryEnvelope.Raw` with explanatory comment |
| RC-18 | MINOR | Replace all `os.IsNotExist(err)` with `errors.Is(err, fs.ErrNotExist)` |
| RC-19 | MINOR | Add JSON tags to all `ToolPermissionContext` fields matching TypeScript camelCase field names |
| RC-20 | MINOR | Handle `rand.Read` error in `pkg/utils/ids/ids.go` — panic on entropy source failure |
| RC-21 | MINOR | Define `homeDir()` in `internal/bootstrap`; document the project hash algorithm |

---

## 5. Implementation Notes for Agent-Infra

### 5.1 Implementation Order

Implement in this order to minimise rework:

1. **`pkg/types`** (all files) — stabilise before touching `internal/`. Every other package depends on this; type changes cascade across the entire codebase.
2. **`pkg/utils`** — stateless helpers, easy to unit test in isolation.
3. **`internal/config`** — `AppState` embeds `SettingsJson`, so `GetDefaultAppState` cannot be finalised until config types are stable.
4. **`internal/state`** — implement `Store[T]` with the race detector enabled from day one.
5. **`internal/session`** — depends on stable `pkg/types` entry types.
6. **`internal/bootstrap`** — wires everything together; implement last.

### 5.2 Testing Requirements

- **`pkg/types`**: unit tests for every constructor and type-conversion helper; zero filesystem I/O.
- **`internal/config`**: table-driven tests covering: (a) no config files present, (b) all three layers present, (c) Policy overrides Local, (d) permission arrays merge with deduplication, (e) env-var overrides file value.
- **`internal/state`**: concurrent tests with multiple goroutines calling `SetState` and `GetState` simultaneously; must pass `go test -race`.
- **`internal/session`**: round-trip tests (write entries → read back → assert equality); tests for corrupt-line handling.

### 5.3 Race Detector CI Gate

Add the following to the CI pipeline before Phase 1 is marked complete:

```
go test -race ./internal/state/... ./internal/session/... ./internal/config/...
```

No PR touching `internal/state` may be merged if the race detector reports any issues.

### 5.4 `pkg/types` Stability Contract

`pkg/types` is the dependency anchor for the entire project. After Agent-Infra submits the initial implementation, treat all exported types as stable — breaking changes require a Tech Lead approval and coordinated update across all dependent packages. Add a comment block at the top of each file in `pkg/types`:

```go
// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
```

### 5.5 Interface Placement Convention

Define interfaces in the **consumer** package, not the producer package, following standard Go convention. The `ConfigLoader` interface belongs in the package that uses it (e.g. `internal/bootstrap`), not in `internal/config`. `SessionStorer` belongs in `internal/bootstrap` or wherever the session is consumed. This prevents import cycles and keeps the interface minimal.

### 5.6 Project Hash Algorithm

The session directory path is derived from the project root using a hash. Document and fix the algorithm to:

```go
func projectHash(root string) string {
    abs, _ := filepath.Abs(root)
    h := sha256.Sum256([]byte(abs))
    return hex.EncodeToString(h[:4]) // 8 hex chars
}
```

This must be identical in every component that constructs or resolves session paths (`internal/session`, `internal/bootstrap`, any future Agent-Session). Put the canonical implementation in `pkg/utils/fs`.

### 5.7 `ToolPermissionContext` JSON Tags

When adding JSON tags to `ToolPermissionContext`, verify the field names against the TypeScript source. The canonical mapping (TypeScript → Go JSON tag) is:

| TypeScript field | Go JSON tag |
|-----------------|-------------|
| `toolName` | `json:"toolName"` |
| `toolInput` | `json:"toolInput"` |
| `sessionId` | `json:"sessionId"` |
| `transcriptPath` | `json:"transcriptPath"` |

### 5.8 `ReadAll` Logging

When adding the `slog` warn log for corrupt entries in `ReadAll` (RC-20 / N-9), use structured fields:

```go
slog.Warn("session: skipping corrupt entry",
    "file", s.path,
    "line", lineNum,
    "error", err,
)
```

Do not log the raw line content — it may contain PII from tool outputs.

### 5.9 `crypto/rand` Error Handling

In `pkg/utils/ids/ids.go`, the `rand.Read` call should panic with a diagnostic message rather than silently continuing:

```go
if _, err := rand.Read(b); err != nil {
    panic(fmt.Sprintf("claude-code-go: crypto/rand unavailable: %v", err))
}
```

This is the correct Go idiom for non-recoverable initialisation failures. It is preferable to silent ID collisions.

---

## 6. Summary Checklist

| Criterion | Status | Notes |
|-----------|--------|-------|
| `pkg/types` zero-dependency | ✅ Correct | Remove panic stub (RC-4) |
| Shared types completeness | ⚠️ Partial | `hooks.go`/`plugin.go` empty (RC-7); `ToolCall`/`ToolResult` absent (RC-6); `EntryType` incomplete (RC-16) |
| Config priority (env > file > default) | ❌ Two defects | Policy merge order inverted (RC-1 / B-1); env-var layer entirely absent (RC-3 / B-3) |
| `applyLayer` completeness | ❌ Incomplete | 15+ fields silently discarded (RC-9 / M-6) |
| `AppState` concurrency safety | ❌ Design defect | map/slice copy-on-write not enforced (RC-2 / B-2); `Subscribe` memory leak (RC-8 / M-5) |
| Session persistence robustness | ⚠️ Mostly correct | `scanner.Bytes()` copy bug (RC-5 / M-1); `file.Sync()` before rename not specified (N-11) |
| No circular dependencies | ✅ Correct | DAG is correct; `AppStateReader` contract should be documented |
| Testability / interfaces | ❌ Absent | `Loader`, `SessionStore`, `SessionManager` have no interfaces (RC-11 / M-8) |
| Bootstrap assembly | ⚠️ Mostly correct | `homeDir()` undefined (RC-21); env-var override absent (RC-3); `context.Context` missing from `Call` (RC-12) |

**Agent-Infra may begin implementation (task #13) once all BLOCKER items (B-1, B-2, B-3) and MAJOR items (M-1 through M-8) are resolved.**
