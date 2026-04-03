// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
//
// Types are split across multiple files:
//   ids.go        — SessionId, AgentId branded string types
//   permissions.go — PermissionMode, PermissionRule, PermissionDecision, etc.
//   message.go    — Message, Role, ContentBlock, ToolCall, ToolResult
//   logs.go       — EntryEnvelope, SerializedMessage, TranscriptMessage, LogOption
//   command.go    — CommandBase, PromptCommand, LocalCommand, AppStateReader
//   hooks.go      — HookType, HookDefinition, HookResult, HookCallback
//   plugin.go     — LoadedPlugin, PluginConfig, PluginError, MCPConnection
package types
