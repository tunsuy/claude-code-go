package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// BaseTool provides default implementations for the "optional zero-value"
// methods of the Tool interface. Embed *BaseTool (or BaseTool) in a concrete
// tool struct to avoid boilerplate.
//
// Methods that have meaningful domain defaults are covered here; methods that
// MUST be implemented by each tool (Name, Description, InputSchema, Call, …)
// are intentionally omitted so the compiler enforces them.
type BaseTool struct{}

// Aliases returns nil — most tools have no alternate names.
func (b BaseTool) Aliases() []string { return nil }

// Prompt returns ("", nil) — most tools add nothing to the system prompt.
func (b BaseTool) Prompt(_ context.Context, _ PermissionContext) (string, error) {
	return "", nil
}

// MaxResultSizeChars returns -1 (no limit).
func (b BaseTool) MaxResultSizeChars() int { return -1 }

// SearchHint returns "" — most tools don't need a custom search hint.
func (b BaseTool) SearchHint() string { return "" }

// IsDestructive returns false — most tools are not irreversible.
func (b BaseTool) IsDestructive(_ Input) bool { return false }

// IsEnabled returns true — tools are enabled by default.
func (b BaseTool) IsEnabled() bool { return true }

// InterruptBehavior returns Block — wait for the current operation to complete.
func (b BaseTool) InterruptBehavior() InterruptBehavior { return InterruptBehaviorBlock }

// ValidateInput returns OK=true — skip validation by default.
func (b BaseTool) ValidateInput(_ Input, _ *UseContext) (ValidationResult, error) {
	return ValidationResult{OK: true}, nil
}

// CheckPermissions returns Passthrough — delegate to the framework.
func (b BaseTool) CheckPermissions(_ Input, _ *UseContext) (PermissionResult, error) {
	return PermissionResult{Behavior: PermissionPassthrough}, nil
}

// PreparePermissionMatcher returns nil — no pattern matching by default.
func (b BaseTool) PreparePermissionMatcher(_ Input) (func(string) bool, error) {
	return nil, nil
}

// MapResultToToolResultBlock converts the output to a standard Anthropic
// tool_result content block. Concrete tools can override for custom formats.
func (b BaseTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	var contentStr string
	switch v := output.(type) {
	case string:
		contentStr = v
	case []byte:
		contentStr = string(v)
	default:
		raw, err := json.Marshal(output)
		if err != nil {
			return nil, fmt.Errorf("MapResultToToolResultBlock: marshal failed: %w", err)
		}
		contentStr = string(raw)
	}

	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     contentStr,
	}
	return json.Marshal(block)
}

// ToAutoClassifierInput returns "" — skip auto-classifier by default.
func (b BaseTool) ToAutoClassifierInput(_ Input) string { return "" }
