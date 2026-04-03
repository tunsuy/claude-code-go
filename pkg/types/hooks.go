// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
package types

// HookType enumerates the supported hook lifecycle events.
type HookType string

const (
	HookPreToolUse     HookType = "PreToolUse"
	HookPostToolUse    HookType = "PostToolUse"
	HookStop           HookType = "Stop"
	HookSessionStart   HookType = "SessionStart"
	HookSessionEnd     HookType = "SessionEnd"
	HookPreSampling    HookType = "PreSampling"
	HookPostSampling   HookType = "PostSampling"
	HookNotification   HookType = "Notification"
	HookSubagentStop   HookType = "SubagentStop"
)

// HookDecision is the decision returned by a hook execution.
type HookDecision string

const (
	HookDecisionBlock    HookDecision = "block"
	HookDecisionApprove  HookDecision = "approve"
	HookDecisionModify   HookDecision = "modify"
)

// HookDefinition is a single hook configuration entry from settings.json.
type HookDefinition struct {
	// Command is the shell command to execute.
	Command string `json:"command"`
	// Matcher is an optional glob/regex to match the tool name (for PreToolUse / PostToolUse).
	Matcher string `json:"matcher,omitempty"`
	// TimeoutMs is the execution timeout in milliseconds (0 means use default).
	TimeoutMs int `json:"timeout_ms,omitempty"`
}

// HookResult is the structured output returned by a hook command.
type HookResult struct {
	Decision HookDecision `json:"decision"`
	Reason   string       `json:"reason,omitempty"`
	// ModifiedContent is set when Decision == HookDecisionModify.
	ModifiedContent string `json:"modifiedContent,omitempty"`
}

// AggregatedHookResult combines the results of multiple hook executions for
// the same lifecycle event.
type AggregatedHookResult struct {
	// Blocked is true if any hook returned HookDecisionBlock.
	Blocked bool
	// BlockReasons collects the reasons from all blocking hooks.
	BlockReasons []string
	// ModifiedContent contains the final modified content (last writer wins).
	ModifiedContent string
}

// HookCallback is the Go-native alternative to a shell command hook.
// Either Command (shell) or Callback (Go func) is set per HookDefinition.
type HookCallback func(input map[string]any) (*HookResult, error)
