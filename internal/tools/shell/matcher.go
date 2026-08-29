package shell

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// PreparePermissionMatcher returns a matcher that tests whether a permission
// rule pattern covers this Bash input's command. This is what makes settings
// rules like "Bash(git *)" or "Bash(go test:*)" effective: the permission
// checker extracts the content pattern (the part inside the parentheses) and
// calls the returned matcher against it.
func (t *bashTool) PreparePermissionMatcher(input tools.Input) (func(string) bool, error) {
	var in BashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, nil
	}
	return func(pattern string) bool {
		return MatchCommandRule(pattern, in.Command)
	}, nil
}

// MatchCommandRule reports whether command is covered by a permission rule
// content pattern. Supported pattern forms:
//
//	"git *"       — glob; '*' matches any run of characters (including spaces
//	                 and '/'), so it matches "git status" and
//	                 "git push origin main" but not "gits"
//	"go test:*"   — prefix form; matches the bare prefix or the prefix
//	                 followed by more arguments, so it matches "go test" and
//	                 "go test ./..." but not "go tests"
//	"ls"          — exact match; matches only "ls"
//
// For compound or piped commands, every sub-command and pipeline stage must
// individually match the pattern — a rule may not be satisfied by only part of
// a command line.
func MatchCommandRule(pattern, command string) bool {
	pattern = strings.TrimSpace(pattern)
	command = strings.TrimSpace(command)
	if pattern == "" || command == "" {
		return false
	}

	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		if prefix == "" {
			return true // ":*" covers everything
		}
		for _, stage := range commandStages(command) {
			if !matchesPrefix(stage, prefix) {
				return false
			}
		}
		return true
	}

	if strings.Contains(pattern, "*") {
		// Glob: '*' matches any characters, including spaces and separators.
		regex := strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*")
		re := regexp.MustCompile("^" + regex + "$")
		for _, stage := range commandStages(command) {
			if !re.MatchString(stage) {
				return false
			}
		}
		return true
	}

	// Exact match.
	for _, stage := range commandStages(command) {
		if stage != pattern {
			return false
		}
	}
	return true
}

// matchesPrefix reports whether stage equals prefix or continues with another
// argument after prefix (so "go test:*" matches "go test ./..." but not
// "go tests").
func matchesPrefix(stage, prefix string) bool {
	if stage == prefix {
		return true
	}
	return strings.HasPrefix(stage, prefix+" ")
}

// commandStages splits command into every unit that must individually satisfy
// a rule: each sub-command of a compound line (split on &&, ||, ;) and each
// pipeline stage within it (split on |).
func commandStages(command string) []string {
	var stages []string
	for _, segment := range splitCompoundCommand(command) {
		stages = append(stages, splitPipeline(segment)...)
	}
	return stages
}
