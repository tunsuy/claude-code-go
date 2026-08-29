package shell

import (
	"fmt"
	"regexp"
	"strings"
)

// SecurityFinding describes a single dangerous pattern found in a command.
type SecurityFinding struct {
	// Pattern is the human-readable name of the matched dangerous pattern.
	Pattern string
	// Segment is the sub-command in which the pattern was found.
	Segment string
	// Reason explains why the pattern is dangerous.
	Reason string
}

// String renders the finding for permission-denial messages.
func (f SecurityFinding) String() string {
	return fmt.Sprintf("%s (matched in %q): %s", f.Pattern, truncate(f.Segment, 60), f.Reason)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// dangerousCommandPatterns matches pipeline stages that escalate privileges or
// execute untrusted/arbitrary code. Patterns are anchored to the start of a
// stage (after ; / && / || / | splitting), so a dangerous word appearing as a
// plain argument — e.g. `echo "sudo is a word"` — does not trigger a finding.
var dangerousCommandPatterns = []struct {
	pattern *regexp.Regexp
	name    string
	reason  string
}{
	{
		pattern: regexp.MustCompile(`^\s*sudo\b`),
		name:    "sudo",
		reason:  "privilege escalation via sudo is not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*eval\b`),
		name:    "eval",
		reason:  "eval executes dynamically-constructed strings and is not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*exec\b`),
		name:    "exec",
		reason:  "exec replaces the shell process and can bypass auditing",
	},
	{
		pattern: regexp.MustCompile(`^\s*su\b`),
		name:    "su",
		reason:  "switching users via su is not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*chmod\s+(-[a-zA-Z]*\s+)*777\b`),
		name:    "chmod 777",
		reason:  "world-writable permissions (chmod 777) are not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*chown\s+(-[a-zA-Z]*R|[a-zA-Z]*R[a-zA-Z]*)\b`),
		name:    "chown -R",
		reason:  "recursive ownership changes are not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*(shutdown|reboot|halt|poweroff)\b`),
		name:    "system power control",
		reason:  "shutting down or rebooting the system is not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*(mkfs(\.\w+)?|fdisk)\b`),
		name:    "disk formatting",
		reason:  "disk formatting or partitioning commands are not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*diskutil\s+eraseDisk\b`),
		name:    "disk formatting",
		reason:  "disk formatting or partitioning commands are not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*dd\b[^|]*of=/dev/`),
		name:    "dd to device",
		reason:  "writing raw data to device files is not allowed",
	},
	{
		pattern: regexp.MustCompile(`^\s*rm\s+(-[a-zA-Z]*[rf][a-zA-Z]*\s+)+`),
		name:    "rm -rf",
		reason:  "recursive/forced deletion is not allowed without explicit review",
	},
	{
		pattern: regexp.MustCompile(`^\s*(crontab\s+-[er]|launchctl\s+(load|bootstrap))\b`),
		name:    "persistence modification",
		reason:  "modifying scheduled jobs or launch agents is not allowed",
	},
}

// downloadCommands are commands whose output may be piped into a shell.
var downloadCommands = map[string]bool{"curl": true, "wget": true}

// shellStages are shells that a pipeline may pipe downloaded content into.
var shellStages = map[string]bool{"sh": true, "bash": true, "zsh": true}

// stageCommand returns the first word of a pipeline stage (the command name).
func stageCommand(stage string) string {
	fields := strings.Fields(stage)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// splitCompoundCommand splits a shell command on &&, ||, and ; separators
// (respecting quotes) so each sub-command can be checked independently.
func splitCompoundCommand(cmd string) []string {
	var (
		segments []string
		current  strings.Builder
		inSingle bool
		inDouble bool
	)
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(c)
		case c == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(c)
		case !inSingle && !inDouble:
			if c == ';' {
				segments = append(segments, current.String())
				current.Reset()
			} else if c == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
				segments = append(segments, current.String())
				current.Reset()
				i++
			} else if c == '|' && i+1 < len(cmd) && cmd[i+1] == '|' {
				segments = append(segments, current.String())
				current.Reset()
				i++
			} else {
				current.WriteByte(c)
			}
		default:
			current.WriteByte(c)
		}
	}
	segments = append(segments, current.String())

	out := make([]string, 0, len(segments))
	for _, s := range segments {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// splitPipeline splits a single sub-command on unquoted | into its pipeline
// stages. Callers must have already split on || via splitCompoundCommand.
func splitPipeline(cmd string) []string {
	var (
		stages   []string
		current  strings.Builder
		inSingle bool
		inDouble bool
	)
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(c)
		case c == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(c)
		case c == '|' && !inSingle && !inDouble:
			stages = append(stages, current.String())
			current.Reset()
		default:
			current.WriteByte(c)
		}
	}
	stages = append(stages, current.String())

	out := make([]string, 0, len(stages))
	for _, s := range stages {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// analyzeStage checks one pipeline stage against the anchored dangerous
// patterns.
func analyzeStage(stage string) []SecurityFinding {
	var findings []SecurityFinding
	for _, p := range dangerousCommandPatterns {
		if p.pattern.MatchString(stage) {
			findings = append(findings, SecurityFinding{
				Pattern: p.name,
				Segment: stage,
				Reason:  p.reason,
			})
		}
	}
	return findings
}

// analyzePipeline checks the stages of a single sub-command. In addition to
// per-stage patterns, it detects download commands (curl/wget) whose output
// is piped into a shell.
func analyzePipeline(segment string) []SecurityFinding {
	stages := splitPipeline(segment)
	var findings []SecurityFinding
	sawDownload := false
	downloadName := ""
	for _, stage := range stages {
		findings = append(findings, analyzeStage(stage)...)
		cmd := stageCommand(stage)
		if downloadCommands[cmd] {
			sawDownload = true
			downloadName = cmd
		} else if sawDownload && shellStages[cmd] {
			findings = append(findings, SecurityFinding{
				Pattern: downloadName + " | sh",
				Segment: segment,
				Reason:  "piping downloaded content into a shell is not allowed",
			})
			sawDownload = false
		}
	}
	return findings
}

// AnalyzeCommand checks a (possibly compound) shell command for dangerous
// patterns. Every sub-command produced by splitting on &&, ||, and ; is
// checked independently, and within each sub-command every pipeline stage is
// checked at command position. Returns all findings, empty when the command
// is safe.
func AnalyzeCommand(command string) []SecurityFinding {
	var findings []SecurityFinding
	for _, segment := range splitCompoundCommand(command) {
		findings = append(findings, analyzePipeline(segment)...)
	}
	return findings
}
