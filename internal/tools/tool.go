// Package tools defines the Tool interface contract and registry.
// All built-in tool implementations live in sub-packages of this package.
//
// Dependency direction: this package has ZERO external dependencies and must
// NOT import engine, permissions, tui, or any other internal package.
package tools

import (
	"context"
	"encoding/json"
)

// Input represents tool call input parameters (a JSON object).
type Input = json.RawMessage

// InputSchema describes the tool input as a JSON Schema (object type).
type InputSchema struct {
	Type       string                     `json:"type"`                 // always "object"
	Properties map[string]json.RawMessage `json:"properties,omitempty"` // property definitions
	Required   []string                   `json:"required,omitempty"`   // required property names
}

// InterruptBehavior specifies what happens when a new user message arrives
// while this tool is executing.
type InterruptBehavior string

const (
	// InterruptBehaviorCancel stops the tool and discards the result.
	InterruptBehaviorCancel InterruptBehavior = "cancel"
	// InterruptBehaviorBlock continues execution; the new message waits.
	InterruptBehaviorBlock InterruptBehavior = "block"
)

// PermissionBehavior is the decision returned by permission checks.
type PermissionBehavior string

const (
	// PermissionAllow allows the tool call unconditionally.
	PermissionAllow PermissionBehavior = "allow"
	// PermissionDeny denies the tool call unconditionally.
	PermissionDeny PermissionBehavior = "deny"
	// PermissionAsk requires explicit user confirmation.
	PermissionAsk PermissionBehavior = "ask"
	// PermissionPassthrough defers the decision to the framework.
	PermissionPassthrough PermissionBehavior = "passthrough"
)

// PermissionContext provides current permission-mode information to tools for
// rendering their descriptions and prompts.
// The concrete implementation is owned by Agent-Core; tools treat it as opaque.
type PermissionContext interface {
	// Mode returns the current permission mode, e.g. "normal" or "plan".
	Mode() string
}

// AgentCoordinator is the subset of coordinator.Coordinator that tools need.
// Defined in tools to avoid a dependency cycle (tools → coordinator).
// The concrete implementation lives in internal/coordinator.
type AgentCoordinator interface {
	// SpawnAgent launches a new sub-agent and returns its ID (string).
	SpawnAgent(ctx context.Context, req AgentSpawnRequest) (string, error)

	// SendMessage delivers a follow-up message to a running sub-agent.
	SendMessage(ctx context.Context, agentID string, message string) error

	// StopAgent stops a running sub-agent.
	StopAgent(ctx context.Context, agentID string) error

	// GetAgentStatus returns the current lifecycle state of a sub-agent.
	GetAgentStatus(ctx context.Context, agentID string) (string, error)

	// GetAgentResult returns the final result and status of a finished sub-agent.
	// Returns ("", "", ErrNotFound) if unknown or ("", status, nil) if still running.
	GetAgentResult(ctx context.Context, agentID string) (result string, status string, err error)

	// ListAgents returns all agent IDs with their statuses.
	ListAgents() map[string]string

	// WaitForAgent blocks until the specified agent finishes (or ctx is cancelled).
	// Returns the agent's final result and error.
	WaitForAgent(ctx context.Context, agentID string) (string, error)

	// ResolveAgent resolves a name or ID to an agent ID string.
	// Returns the resolved agent ID, or an error if not found.
	ResolveAgent(ctx context.Context, target string) (string, error)

	// BroadcastMessage sends a message to all running agents.
	// Returns the number of agents that received the message.
	BroadcastMessage(ctx context.Context, message string) (int, error)
}

// AgentSpawnRequest is the parameter bundle for AgentCoordinator.SpawnAgent.
// Mirrors coordinator.SpawnRequest but uses only standard-library types.
type AgentSpawnRequest struct {
	Description  string
	Prompt       string
	AllowedTools []string
	DenyTools    []string // tools to explicitly exclude
	MaxTurns     int
	AgentType    string // agent type key (e.g. "worker", "explore")
	Model        string // model override (empty = inherit)
	Background   bool   // if true, don't block for completion
	AgentName    string // human-readable name for routing
	CacheParams  any    // opaque; cast to *engine.CacheSafeParams at wire time
}

// UseContext is the per-call context passed into Tool methods.
// The engine constructs a fresh UseContext for every tool invocation.
//
// NOTE(M-1): context.Context is embedded here as Ctx. A future API revision
// may promote it to a first parameter of Call; tools should cancel via Ctx
// and must NOT hold a reference to *UseContext beyond the Call return.
type UseContext struct {
	// Ctx is the cancellation context for this tool call.
	Ctx context.Context
	// AbortCh is closed when the tool should stop early.
	AbortCh <-chan struct{}
	// PermCtx provides permission information to the tool.
	PermCtx PermissionContext
	// Coordinator provides access to the multi-agent coordinator.
	// May be nil if coordinator mode is not enabled.
	Coordinator AgentCoordinator
	// AgentID is the ID of the current agent ("" for main session).
	AgentID string
}

// ValidationResult is the output of ValidateInput.
type ValidationResult struct {
	OK     bool   // true if input is valid
	Reason string // human-readable explanation when OK is false
}

// PermissionResult is the output of CheckPermissions.
type PermissionResult struct {
	Behavior PermissionBehavior // the tool's permission decision
	Reason   string             // human-readable explanation
}

// Result is the output of a successful tool Call.
type Result struct {
	// Content is the structured result data passed back to the LLM.
	Content any
	// IsError indicates the call failed (content holds error message).
	IsError bool
	// ContextModifier optionally mutates the UseContext after the call.
	// MUST be nil when IsConcurrencySafe returns true (enforced by engine).
	ContextModifier func(*UseContext)
}

