// Package fileops implements file-operation built-in tools:
// Read, Write, Edit, Glob, Grep, NotebookEdit.
//
// All tools in this package are safe to use as singletons (no mutable fields).
// File-type tools (Read, Write, Edit, NotebookEdit) implement the optional
// tool.PathTool sub-interface so the engine can detect path conflicts in
// concurrent batches.
package fileops
