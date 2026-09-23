---
package: fileops
import_path: internal/tools/fileops
layer: tools
generated_at: 2026-09-23T11:03:32Z
source_files: [dangerous.go, doc.go, fileedit.go, fileread.go, filewrite.go, glob.go, grep.go, helpers.go, notebookedit.go, write_atomic.go]
---

# internal/tools/fileops

> Layer: **Tools** · Files: 10 · Interfaces: 0 · Structs: 10 · Functions: 0

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

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。
