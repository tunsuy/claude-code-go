---
package: types
import_path: pkg/types
layer: types
generated_at: 2026-04-29T02:31:52Z
source_files: [command.go, hooks.go, ids.go, logs.go, message.go, permissions.go, plugin.go, types.go]
---

# pkg/types

> Layer: **Types (zero-dep)** · Files: 8 · Interfaces: 2 · Structs: 27 · Functions: 2

## Interfaces

### AppStateReader (3 methods)
> AppStateReader is the read-only AppState access interface used by commands

```go
type AppStateReader interface {
    GetPermissionContext() ToolPermissionContext
    GetModel() string
    GetVerbose() bool
}
```

### MCPConnection (2 methods)
> MCPConnection is a forward-declaration interface for an MCP server connection.

```go
type MCPConnection interface {
    ID() string
    IsConnected() bool
}
```

## Structs

- **AdditionalWorkingDirectory** — 2 fields: Path, Source
- **AggregatedHookResult** — 3 fields: Blocked, BlockReasons, ModifiedContent
- **CommandBase** — 12 fields: Name, Description, Type, Source, Aliases, IsHidden, IsMCP, ArgumentHint, ...
- **ContentBlock** — 11 fields: Type, Text, ID, Name, Input, ToolUseID, Content, IsError, ...
- **EntryEnvelope** — 2 fields: Type, Raw
- **HookDefinition** — 3 fields: Command, Matcher, TimeoutMs
- **HookResult** — 3 fields: Decision, Reason, ModifiedContent
- **ImageSource** — 4 fields: Type, MediaType, Data, URL
- **LoadedPlugin** — 3 fields: Config, Commands, Hooks
- **LocalCommand** — 3 fields: SupportsNonInteractive, Call (embeds CommandBase)
- **LocalCommandContext** — 3 fields: AppState, SessionId, WorkingDir
- **LocalCommandResult** — 2 fields: Type, Value
- **LogOption** — 7 fields: SessionId, Date, FirstPrompt, IsSidechain, Messages, Title, GitBranch
- **Message** — 6 fields: ID, Role, Content, UUID, Timestamp, SessionId
- **PermissionDecision** — 5 fields: Behavior, Message, UserModified, Suggestions, BlockedPath
- **PermissionRule** — 3 fields: Source, RuleBehavior, RuleValue
- **PermissionRuleValue** — 2 fields: ToolName, RuleContent
- **PermissionUpdate** — 6 fields: Type, Destination, Rules, Behavior, Mode, Directories
- **PluginConfig** — 3 fields: Name, Enabled, Options
- **PluginError** — 3 fields: PluginName, Message, Fatal
- **PromptCommand** — 8 fields: ContentLength, ArgNames, AllowedTools, Model, Context, Agent, GetPrompt (embeds CommandBase)
- **SerializedMessage** — 8 fields: Message, CWD, UserType, SessionId, Timestamp, Version, GitBranch, Slug
- **SummaryEntry** — 3 fields: Type, Summary, LeafId
- **ToolCall** — 3 fields: ID, Name, Input
- **ToolPermissionContext** — 6 fields: Mode, AdditionalWorkingDirectories, AlwaysAllowRules, AlwaysDenyRules, AlwaysAskRules, IsBypassPermissionsModeAvailable
- **ToolResult** — 3 fields: ToolUseId, Content, IsError
- **TranscriptMessage** — 4 fields: SerializedMessage, ParentUUID, IsSidechain, AgentId

## Function Types

- `HookCallback` — `func(input map[string]any) (*HookResult, error)`

## Functions

- `AsAgentId(s string) (AgentId, error)`
- `AsSessionId(s string) SessionId`

## Constants

