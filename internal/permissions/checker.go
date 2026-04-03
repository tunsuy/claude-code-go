// Package permissions implements the tool-use permission decision pipeline.
// The pipeline evaluates a layered chain of rules (bypass → deny → validate →
// hook → allow → ask → tool-specific → mode default) and produces a three-level
// decision: Allow, Deny, or Ask.
package permissions

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/claude-code-go/internal/hooks"
	"github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// Checker is the top-level permission pipeline interface.
type Checker interface {
	// CanUseTool performs a non-interactive permission check for a tool call.
	// Returns Allow, Deny, or Ask; never blocks waiting for user input.
	CanUseTool(ctx context.Context, toolName string, input tool.Input, tctx *tool.UseContext) (tool.PermissionResult, error)

	// RequestPermission runs the full permission flow:
	//   1. CanUseTool
	//   2. If Ask → send AskRequest on askCh and wait for AskResponse
	//   3. Record decision in denial state
	RequestPermission(ctx context.Context, req PermissionRequest, tctx *tool.UseContext) (tool.PermissionResult, error)

	// GetDenialCount returns the number of permission denials recorded in this
	// session (used for automatic prompting-mode downgrade).
	GetDenialCount() int
}

// PermissionRequest bundles the parameters for a full permission flow.
type PermissionRequest struct {
	ToolName   string
	ToolUseID  string
	Input      tool.Input
	// ToolResult is the preliminary result from tool.CheckPermissions (may be
	// PermissionPassthrough if the tool deferred).
	ToolResult tool.PermissionResult
}

// checker is the concrete Checker implementation.
type checker struct {
	permCtx    types.ToolPermissionContext
	dispatcher *hooks.Dispatcher
	// t is the resolved tool (may be nil if registry is nil).
	registry   interface {
		Get(name string) (tool.Tool, bool)
	}
	askCh      chan<- AskRequest
	respCh     <-chan AskResponse
	denial     *DenialTrackingState
}

// CheckerConfig is the constructor config for NewChecker.
type CheckerConfig struct {
	// PermCtx is the current permission context snapshot.
	PermCtx types.ToolPermissionContext
	// Dispatcher runs PreToolUse hooks (may be nil to skip hooks).
	Dispatcher *hooks.Dispatcher
	// Registry resolves tool names to Tool implementations (may be nil).
	Registry interface {
		Get(name string) (tool.Tool, bool)
	}
	// AskCh receives AskRequest events from the permission system.
	// If nil, Ask decisions are downgraded to Deny.
	AskCh chan<- AskRequest
	// RespCh is used to receive user responses to AskRequests.
	// If nil, Ask decisions are downgraded to Deny.
	RespCh <-chan AskResponse
}

// NewChecker constructs a new Checker.
func NewChecker(cfg CheckerConfig) Checker {
	return &checker{
		permCtx:    cfg.PermCtx,
		dispatcher: cfg.Dispatcher,
		registry:   cfg.Registry,
		askCh:      cfg.AskCh,
		respCh:     cfg.RespCh,
		denial:     &DenialTrackingState{},
	}
}

