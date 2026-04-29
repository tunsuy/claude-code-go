---
package: config
import_path: internal/config
layer: infra
generated_at: 2026-04-29T02:31:52Z
source_files: [loader.go, settings.go]
---

# internal/config

> Layer: **Infra** · Files: 2 · Interfaces: 1 · Structs: 6 · Functions: 1

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
- **SettingsJson** — 26 fields: Schema, APIKey, APIKeyHelper, BaseURL, Provider, AWSCredentialExport, AWSAuthRefresh, GCPAuthRefresh, ...
- **WorktreeConfig** — 2 fields: SymlinkDirectories, SparsePaths

## Functions

- `NewLoader(homeDir string, projectDir string) *Loader`

## Constants

- `ClaudeDir`
- `ClaudeLocalDir`
- `ManagedSettingsFile`
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

**Imported by:** `internal/bootstrap`, `internal/state`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
