---
package: commands
import_path: internal/commands
layer: cli
generated_at: 2026-04-28T11:59:48Z
source_files: [builtins.go, registry.go]
---

# internal/commands

> Layer: **CLI** · Files: 2 · Interfaces: 0 · Structs: 4 · Functions: 2

## Structs

- **Command** — 3 fields: Name, Description, Execute
- **CommandContext** — 9 fields: WorkingDir, Model, SessionID, DarkMode, VimEnabled, Effort, MessageCount, TokensInput, ...
- **Registry** — 2 fields
- **Result** — 10 fields: Text, Display, ShouldExit, NewMessages, NewTheme, NewModel, NewEffort, ToggleVim, ...

## Functions

- `NewRegistry() *Registry`
- `RegisterBuiltins(r *Registry)`

## Constants

- `DisplayDialog`
- `DisplayError`
- `DisplayMessage`
- `DisplayNone`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/tui`

