---
package: fs
import_path: pkg/utils/fs
layer: types
generated_at: 2026-04-29T02:31:52Z
source_files: [fs.go]
---

# pkg/utils/fs

> Layer: **Types (zero-dep)** · Files: 1 · Interfaces: 0 · Structs: 0 · Functions: 4

## Functions

- `AtomicWriteFile(path string, data []byte, perm os.FileMode) error`
- `EnsureDir(path string) error`
- `ProjectHash(projectRoot string) (string, error)`
- `SafeReadFile(path string) ([]byte, error)`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/session`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
