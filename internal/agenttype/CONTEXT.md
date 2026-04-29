---
package: agenttype
import_path: internal/agenttype
layer: infra
generated_at: 2026-04-29T02:31:52Z
source_files: [agenttype.go, builtins.go, loader.go, prompts.go, registry.go, tasktype.go]
---

# internal/agenttype

> Layer: **Infra** · Files: 6 · Interfaces: 0 · Structs: 3 · Functions: 3

## Structs

- **AgentProfile** — 9 fields: Type, DisplayName, Description, WhenToUse, SystemPrompt, ToolFilter, Model, MaxTurns, ...
- **Registry** — 3 fields
- **ToolFilter** — 2 fields: Mode, Tools

## Functions

- `LoadCustomAgents(dir string) ([]*AgentProfile, error)`
- `NewRegistry() *Registry`
- `RegisterBuiltins(r *Registry)`

## Constants

- `AgentTypeCustom`
- `AgentTypeExplore`
- `AgentTypeGuide`
- `AgentTypePlan`
- `AgentTypeVerify`
- `AgentTypeWorker`
- `TaskTypeInProcessTeammate`
- `TaskTypeLocalAgent`
- `TaskTypeLocalBash`
- `TaskTypeRemote`
- `ToolFilterAllowlist`
- `ToolFilterDenylist`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `stubRegistry` in `internal/permissions/checker_test.go`

**Exported type references (files that use types from this package):**
- `AgentProfile` → `internal/bootstrap/wire.go`
- `Registry` → `internal/bootstrap/wire.go`
- `ToolFilter` → `internal/bootstrap/wire.go`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
