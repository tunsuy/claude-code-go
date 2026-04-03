// Package permission provides rule-matching helpers for the permission system.
package permission

import (
	"path/filepath"
	"strings"
)

// MatchToolRule reports whether toolName matches the given rule pattern.
// Patterns support shell glob syntax (e.g., "Bash*", "Read*").
// An exact match is also accepted.
func MatchToolRule(toolName, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	// Exact match first (fast path)
	if toolName == pattern {
		return true
	}
	// Glob match via filepath.Match (shell wildcard semantics)
	matched, err := filepath.Match(pattern, toolName)
	return err == nil && matched
}

// MatchPathRule reports whether path is covered by the given pathRule.
// pathRule may contain a trailing "**" wildcard or a plain prefix.
func MatchPathRule(path, pathRule string) bool {
	if pathRule == "" || pathRule == "*" || pathRule == "**" {
		return true
	}
	// Normalise separators for cross-platform safety
	path = filepath.ToSlash(path)
	pathRule = filepath.ToSlash(pathRule)

	// Support trailing "**" — match any subpath
	if strings.HasSuffix(pathRule, "/**") {
		prefix := strings.TrimSuffix(pathRule, "/**")
		return strings.HasPrefix(path, prefix+"/") || path == prefix
	}
	// Prefix match
	if strings.HasPrefix(path, pathRule) {
		return true
	}
	// Glob match
	matched, err := filepath.Match(pathRule, path)
	return err == nil && matched
}
