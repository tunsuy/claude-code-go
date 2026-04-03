// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
package types

// PermissionMode enumerates the available permission strategy modes.
type PermissionMode string

const (
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeAuto              PermissionMode = "auto"
	PermissionModeBubble            PermissionMode = "bubble"
)

// PermissionBehavior describes the outcome of a single permission decision.
type PermissionBehavior string

const (
	BehaviorAllow PermissionBehavior = "allow"
	BehaviorDeny  PermissionBehavior = "deny"
	BehaviorAsk   PermissionBehavior = "ask"
)

// PermissionRuleSource indicates which configuration layer a rule originated from.
type PermissionRuleSource string

const (
	RuleSourceUser    PermissionRuleSource = "userSettings"
	RuleSourceProject PermissionRuleSource = "projectSettings"
	RuleSourceLocal   PermissionRuleSource = "localSettings"
	RuleSourceCLI     PermissionRuleSource = "cliArg"
	RuleSourceSession PermissionRuleSource = "session"
	RuleSourceCommand PermissionRuleSource = "command"
	RuleSourcePolicy  PermissionRuleSource = "policySettings"
	RuleSourceFlag    PermissionRuleSource = "flagSettings"
)

// PermissionRuleValue describes the match target of a permission rule.
type PermissionRuleValue struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionRule is a concrete permission rule entry.
type PermissionRule struct {
	Source       PermissionRuleSource `json:"source"`
	RuleBehavior PermissionBehavior   `json:"ruleBehavior"`
	RuleValue    PermissionRuleValue  `json:"ruleValue"`
}

// PermissionDecision is the result of a permission check (discriminated union).
// The Behavior field distinguishes allow / ask / deny.
type PermissionDecision struct {
	Behavior PermissionBehavior `json:"behavior"`
	Message  string             `json:"message,omitempty"`
	// allow-specific
	UserModified bool `json:"userModified,omitempty"`
	// ask-specific
	Suggestions []PermissionUpdate `json:"suggestions,omitempty"`
	BlockedPath string             `json:"blockedPath,omitempty"`
}

// PermissionUpdateType is a typed string for the kind of permission config change.
type PermissionUpdateType string

const (
	PermissionUpdateAddRules         PermissionUpdateType = "addRules"
	PermissionUpdateReplaceRules     PermissionUpdateType = "replaceRules"
	PermissionUpdateRemoveRules      PermissionUpdateType = "removeRules"
	PermissionUpdateSetMode          PermissionUpdateType = "setMode"
	PermissionUpdateAddDirectories   PermissionUpdateType = "addDirectories"
	PermissionUpdateRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionUpdate represents a single permission configuration change operation.
type PermissionUpdate struct {
	Type        PermissionUpdateType  `json:"type"`
	Destination string                `json:"destination"`
	Rules       []PermissionRuleValue `json:"rules,omitempty"`
	Behavior    PermissionBehavior    `json:"behavior,omitempty"`
	Mode        PermissionMode        `json:"mode,omitempty"`
	Directories []string              `json:"directories,omitempty"`
}

// ToolPermissionRulesBySource is a set of tool permission rules grouped by source layer.
type ToolPermissionRulesBySource map[PermissionRuleSource][]string

// ToolPermissionContext is a read-only snapshot of the permission context for tool execution.
type ToolPermissionContext struct {
	Mode                             PermissionMode                         `json:"mode"`
	AdditionalWorkingDirectories     map[string]AdditionalWorkingDirectory  `json:"additionalWorkingDirectories,omitempty"`
	AlwaysAllowRules                 ToolPermissionRulesBySource            `json:"alwaysAllowRules,omitempty"`
	AlwaysDenyRules                  ToolPermissionRulesBySource            `json:"alwaysDenyRules,omitempty"`
	AlwaysAskRules                   ToolPermissionRulesBySource            `json:"alwaysAskRules,omitempty"`
	IsBypassPermissionsModeAvailable bool                                   `json:"isBypassPermissionsModeAvailable,omitempty"`
}

// AdditionalWorkingDirectory is an extra working directory within the permission scope.
type AdditionalWorkingDirectory struct {
	Path   string               `json:"path"`
	Source PermissionRuleSource `json:"source"`
}