- `BehaviorAllow`
- `BehaviorAsk`
- `BehaviorDeny`
- `CommandSourceBuiltin`
- `CommandSourceBundled`
- `CommandSourceMCP`
- `CommandSourcePlugin`
- `CommandSourceSkills`
- `CommandTypeLocal`
- `CommandTypeLocalJSX`
- `CommandTypePrompt`
- `ContentTypeImage`
- `ContentTypeText`
- `ContentTypeThinking`
- `ContentTypeToolResult`
- `ContentTypeToolUse`
- `EntryTypeAssistant`
- `EntryTypeBash`
- `EntryTypeBranchSwitch`
- `EntryTypeCompletion`
- `EntryTypeContextCompact`
- `EntryTypeDebug`
- `EntryTypeError`
- `EntryTypeImage`
- `EntryTypeInfo`
- `EntryTypePRLink`
- `EntryTypeProgress`
- `EntryTypeResult`
- `EntryTypeSessionEnd`
- `EntryTypeSessionStart`
- `EntryTypeSummary`
- `EntryTypeSystem`
- `EntryTypeTag`
- `EntryTypeThinking`
- `EntryTypeToolResult`
- `EntryTypeToolUse`
- `EntryTypeTranscript`
- `EntryTypeUser`
- `EntryTypeWorktree`
- `HookDecisionApprove`
- `HookDecisionBlock`
- `HookDecisionModify`
- `HookNotification`
- `HookPostSampling`
- `HookPostToolUse`
- `HookPreSampling`
- `HookPreToolUse`
- `HookSessionEnd`
- `HookSessionStart`
- `HookStop`
- `HookSubagentStop`
- `PermissionModeAcceptEdits`
- `PermissionModeAuto`
- `PermissionModeBubble`
- `PermissionModeBypassPermissions`
- `PermissionModeDefault`
- `PermissionModeDontAsk`
- `PermissionModePlan`
- `PermissionUpdateAddDirectories`
- `PermissionUpdateAddRules`
- `PermissionUpdateRemoveDirectories`
- `PermissionUpdateRemoveRules`
- `PermissionUpdateReplaceRules`
- `PermissionUpdateSetMode`
- `RoleAssistant`
- `RoleUser`
- `RuleSourceCLI`
- `RuleSourceCommand`
- `RuleSourceFlag`
- `RuleSourceLocal`
- `RuleSourcePolicy`
- `RuleSourceProject`
- `RuleSourceSession`
- `RuleSourceUser`

## Change Impact

**Exported type references (files that use types from this package):**
- `AdditionalWorkingDirectory` → `internal/bootstrap/wire.go`, `internal/state/store.go`
- `AggregatedHookResult` → `internal/hooks/hooks.go`
- `AppStateReader` → `internal/state/store.go`
- `ContentBlock` → `internal/bootstrap/run.go`, `internal/bootstrap/session_test.go` (test), `internal/bootstrap/wire.go`, `internal/compact/auto.go`, `internal/compact/compact_test.go` (test) + 14 more
- `EntryEnvelope` → `internal/session/store.go`, `internal/session/store_test.go` (test)
- `HookDefinition` → `internal/config/loader.go`, `internal/config/loader_test.go` (test), `internal/hooks/hooks.go`
- `HookResult` → `internal/hooks/hooks.go`
- `LoadedPlugin` → `internal/plugin/plugin.go`, `internal/state/store.go`
- `MCPConnection` → `internal/state/store.go`
- `Message` → `internal/bootstrap/run.go`, `internal/bootstrap/session.go`, `internal/bootstrap/session_test.go` (test), `internal/bootstrap/wire.go`, `internal/compact/auto.go` + 21 more
- `PluginConfig` → `internal/plugin/plugin.go`
- `PluginError` → `internal/plugin/plugin.go`
- `SerializedMessage` → `internal/bootstrap/session.go`, `internal/bootstrap/session_test.go` (test)
- `ToolPermissionContext` → `internal/permissions/checker.go`, `internal/permissions/checker_test.go` (test), `internal/state/store.go`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/compact`, `internal/config`, `internal/engine`, `internal/hooks`, `internal/memdir`, `internal/permissions`, `internal/plugin`, `internal/session`, `internal/state`, `internal/tui`, `pkg/utils/ids`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
