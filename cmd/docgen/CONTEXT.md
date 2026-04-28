---
package: main
import_path: cmd/docgen
layer: unknown
generated_at: 2026-04-28T12:11:54Z
source_files: [analyzer.go, dependency.go, impact.go, layer.go, main.go, renderer.go]
---

# cmd/docgen

> Layer: **unknown** · Files: 6 · Interfaces: 0 · Structs: 11 · Functions: 0

## Structs

- **DependencyGraph** — 2 fields: Forward, Reverse
- **FuncInfo** — 3 fields: Name, Signature, Doc
- **FuncTypeInfo** — 2 fields: Name, Signature
- **InterfaceInfo** — 3 fields: Name, Methods, Doc
- **Layer** — 3 fields: Name, Label, Order
- **MethodInfo** — 2 fields: Name, Signature
- **MockInfo** — 2 fields: File, TypeName
- **PackageImpact** — 3 fields: AdapterFiles, Mocks, TypeRefs
- **PackageInfo** — 10 fields: Name, ImportPath, Layer, SourceFiles, Interfaces, Structs, FuncTypes, Functions, ...
- **StructInfo** — 5 fields: Name, FieldCount, FieldNames, Doc, Embeds
- **TypeReference** — 3 fields: File, Package, IsTest

## Dependencies

**Imports:** *(none — zero-dependency)*

