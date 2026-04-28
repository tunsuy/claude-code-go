// Package agenttype defines agent type profiles, tool filters, and a thread-safe
// registry for built-in and custom agent types.
//
// This package sits at the same layer as internal/config — both coordinator/ and
// bootstrap/ may import it without creating dependency cycles.
package agenttype

// AgentType is the canonical string identifier for a sub-agent kind.
type AgentType string

const (
	// AgentTypeWorker is the default general-purpose agent.
	AgentTypeWorker AgentType = "worker"
	// AgentTypeExplore is a read-only exploration agent.
	AgentTypeExplore AgentType = "explore"
	// AgentTypePlan produces structured implementation plans without making changes.
	AgentTypePlan AgentType = "plan"
	// AgentTypeVerify runs tests and linting to validate changes.
	AgentTypeVerify AgentType = "verify"
	// AgentTypeGuide helps with Claude Code usage and documentation.
	AgentTypeGuide AgentType = "guide"
	// AgentTypeCustom is for user-defined agent types loaded from .claude/agents/.
	AgentTypeCustom AgentType = "custom"
)

// String returns the string representation of the agent type.
func (t AgentType) String() string { return string(t) }

// IsBuiltin returns true if the agent type is a built-in type.
func (t AgentType) IsBuiltin() bool {
	switch t {
	case AgentTypeWorker, AgentTypeExplore, AgentTypePlan, AgentTypeVerify, AgentTypeGuide:
		return true
	default:
		return false
	}
}

// ToolFilterMode specifies how tool access is controlled.
type ToolFilterMode string

const (
	// ToolFilterAllowlist only permits the listed tools.
	ToolFilterAllowlist ToolFilterMode = "allowlist"
	// ToolFilterDenylist permits all tools except the listed ones.
	ToolFilterDenylist ToolFilterMode = "denylist"
)

// ToolFilter specifies which tools an agent type may or may not use.
type ToolFilter struct {
	// Mode is "allowlist" or "denylist".
	Mode ToolFilterMode `json:"mode" yaml:"mode"`
	// Tools is the list of tool names for the selected mode.
	Tools []string `json:"list" yaml:"list"`
}

// AgentProfile defines the full configuration for an agent type.
type AgentProfile struct {
	// Type is the canonical agent type identifier.
	Type AgentType `json:"type" yaml:"type"`
	// DisplayName is a human-readable name for TUI display.
	DisplayName string `json:"display_name" yaml:"display_name"`
	// Description is a short description of the agent's purpose.
	Description string `json:"description" yaml:"description"`
	// WhenToUse explains when the main agent should invoke this type.
	WhenToUse string `json:"when_to_use" yaml:"when_to_use"`
	// SystemPrompt is the system prompt template for this agent type.
	SystemPrompt string `json:"system_prompt" yaml:"system_prompt"`
	// ToolFilter defines the tool access policy.
	ToolFilter ToolFilter `json:"tools" yaml:"tools"`
	// Model overrides the default model; empty means inherit parent.
	// Supported values: "", "inherit", "haiku", "sonnet", "opus", or a full model ID.
	Model string `json:"model" yaml:"model"`
	// MaxTurns default; 0 means use coordinator default (30).
	MaxTurns int `json:"max_turns" yaml:"max_turns"`
	// CanSpawnSubAgents controls whether this type can use Agent/SendMessage tools.
	CanSpawnSubAgents bool `json:"can_spawn_sub_agents" yaml:"can_spawn_sub_agents"`
}

// EffectiveModel returns the model to use, resolving "inherit" and "" to empty
// (meaning inherit from parent).
func (p *AgentProfile) EffectiveModel() string {
	if p.Model == "" || p.Model == "inherit" {
		return ""
	}
	return p.Model
}

// CoordinatorOnlyTools is the list of tools that should generally be excluded
// from sub-agents to prevent infinite recursion and unauthorized coordinator
// operations.
var CoordinatorOnlyTools = []string{
	"Agent",
	"SendMessage",
	"GetAgentStatus",
	"TaskCreate",
	"TaskGet",
	"TaskList",
	"TaskUpdate",
	"TaskStop",
	"TaskOutput",
	"EnterPlanMode",
	"ExitPlanMode",
}
