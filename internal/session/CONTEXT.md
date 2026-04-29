---
package: session
import_path: internal/session
layer: infra
generated_at: 2026-04-29T02:31:52Z
source_files: [store.go]
---

# internal/session

> Layer: **Infra** · Files: 1 · Interfaces: 1 · Structs: 2 · Functions: 3

## Interfaces

### SessionStorer (3 methods)
> SessionStorer is the interface for reading and writing session entries.

```go
type SessionStorer interface {
    AppendEntry(entry any) error
    ReadAll() ([]types.EntryEnvelope, error)
    Close() error
}
```

## Structs

- **SessionManager** — 3 fields: SessionId
- **SessionStore** — 3 fields

## Functions

- `New(projectDir string) (types.SessionId, *SessionManager, error)`
- `OpenSessionStore(path string) (*SessionStore, error)`
- `Resume(sessionIDStr string, projectDir string) (types.SessionId, *SessionManager, error)`

## Change Impact

**Exported type references (files that use types from this package):**
- `SessionManager` → `internal/bootstrap/session.go`

## Dependencies

**Imports:** `pkg/types`, `pkg/utils/fs`, `pkg/utils/ids`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
