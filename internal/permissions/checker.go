// Package permissions implements the tool-use permission decision pipeline.
// The pipeline evaluates a layered chain of rules (bypass → deny → validate →
// hook → allow → ask → tool-specific → mode default) and produces a three-level
// decision: Allow, Deny, or Ask.
package permissions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/tunsuy/claude-code-go/internal/hooks"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// Checker is the top-level permission pipeline interface.
type Checker interface {
	// CanUseTool performs a non-interactive permission check for a tool call.
	// Returns Allow, Deny, or Ask; never blocks waiting for user input.
	CanUseTool(ctx context.Context, toolName string, input tools.Input, tctx *tools.UseContext) (tools.PermissionResult, error)

	// RequestPermission runs the full permission flow:
	//   1. CanUseTool
	//   2. If Ask → send AskRequest on askCh and wait for AskResponse
	//   3. Record decision in denial state
	RequestPermission(ctx context.Context, req PermissionRequest, tctx *tools.UseContext) (tools.PermissionResult, error)

	// GetDenialCount returns the number of permission denials recorded in this
	// session (used for automatic prompting-mode downgrade).
	GetDenialCount() int
}

// PermissionRequest bundles the parameters for a full permission flow.
type PermissionRequest struct {
	ToolName  string
	ToolUseID string
	Input     tools.Input
	// ToolResult is the preliminary result from tools.CheckPermissions (may be
	// PermissionPassthrough if the tool deferred).
	ToolResult tools.PermissionResult
}

// checker is the concrete Checker implementation.
type checker struct {
	permCtx types.ToolPermissionContext
	// mu guards permCtx and persist: persistAllowRule mutates the allow rules
	// while CanUseTool reads them from the engine goroutine.
	mu         sync.RWMutex
	persist    PersistConfig
	dispatcher *hooks.Dispatcher
	// t is the resolved tool (may be nil if registry is nil).
	registry interface {
		Get(name string) (tools.Tool, bool)
	}
	askCh  chan<- AskRequest
	respCh <-chan AskResponse
	denial *DenialTrackingState
}

// CheckerConfig is the constructor config for NewChecker.
type CheckerConfig struct {
	// PermCtx is the current permission context snapshot.
	PermCtx types.ToolPermissionContext
	// Dispatcher runs PreToolUse hooks (may be nil to skip hooks).
	Dispatcher *hooks.Dispatcher
	// Registry resolves tool names to Tool implementations (may be nil).
	Registry interface {
		Get(name string) (tools.Tool, bool)
	}
	// AskCh receives AskRequest events from the permission system.
	// If nil, Ask decisions are downgraded to Deny.
	AskCh chan<- AskRequest
	// RespCh is used to receive user responses to AskRequests.
	// If nil, Ask decisions are downgraded to Deny.
	RespCh <-chan AskResponse
	// Persist optionally configures always-allow rule persistence to the
	// project's .claude/settings.json. If nil, persistence is disabled and
	// always-allow responses only update the in-memory rules.
	Persist *PersistConfig
}

// NewChecker constructs a new Checker.
func NewChecker(cfg CheckerConfig) Checker {
	c := &checker{
		permCtx:    cfg.PermCtx,
		dispatcher: cfg.Dispatcher,
		registry:   cfg.Registry,
		askCh:      cfg.AskCh,
		respCh:     cfg.RespCh,
		denial:     &DenialTrackingState{},
	}
	if cfg.Persist != nil {
		c.persist = *cfg.Persist
	}
	return c
}

// CanUseTool implements the permission decision chain without user interaction.
func (c *checker) CanUseTool(
	ctx context.Context,
	toolName string,
	input tools.Input,
	tctx *tools.UseContext,
) (tools.PermissionResult, error) {
	// Snapshot read: persistAllowRule may concurrently append allow rules from
	// a user response while the engine goroutine runs this check.
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 1. bypassPermissions mode → unconditional allow.
	if c.permCtx.Mode == types.PermissionModeBypassPermissions {
		return tools.PermissionResult{Behavior: tools.PermissionAllow}, nil
	}

	// 2. alwaysDenyRules → deny.
	if matched, reason := matchRules(c.permCtx.AlwaysDenyRules, toolName, input, c.resolveMatcherFn(toolName, input)); matched {
		return tools.PermissionResult{
			Behavior: tools.PermissionDeny,
			Reason:   fmt.Sprintf("tool %q is in the always-deny list: %s", toolName, reason),
		}, nil
	}

	// 3. ValidateInput (via tool).
	if c.registry != nil {
		if t, ok := c.registry.Get(toolName); ok {
			vr, verr := t.ValidateInput(input, tctx)
			if verr != nil {
				return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: verr.Error()}, nil
			}
			if !vr.OK {
				return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: vr.Reason}, nil
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
			return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: reason}, nil
		}
	}

	// 5. alwaysAllowRules → allow.
	if matched, _ := matchRules(c.permCtx.AlwaysAllowRules, toolName, input, c.resolveMatcherFn(toolName, input)); matched {
		return tools.PermissionResult{Behavior: tools.PermissionAllow}, nil
	}

	// 6. alwaysAskRules → ask.
	if matched, _ := matchRules(c.permCtx.AlwaysAskRules, toolName, input, c.resolveMatcherFn(toolName, input)); matched {
		return tools.PermissionResult{Behavior: tools.PermissionAsk}, nil
	}

	// 7. PermissionMode default decisions.
	switch c.permCtx.Mode {
	case types.PermissionModeDontAsk:
		return tools.PermissionResult{Behavior: tools.PermissionAllow}, nil
	case types.PermissionModePlan:
		// In plan mode, deny all write operations; allow reads.
		if c.registry != nil {
			if t, ok := c.registry.Get(toolName); ok {
				if !t.IsReadOnly(input) {
					return tools.PermissionResult{
						Behavior: tools.PermissionDeny,
						Reason:   "write operations are not allowed in plan mode",
					}, nil
				}
			}
		}
		return tools.PermissionResult{Behavior: tools.PermissionAllow}, nil
	case types.PermissionModeAcceptEdits:
		// Allow all edit-type tools; ask for other write tools.
		if c.registry != nil {
			if t, ok := c.registry.Get(toolName); ok {
				if t.IsReadOnly(input) {
					return tools.PermissionResult{Behavior: tools.PermissionAllow}, nil
				}
			}
		}
		// Default to ask for non-read tools.
		return tools.PermissionResult{Behavior: tools.PermissionAsk}, nil
	}

	// 8. Tool-specific CheckPermissions.
	if c.registry != nil {
		if t, ok := c.registry.Get(toolName); ok {
			result, terr := t.CheckPermissions(input, tctx)
			if terr != nil {
				return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: terr.Error()}, nil
			}
			if result.Behavior != tools.PermissionPassthrough {
				return result, nil
			}
		}
	}

	// 9. Default mode: ask for write tools, allow read tools.
	if c.registry != nil {
		if t, ok := c.registry.Get(toolName); ok {
			if t.IsReadOnly(input) {
				return tools.PermissionResult{Behavior: tools.PermissionAllow}, nil
			}
		}
	}
	// Unknown or write tool in default mode → ask.
	return tools.PermissionResult{Behavior: tools.PermissionAsk}, nil
}

