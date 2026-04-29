---
package: main
import_path: cmd/docgen
layer: unknown
generated_at: 2026-04-29T02:31:52Z
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

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- docgen 用 go/ast 而不是 go/types 解析代码，因为只需要语法级信息（类型名、方法签名），不需要类型推导。go/ast 更轻量且不依赖完整编译。(2026-04-29)
- Design Notes 保留机制：渲染时先写 separator 注释行，重生成时用 extractDesignNotes() 提取旧文件 separator 之后的内容并 append 到新文件。(2026-04-29)
- -check 模式分两步检查：1) AST 生成部分是否过期（hash 比对）2) 改了代码的包是否有 Design Notes（git diff 检测变更包）。(2026-04-29)
- 变更检测用 git merge-base HEAD main 作为基准，比较分支上所有改动的 .go 文件，排除 _test.go 和 CONTEXT.md。(2026-04-29)
