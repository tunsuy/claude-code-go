---
package: msgqueue
import_path: internal/msgqueue
layer: services
generated_at: 2026-04-29T02:31:52Z
source_files: [command.go, guard.go, queue.go, signal.go]
---

# internal/msgqueue

> Layer: **Services** · Files: 4 · Interfaces: 0 · Structs: 4 · Functions: 5

## Structs

- **MessageQueue** — 3 fields
- **QueryGuard** — 4 fields
- **QueuedCommand** — 6 fields: ID, Value, Mode, Priority, AgentID, CreatedAt
- **Signal** — 3 fields

## Functions

- `NewCommand(value string, mode CommandMode, priority Priority) QueuedCommand`
- `NewCommandWithAgent(value string, mode CommandMode, priority Priority, agentID string) QueuedCommand`
- `NewMessageQueue() *MessageQueue`
- `NewQueryGuard() *QueryGuard`
- `NewSignal() *Signal`

## Constants

- `Dispatching`
- `Idle`
- `ModePrompt`
- `ModeSlashCommand`
- `PriorityLater`
- `PriorityNext`
- `PriorityNow`
- `Running`

## Change Impact

**Exported type references (files that use types from this package):**
- `MessageQueue` → `internal/bootstrap/wire.go`, `internal/engine/engine.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `QueryGuard` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `QueuedCommand` → `internal/engine/query.go`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/engine`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