// OnProgressFn is called by long-running tools to report intermediate progress.
// Tools must tolerate a nil callback.
type OnProgressFn func(data any)

// SearchOrReadResult carries metadata from SearchOrReadTool.
type SearchOrReadResult struct {
	IsSearch bool
	IsRead   bool
	Path     string
}

// MCPInfo holds MCP-specific tool metadata.
type MCPInfo struct {
	ServerName string
	ToolName   string
}

// Tool is the interface every built-in tool must implement.
//
// Methods are grouped into five concerns:
//  1. Identity & Metadata
//  2. Concurrency & Safety
//  3. Permissions
//  4. Execution
//  5. Serialization
//
// See BaseTool for default implementations of optional zero-value methods.
type Tool interface {
	// ── Identity & Metadata ──────────────────────────────────────────────────

	// Name returns the canonical tool name (e.g. "Bash", "Read", "Glob").
	Name() string

	// Aliases returns alternate names for this tool (used by Registry.Get).
	Aliases() []string

	// Description returns the tool description injected into the system prompt.
	// permCtx may be nil; implementations must handle that gracefully.
	Description(input Input, permCtx PermissionContext) string

	// InputSchema returns the JSON Schema for this tool's input object.
	InputSchema() InputSchema

	// Prompt optionally returns extra content to inject into the system prompt
	// for this tool. Return ("", nil) to inject nothing.
	Prompt(ctx context.Context, permCtx PermissionContext) (string, error)

	// MaxResultSizeChars returns the character limit for tool results.
	// Return -1 for no limit.
	MaxResultSizeChars() int

	// SearchHint returns a short string used by ToolSearch for keyword matching.
	SearchHint() string

	// ── Concurrency & Safety ─────────────────────────────────────────────────

	// IsConcurrencySafe returns true if the tool can execute concurrently with
	// other tools in the same turn without side-effect conflicts.
	// Read-only tools should return true; write tools must return false.
	IsConcurrencySafe(input Input) bool

	// IsReadOnly returns true if the tool makes no persistent changes.
	IsReadOnly(input Input) bool

	// IsDestructive returns true if the tool's effects are hard to reverse.
	IsDestructive(input Input) bool

	// IsEnabled returns false when the tool is disabled by feature flags or
	// platform constraints (the Registry will not surface disabled tools).
	IsEnabled() bool

	// InterruptBehavior specifies what the engine does when a new user message
	// arrives while this tool is running.
	InterruptBehavior() InterruptBehavior

	// ── Permissions ──────────────────────────────────────────────────────────

	// ValidateInput checks whether input is structurally valid before the
	// permission layer runs. Return ValidationResult{OK: true} to skip.
	ValidateInput(input Input, ctx *UseContext) (ValidationResult, error)

	// CheckPermissions performs tool-specific permission logic.
	// Return PermissionResult{Behavior: PermissionPassthrough} to delegate.
	CheckPermissions(input Input, ctx *UseContext) (PermissionResult, error)

	// PreparePermissionMatcher returns a function that tests whether a given
	// permission pattern (e.g. "git commit *") covers this input.
	// Return nil to skip pattern matching.
	PreparePermissionMatcher(input Input) (func(pattern string) bool, error)

	// ── Execution ────────────────────────────────────────────────────────────

	// Call executes the tool and returns its result.
	// ctx.Ctx carries the cancellation signal; onProgress may be nil.
	Call(input Input, ctx *UseContext, onProgress OnProgressFn) (*Result, error)

	// ── Serialization ────────────────────────────────────────────────────────

	// MapResultToToolResultBlock converts tool output into an Anthropic API
	// tool_result content block (json.RawMessage).
	MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error)

	// ToAutoClassifierInput returns the string fed into the auto-classifier.
	// Return "" to skip classification.
	ToAutoClassifierInput(input Input) string

	// UserFacingName returns a human-readable label for this tool call,
	// e.g. "Edit(src/main.go)" or "Bash(git status)".
	UserFacingName(input Input) string

	// NOTE: GetPath has been moved to the PathTool sub-interface (B-1 fix).
}

// PathTool is the optional sub-interface implemented by file-operation tools.
// The engine uses type assertion to call GetPath for path-conflict detection
// in concurrent batches.
type PathTool interface {
	Tool
	// GetPath extracts the primary file path from the tool input.
	GetPath(input Input) string
}

// SearchOrReadTool is the optional sub-interface for search/read tools.
// The engine uses it to decide UI folding behaviour.
type SearchOrReadTool interface {
	Tool
	// IsSearchOrRead returns metadata about whether this call is a search or read.
	IsSearchOrRead(input Input) SearchOrReadResult
}

// MCPToolInfo is the optional sub-interface for MCP-backed tools.
// Used only for metadata display; the engine schedules these like any Tool.
type MCPToolInfo interface {
	Tool
	// MCPInfo returns the MCP server and tool name for this tool.
	MCPInfo() MCPInfo
}

// NewInputSchema is a convenience constructor for InputSchema.
// props is a map of property-name → JSON Schema bytes.
func NewInputSchema(props map[string]json.RawMessage, required []string) InputSchema {
	return InputSchema{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}

// PropSchema marshals a property definition map into json.RawMessage.
// Panics on marshal error (only called with literal maps at init time).
func PropSchema(def map[string]any) json.RawMessage {
	b, err := json.Marshal(def)
	if err != nil {
		panic("tool.PropSchema: " + err.Error())
	}
	return b
}
