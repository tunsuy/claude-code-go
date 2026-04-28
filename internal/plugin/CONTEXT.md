---
package: plugin
import_path: internal/plugin
layer: infra
generated_at: 2026-04-28T12:11:54Z
source_files: [plugin.go]
---

# internal/plugin

> Layer: **Infra** · Files: 1 · Interfaces: 0 · Structs: 1 · Functions: 2

## Structs

- **Manager** — 2 fields

## Functions

- `EnsureNoFatalErrors(errs []error) error`
- `NewManager(configs []types.PluginConfig) (*Manager, []error)`

## Dependencies

**Imports:** `pkg/types`