// CanUseTool implements the permission decision chain without user interaction.
func (c *checker) CanUseTool(
	ctx context.Context,
	toolName string,
	input tool.Input,
	tctx *tool.UseContext,
) (tool.PermissionResult, error) {
	// 1. bypassPermissions mode → unconditional allow.
	if c.permCtx.Mode == types.PermissionModeBypassPermissions {
		return tool.PermissionResult{Behavior: tool.PermissionAllow}, nil
	}

	// 2. alwaysDenyRules → deny.
	if matched, reason := matchRules(c.permCtx.AlwaysDenyRules, toolName, input, c.resolveMatcherFn(toolName, input)); matched {
		return tool.PermissionResult{
			Behavior: tool.PermissionDeny,
			Reason:   fmt.Sprintf("tool %q is in the always-deny list: %s", toolName, reason),
		}, nil
	}

	// 3. ValidateInput (via tool).
	if c.registry != nil {
		if t, ok := c.registry.Get(toolName); ok {
			vr, verr := t.ValidateInput(input, tctx)
			if verr != nil {
				return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: verr.Error()}, nil
			}
			if !vr.OK {
				return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: vr.Reason}, nil
			}
		}
	}

	// 4. Hook PreToolUse.
	if c.dispatcher != nil {
		hookResult, herr := c.dispatcher.Run(ctx, types.HookPreToolUse, map[string]any{
			"tool_name": toolName,
			"input":     string(input),
		})
		if herr == nil && hookResult != nil && hookResult.Blocked {
			reason := "blocked by PreToolUse hook"
			if len(hookResult.BlockReasons) > 0 {
				reason = strings.Join(hookResult.BlockReasons, "; ")
			}
			return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: reason}, nil
		}
	}

	// 5. alwaysAllowRules → allow.
	if matched, _ := matchRules(c.permCtx.AlwaysAllowRules, toolName, input, c.resolveMatcherFn(toolName, input)); matched {
		return tool.PermissionResult{Behavior: tool.PermissionAllow}, nil
	}

	// 6. alwaysAskRules → ask.
	if matched, _ := matchRules(c.permCtx.AlwaysAskRules, toolName, input, c.resolveMatcherFn(toolName, input)); matched {
		return tool.PermissionResult{Behavior: tool.PermissionAsk}, nil
	}

	// 7. PermissionMode default decisions.
	switch c.permCtx.Mode {
	case types.PermissionModeDontAsk:
		return tool.PermissionResult{Behavior: tool.PermissionAllow}, nil
	case types.PermissionModePlan:
		// In plan mode, deny all write operations; allow reads.
		if c.registry != nil {
			if t, ok := c.registry.Get(toolName); ok {
				if !t.IsReadOnly(input) {
					return tool.PermissionResult{
						Behavior: tool.PermissionDeny,
						Reason:   "write operations are not allowed in plan mode",
					}, nil
				}
			}
		}
		return tool.PermissionResult{Behavior: tool.PermissionAllow}, nil
	case types.PermissionModeAcceptEdits:
		// Allow all edit-type tools; ask for other write tools.
		if c.registry != nil {
			if t, ok := c.registry.Get(toolName); ok {
				if t.IsReadOnly(input) {
					return tool.PermissionResult{Behavior: tool.PermissionAllow}, nil
				}
			}
		}
		// Default to ask for non-read tools.
		return tool.PermissionResult{Behavior: tool.PermissionAsk}, nil
	}

	// 8. Tool-specific CheckPermissions.
	if c.registry != nil {
		if t, ok := c.registry.Get(toolName); ok {
			result, terr := t.CheckPermissions(input, tctx)
			if terr != nil {
				return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: terr.Error()}, nil
			}
			if result.Behavior != tool.PermissionPassthrough {
				return result, nil
			}
		}
	}

	// 9. Default mode: ask for write tools, allow read tools.
	if c.registry != nil {
		if t, ok := c.registry.Get(toolName); ok {
			if t.IsReadOnly(input) {
				return tool.PermissionResult{Behavior: tool.PermissionAllow}, nil
			}
		}
	}
	// Unknown or write tool in default mode → ask.
	return tool.PermissionResult{Behavior: tool.PermissionAsk}, nil
}