// RequestPermission implements the full interactive permission flow.
func (c *checker) RequestPermission(
	ctx context.Context,
	req PermissionRequest,
	tctx *tools.UseContext,
) (tools.PermissionResult, error) {
	result, err := c.CanUseTool(ctx, req.ToolName, req.Input, tctx)
	if err != nil {
		return result, err
	}

	switch result.Behavior {
	case tools.PermissionAllow:
		c.denial.RecordApproval()
		return result, nil

	case tools.PermissionDeny:
		// Denial-downgrade: when the session keeps hitting denials (consecutive
		// streak or accumulated total), stop auto-denying and ask the user
		// directly — the deny rules are fighting the session, not guarding it.
		if c.shouldFallbackToPrompting() && c.askCh != nil && c.respCh != nil {
			return c.askUser(ctx, req, "many denials in this session; confirm directly: "+result.Reason)
		}
		c.denial.Record(DenialRecord{
			ToolName:  req.ToolName,
			ToolUseID: req.ToolUseID,
			Reason:    result.Reason,
		})
		return result, nil

	case tools.PermissionAsk:
		// Forward the ask to the TUI layer.
		if c.askCh == nil || c.respCh == nil {
			// No UI available — deny.
			c.denial.Record(DenialRecord{
				ToolName:  req.ToolName,
				ToolUseID: req.ToolUseID,
				Reason:    "no interactive channel; ask downgraded to deny",
			})
			return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: result.Reason}, nil
		}

		return c.askUser(ctx, req, result.Reason)
	}

	return result, nil
}

// askUser forwards an AskRequest to the TUI layer and waits for the response.
// message is the human-readable explanation shown in the permission dialog.
func (c *checker) askUser(
	ctx context.Context,
	req PermissionRequest,
	message string,
) (tools.PermissionResult, error) {
	askReq := AskRequest{
		ID:          req.ToolUseID,
		ToolName:    req.ToolName,
		ToolUseID:   req.ToolUseID,
		Message:     message,
		Input:       req.Input,
		ProjectPath: getProjectPath(),
	}
	select {
	case c.askCh <- askReq:
	case <-ctx.Done():
		return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: "context cancelled"}, nil
	}

	// Wait for response (60-second deadline handled by the TUI side).
	select {
	case resp, ok := <-c.respCh:
		if !ok {
			return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: "response channel closed"}, nil
		}
		if resp.Decision == tools.PermissionAllow {
			if resp.AlwaysAllow {
				// "Yes, and always allow" — register a rule so subsequent
				// calls of the same shape are allowed without asking.
				c.persistAllowRule(ruleForTool(req.ToolName, req.Input, c.registry))
			}
			c.denial.RecordApproval()
			reason := "user approved"
			if resp.AlwaysAllow {
				reason = "user approved (always allowed)"
			}
			return tools.PermissionResult{Behavior: tools.PermissionAllow, Reason: reason}, nil
		}
		c.denial.Record(DenialRecord{
			ToolName:  req.ToolName,
			ToolUseID: req.ToolUseID,
			Reason:    "user denied",
		})
		return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: "user denied"}, nil
	case <-ctx.Done():
		return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: "context cancelled"}, nil
	}
}

// shouldFallbackToPrompting reports whether the denial-tracking state has hit
// a threshold that warrants downgrading auto-denials to user prompts.
func (c *checker) shouldFallbackToPrompting() bool {
	return c.denial.shouldFallbackToPrompting()
}

// GetDenialCount implements Checker.
func (c *checker) GetDenialCount() int {
	return c.denial.DenialCount
}

// resolveMatcherFn retrieves the tool's PreparePermissionMatcher result (if available).
// Returns nil if the tool is not found or returns no matcher.
func (c *checker) resolveMatcherFn(toolName string, input tools.Input) func(string) bool {
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
	input tools.Input,
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

// getProjectPath returns the current working directory as the project path.
func getProjectPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}
