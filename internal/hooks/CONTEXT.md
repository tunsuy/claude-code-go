---
package: hooks
import_path: internal/hooks
layer: infra
generated_at: 2026-04-29T02:31:52Z
source_files: [hooks.go]
---

# internal/hooks

> Layer: **Infra** · Files: 1 · Interfaces: 0 · Structs: 1 · Functions: 1

## Structs

- **Dispatcher** — 2 fields

## Functions

- `NewDispatcher(hooks map[types.HookType][]types.HookDefinition, disabled bool) *Dispatcher`

## Change Impact

**Exported type references (files that use types from this package):**
- `Dispatcher` → `internal/permissions/checker.go`

## Dependencies

**Imports:** `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/permissions`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
