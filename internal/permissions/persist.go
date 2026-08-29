package permissions

import (
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/config"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// PersistConfig wires the checker to the on-disk settings that hold its rules.
// ProjectDir is the directory whose .claude/settings.json receives persisted
// rules; if empty, persistence falls back to in-memory updates only.
type PersistConfig struct {
	ProjectDir string
}

// ruleForTool builds the settings.json permission rule for a tool call.
// File-oriented tools get a path rule ("Read(/dir/**)"); everything else gets
// a bare tool-name rule ("WebFetch").
func ruleForTool(toolName string, input tools.Input, registry interface {
	Get(name string) (tools.Tool, bool)
}) string {
	if registry != nil {
		if t, ok := registry.Get(toolName); ok {
			if pt, ok := t.(tools.PathTool); ok {
				if p := pt.GetPath(input); p != "" {
					return fmt.Sprintf("%s(%s/**)", toolName, p)
				}
			}
		}
	}
	return toolName
}

// persistAllowRule writes rule to the project settings' permissions.allow
// (atomic write-back, idempotent) and registers it in the checker's in-memory
// allow rules so subsequent identical calls short-circuit to Allow without
// asking. Returns the rule that was registered.
func (c *checker) persistAllowRule(rule string) string {
	c.mu.Lock()
	if c.permCtx.AlwaysAllowRules == nil {
		c.permCtx.AlwaysAllowRules = make(types.ToolPermissionRulesBySource)
	}
	c.permCtx.AlwaysAllowRules[types.RuleSourceProject] = uniqueStrings(
		c.permCtx.AlwaysAllowRules[types.RuleSourceProject], rule)
	c.mu.Unlock()

	if c.getProjectDir() != "" {
		// Persistence failure must not block the already-approved tool call —
		// the rule still applies for this session via the in-memory update.
		_ = config.AddPermissionRule(c.getProjectDir(), config.PermissionListAllow, rule)
	}
	return rule
}

// SetProjectDir sets the persistence target directory (project root whose
// .claude/settings.json receives always-allow rules).
func (c *checker) SetProjectDir(dir string) {
	c.mu.Lock()
	c.persist.ProjectDir = dir
	c.mu.Unlock()
}

// getProjectDir returns the current persistence target directory.
func (c *checker) getProjectDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persist.ProjectDir
}

// uniqueStrings appends s to rules if not already present (order-preserving).
func uniqueStrings(rules []string, s string) []string {
	for _, r := range rules {
		if r == s {
			return rules
		}
	}
	return append(rules, s)
}
