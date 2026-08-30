---
package: config
import_path: internal/config
layer: infra
generated_at: 2026-08-30T01:47:48Z
source_files: [loader.go, settings.go, writer.go]
---

# internal/config

> Layer: **Infra** · Files: 3 · Interfaces: 1 · Structs: 6 · Functions: 2

## Interfaces

### ConfigLoader (1 methods)
> ConfigLoader is the interface for loading layered settings.

```go
type ConfigLoader interface {
    Load() (*LayeredSettings, error)
}
```

## Structs

- **AttributionConfig** — 2 fields: Commit, PR
- **LayeredSettings** — 5 fields: User, Project, Local, Policy, Merged
- **Loader** — 2 fields
- **PermissionsConfig** — 6 fields: Allow, Deny, Ask, DefaultMode, DisableBypass, AdditionalDirs
- **SettingsJson** — 28 fields: Schema, APIKey, APIKeyHelper, BaseURL, Provider, AWSCredentialExport, AWSAuthRefresh, GCPAuthRefresh, ...
- **WorktreeConfig** — 2 fields: SymlinkDirectories, SparsePaths

## Functions

- `AddPermissionRule(projectDir string, list string, rule string) error`
- `NewLoader(homeDir string, projectDir string) *Loader`

## Constants

- `ClaudeDir`
- `ClaudeLocalDir`
- `ManagedSettingsFile`
- `PermissionListAllow`
- `PermissionListAsk`
- `PermissionListDeny`
- `SessionsDir`
- `SettingsFile`
- `SourceLocal`
- `SourcePolicy`
- `SourceProject`
- `SourceUser`
- `StatsFile`
- `TodosDir`

## Change Impact

**Exported type references (files that use types from this package):**
- `LayeredSettings` → `internal/bootstrap/wire.go`
- `SettingsJson` → `internal/state/store.go`

## Dependencies

**Imports:** `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/permissions`, `internal/state`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
