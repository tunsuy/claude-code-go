---
package: commands
import_path: internal/commands
layer: cli
generated_at: 2026-09-05T09:11:12Z
source_files: [builtins.go, registry.go]
---

# internal/commands

> Layer: **CLI** · Files: 2 · Interfaces: 0 · Structs: 4 · Functions: 2

## Structs

- **Command** — 3 fields: Name, Description, Execute
- **CommandContext** — 9 fields: WorkingDir, Model, SessionID, DarkMode, VimEnabled, Effort, MessageCount, TokensInput, ...
- **Registry** — 2 fields
- **Result** — 11 fields: Text, Display, ShouldExit, NewMessages, NewTheme, NewModel, NewEffort, ToggleVim, ...

## Functions

- `NewRegistry() *Registry`
- `RegisterBuiltins(r *Registry)`

## Constants

- `DisplayDialog`
- `DisplayError`
- `DisplayMessage`
- `DisplayNone`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `stubRegistry` in `internal/permissions/checker_test.go`

**Exported type references (files that use types from this package):**
- `Command` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `CommandContext` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `Registry` → `internal/tui/model.go`
- `Result` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`

## Dependencies

**Imports:** `internal/memdir`

**Imported by:** `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
