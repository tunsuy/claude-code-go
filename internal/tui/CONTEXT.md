---
package: tui
import_path: internal/tui
layer: tui
generated_at: 2026-09-23T11:03:32Z
source_files: [agentcolors.go, cmds.go, coordinator.go, init.go, input.go, keys.go, messagelist.go, messages.go, model.go, permissions.go, spinner.go, statusbar.go, styles.go, theme.go, update.go, view.go, welcome.go]
---

# internal/tui

> Layer: **TUI** · Files: 17 · Interfaces: 0 · Structs: 35 · Functions: 6

## Structs

- **AgentColorManager** — 3 fields
- **AgentProgressMsg** — 3 fields: TaskID, Activity, Detail
- **AgentStatusMsg** — 3 fields: TaskID, Status, Description
- **AgentTaskState** — 12 fields: ID, Name, AgentType, Description, Status, StartTime, ElapsedMs, OutputTokens, ...
- **AppModel** — 45 fields
- **CommandResultMsg** — 2 fields: Text, IsError
- **CompactDoneMsg** — 1 fields: Summary
- **CoordinatorPanel** — 3 fields: Tasks, SelectedIndex, TaskOrder
- **DreamDoneMsg** — 1 fields: Err
- **InputChangedMsg** — 1 fields: Text
- **InputModel** — 4 fields
- **InputSubmittedMsg** — 1 fields: Text
- **MemdirLoadedMsg** — 2 fields: Paths, ScopedFiles
- **MessageLookups** — 4 fields: ToolUseToResult, ResolvedToolUseIDs, ErroredToolUseIDs, InProgressToolUseIDs
- **PermissionDialog** — 8 fields
- **PermissionRequestMsg** — 7 fields: RequestID, ToolName, ToolUseID, Message, Input, ProjectPath, RespFn
- **SlashCommandMsg** — 2 fields: Name, Args
- **SpinnerModel** — 4 fields
- **StatusBar** — 6 fields
- **StreamAssistantTurnMsg** — 1 fields: FinalMessage
- **StreamDoneMsg** — 1 fields: FinalMessage
- **StreamErrorMsg** — 1 fields: Err
- **StreamThinkingMsg** — 1 fields: Delta
- **StreamTokenMsg** — 1 fields: Delta
- **StreamToolResultMsg** — 3 fields: ToolUseID, Content, IsError
- **StreamToolUseCompleteMsg** — 2 fields: ToolUseID, ToolInput
- **StreamToolUseInputDeltaMsg** — 3 fields: ToolUseID, ToolName, InputDelta
- **StreamToolUseStartMsg** — 3 fields: ToolUseID, ToolName, InputDelta
- **StreamUserTurnMsg** — 1 fields: FinalMessage
- **SystemTextMsg** — 1 fields: Text
- **TermResizedMsg** — 2 fields: Width, Height
- **Theme** — 11 fields: Primary, Secondary, Accent, Muted, Error, Warning, Success, CodeBG, ...
- **TickMsg** — 1 fields: Time
- **TokenUsage** — 4 fields: Input, Output, CacheRead, CacheCreated
- **WelcomeHeader** — 4 fields

## Functions

- `IsSlashCommand(text string) bool`
- `MessageListView(messages []types.Message, width int, darkMode bool, theme Theme, mdRenderer *glamour.TermRenderer, expandedToolResults map[string]bool) string`
- `New(qe engine.QueryEngine, appStore *state.AppStateStore, vimEnabled bool, dark bool, permAskCh <-chan permissions.AskRequest, permRespCh chan<- permissions.AskResponse, agentCoord tools.AgentCoordinator, agentEventCh <-chan coordinator.Event, mq *msgqueue.MessageQueue, qg *msgqueue.QueryGuard, memoryStore *memdir.MemoryStore) tea.Model`
- `NewAgentColorManager() *AgentColorManager`
- `NewInput(vimEnabled bool) InputModel`
- `NewWelcomeHeader(model string, cwd string) WelcomeHeader`

## Constants

- `AgentCompleted`
- `AgentFailed`
- `AgentPaused`
- `AgentRunning`
- `PermissionChoiceAlwaysAllow`
- `PermissionChoiceNo`
- `PermissionChoiceYes`
- `SpinnerModeBrief`
- `SpinnerModeNormal`
- `SpinnerModeTeammate`
- `ToolStatusError`
- `ToolStatusInProgress`
- `ToolStatusQueued`
- `ToolStatusResolved`
- `VimModeInsert`
- `VimModeNormal`
- `VimModeVisual`

## Dependencies

**Imports:** `internal/commands`, `internal/coordinator`, `internal/memdir`, `internal/state`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。
