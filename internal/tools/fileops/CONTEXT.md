---
package: fileops
import_path: internal/tools/fileops
layer: tools
generated_at: 2026-04-29T02:31:52Z
source_files: [doc.go, fileedit.go, fileread.go, filewrite.go, glob.go, grep.go, helpers.go, notebookedit.go, write_atomic.go]
---

# internal/tools/fileops

> Layer: **Tools** · Files: 9 · Interfaces: 0 · Structs: 10 · Functions: 0

## Structs

- **FileEditInput** — 3 fields: FilePath, OldString, NewString
- **FileReadInput** — 3 fields: FilePath, Offset, Limit
- **FileReadOutput** — 8 fields: Type, FilePath, Content, NumLines, StartLine, TotalLines, Base64, MediaType
- **FileWriteInput** — 2 fields: FilePath, Content
- **GlobInput** — 2 fields: Pattern, Path
- **GlobOutput** — 3 fields: Filenames, NumFiles, Truncated
- **GrepInput** — 5 fields: Pattern, Path, Include, OutputMode, MaxResults
- **GrepMatch** — 3 fields: Path, Line, Content
- **GrepOutput** — 5 fields: Matches, Files, Counts, NumResults, Truncated
- **NotebookEditInput** — 5 fields: NotebookPath, CellNumber, NewSource, CellType, EditMode

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
