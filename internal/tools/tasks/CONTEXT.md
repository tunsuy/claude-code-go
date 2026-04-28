---
package: tasks
import_path: internal/tools/tasks
layer: tools
generated_at: 2026-04-28T11:59:48Z
source_files: [doc.go, tasks.go]
---

# internal/tools/tasks

> Layer: **Tools** · Files: 2 · Interfaces: 0 · Structs: 7 · Functions: 0

## Structs

- **Task** — 5 fields: ID, Description, Status, CreatedAt, UpdatedAt
- **TaskCreateInput** — 3 fields: Description, Tools, Priority
- **TaskGetInput** — 1 fields: ID
- **TaskListInput** — 1 fields: Status
- **TaskOutputInput** — 2 fields: ID, Since
- **TaskStopInput** — 1 fields: ID
- **TaskUpdateInput** — 3 fields: ID, Description, Status

## Constants

- `TaskStatusCompleted`
- `TaskStatusFailed`
- `TaskStatusPending`
- `TaskStatusRunning`
- `TaskStatusStopped`

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

