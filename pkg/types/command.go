// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
package types

import "context"

// CommandType identifies how a command is implemented.
type CommandType string

const (
	CommandTypePrompt   CommandType = "prompt"
	CommandTypeLocal    CommandType = "local"
	CommandTypeLocalJSX CommandType = "local-jsx" // Go equivalent: TUI component callback
)

// CommandSource indicates the origin of a command.
type CommandSource string

const (
	CommandSourceBuiltin CommandSource = "builtin"
	CommandSourceMCP     CommandSource = "mcp"
	CommandSourcePlugin  CommandSource = "plugin"
	CommandSourceSkills  CommandSource = "skills"
	CommandSourceBundled CommandSource = "bundled"
)

// CommandBase holds the common metadata shared by all commands.
type CommandBase struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Type         CommandType   `json:"type"`
	Source       CommandSource `json:"source,omitempty"`
	Aliases      []string      `json:"aliases,omitempty"`
	IsHidden     bool          `json:"isHidden,omitempty"`
	IsMCP        bool          `json:"isMcp,omitempty"`
	ArgumentHint string        `json:"argumentHint,omitempty"`
	WhenToUse    string        `json:"whenToUse,omitempty"`
	Version      string        `json:"version,omitempty"`
	Immediate    bool          `json:"immediate,omitempty"`
	IsSensitive  bool          `json:"isSensitive,omitempty"`
}

// PromptCommand is a prompt-template-based command.
type PromptCommand struct {
	CommandBase
	ContentLength int      `json:"contentLength"`
	ArgNames      []string `json:"argNames,omitempty"`
	AllowedTools  []string `json:"allowedTools,omitempty"`
	Model         string   `json:"model,omitempty"`
	// Context: "inline" | "fork"
	Context string `json:"context,omitempty"`
	Agent   string `json:"agent,omitempty"`
	// GetPrompt provides the expanded prompt content at runtime (not serialised).
	GetPrompt func(args string) ([]ContentBlock, error) `json:"-"`
}

// LocalCommand is a command implemented directly by a Go function.
type LocalCommand struct {
	CommandBase
	SupportsNonInteractive bool
	// Call is the command execution function.
	// The context.Context is the first argument per Go convention.
	Call func(ctx context.Context, args string, cmdCtx *LocalCommandContext) (*LocalCommandResult, error) `json:"-"`
}

// LocalCommandResult is the execution result of a local command.
type LocalCommandResult struct {
	Type  string // "text" | "compact" | "skip"
	Value string
}

// LocalCommandContext provides the context required for local command execution
// (analogous to TS LocalJSXCommandContext).
type LocalCommandContext struct {
	// Injected core services (interfaces to avoid circular imports)
	AppState   AppStateReader
	SessionId  SessionId
	WorkingDir string
}

// AppStateReader is the read-only AppState access interface used by commands
// and other lower-layer modules to avoid circular dependencies.
type AppStateReader interface {
	GetPermissionContext() ToolPermissionContext
	GetModel() string
	GetVerbose() bool
}