// RequestPermission implements the full interactive permission flow.
func (c *checker) RequestPermission(
	ctx context.Context,
	req PermissionRequest,
	tctx *tool.UseContext,
) (tool.PermissionResult, error) {
	result, err := c.CanUseTool(ctx, req.ToolName, req.Input, tctx)
	if err != nil {
		return result, err
	}

	switch result.Behavior {
	case tool.PermissionAllow:
		return result, nil

	case tool.PermissionDeny:
		c.denial.Record(DenialRecord{
			ToolName:  req.ToolName,
			ToolUseID: req.ToolUseID,
			Reason:    result.Reason,
		})
		return result, nil

	case tool.PermissionAsk:
		// Forward the ask to the TUI layer.
		if c.askCh == nil || c.respCh == nil {
			// No UI available — deny.
			c.denial.Record(DenialRecord{
				ToolName:  req.ToolName,
				ToolUseID: req.ToolUseID,
				Reason:    "no interactive channel; ask downgraded to deny",
			})
			return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: result.Reason}, nil
		}

		askReq := AskRequest{
			ID:          req.ToolUseID,
			ToolName:    req.ToolName,
			ToolUseID:   req.ToolUseID,
			Message:     result.Reason,
			Input:       req.Input,
		}
		select {
		case c.askCh <- askReq:
		case <-ctx.Done():
			return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: "context cancelled"}, nil
		}

		// Wait for response (60-second deadline handled by the TUI side).
		select {
		case resp, ok := <-c.respCh:
			if !ok {
				return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: "response channel closed"}, nil
			}
			if resp.Decision == tool.PermissionAllow {
				return tool.PermissionResult{Behavior: tool.PermissionAllow, Reason: "user approved"}, nil
			}
			c.denial.Record(DenialRecord{
				ToolName:  req.ToolName,
				ToolUseID: req.ToolUseID,
				Reason:    "user denied",
			})
			return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: "user denied"}, nil
		case <-ctx.Done():
			return tool.PermissionResult{Behavior: tool.PermissionDeny, Reason: "context cancelled"}, nil
		}
	}

	return result, nil
}

// GetDenialCount implements Checker.
func (c *checker) GetDenialCount() int {
	return c.denial.DenialCount
}

// resolveMatcherFn retrieves the tool's PreparePermissionMatcher result (if available).
// Returns nil if the tool is not found or returns no matcher.
func (c *checker) resolveMatcherFn(toolName string, input tool.Input) func(string) bool {
	if c.registry == nil {
		return nil
	}
	t, ok := c.registry.Get(toolName)
	if !ok {
		return nil
	}
	fn, _ := t.PreparePermissionMatcher(input)
	return fn
}

// matchRules checks whether toolName + input match any of the rules in the map.
// matcherFn (if non-nil) is used for content-pattern matching; otherwise only
// tool-name matching is performed.
// Returns (matched, firstMatchedPattern).
func matchRules(
	rules types.ToolPermissionRulesBySource,
	toolName string,
	input tool.Input,
	matcherFn func(string) bool,
) (bool, string) {
	for _, patterns := range rules {
		for _, pattern := range patterns {
			if matchPattern(pattern, toolName, matcherFn) {
				return true, pattern
			}
		}
	}
	return false, ""
}

// matchPattern tests whether pattern covers toolName.
//
// Pattern formats:
//
//	"ToolName"           — exact tool-name match
//	"ToolName(*)"        — tool-name + any content (matcherFn required)
//	"ToolName(content)"  — tool-name + content glob (matcherFn required)
func matchPattern(pattern, toolName string, matcherFn func(string) bool) bool {
	// Split "ToolName(content)" or "ToolName".
	parenIdx := strings.Index(pattern, "(")
	if parenIdx < 0 {
		// Exact tool-name match.
		return strings.EqualFold(pattern, toolName)
	}

	// Extract tool-name prefix and content pattern.
	namePart := pattern[:parenIdx]
	if !strings.EqualFold(namePart, toolName) {
		return false
	}

	// Extract the content pattern (strip trailing ')').
	contentPattern := pattern[parenIdx+1:]
	if idx := strings.LastIndex(contentPattern, ")"); idx >= 0 {
		contentPattern = contentPattern[:idx]
	}

	if matcherFn != nil {
		return matcherFn(contentPattern)
	}

	// No matcher available — content patterns require a matcher; deny match.
	return false
}
