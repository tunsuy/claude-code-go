// Package tui implements the BubbleTea TUI layer.
package tui

import (
	"strings"
	"time"
)

// AgentStatus represents the run state of a sub-agent.
type AgentStatus int

const (
	AgentRunning   AgentStatus = iota
	AgentPaused
	AgentCompleted
	AgentFailed
)

func (s AgentStatus) String() string {
	switch s {
	case AgentRunning:
		return "Running"
	case AgentPaused:
		return "Paused"
	case AgentCompleted:
		return "Done"
	case AgentFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// AgentTaskState mirrors TS LocalAgentTaskState.
type AgentTaskState struct {
	ID           string
	Name         string
	AgentType    string     // agent type key (e.g. "worker", "explore")
	Description  string     // human-readable task description
	Status       AgentStatus
	StartTime    time.Time
	ElapsedMs    int64
	OutputTokens int
	Activity     string     // current activity label, e.g. "Streaming", "Running Bash"
	Detail       string     // one-line detail of the current activity
	EvictAfter   *time.Time // nil = never auto-hide
	Color        string     // hex color for this agent
}

// CoordinatorPanel renders the sub-agent status panel.
type CoordinatorPanel struct {
	Tasks         map[string]AgentTaskState
	SelectedIndex int
	TaskOrder     []string // stable ordering
}

// View renders the coordinator panel.
func (p CoordinatorPanel) View(width int, theme Theme) string {
	if len(p.Tasks) == 0 {
		return ""
	}

	border := "─"
	title := mutedStyle(theme).Render("Coordinator Panel")
	sep := strings.Repeat(border, max0(width-2))

	var sb strings.Builder
	sb.WriteString(title + "\n")
	sb.WriteString(mutedStyle(theme).Render(sep) + "\n")

	for i, id := range p.TaskOrder {
		task, ok := p.Tasks[id]
		if !ok {
			continue
		}
		statusIcon := agentStatusIcon(task.Status)
		elapsed := ""
		if !task.StartTime.IsZero() {
			dur := time.Duration(task.ElapsedMs) * time.Millisecond
			elapsed = formatDuration(dur)
		}
		tok := ""
		if task.OutputTokens > 0 {
			tok = formatTokenCount(task.OutputTokens)
		}

		line := statusIcon + " " + agentTypeBadge(task.AgentType) + " " + task.Name
		right := task.Status.String() + "  " + elapsed + "  " + tok
		gap := width - lipglossWidth(line) - lipglossWidth(right) - 2
		if gap < 1 {
			gap = 1
		}
		line = line + strings.Repeat(" ", gap) + right

		if i == p.SelectedIndex {
			line = primaryStyle(theme).Bold(true).Render(line)
		} else {
			line = mutedStyle(theme).Render(line)
		}
		sb.WriteString(line + "\n")

		// Render activity detail line for running tasks.
		if task.Status == AgentRunning && task.Activity != "" {
			detail := "  ↳ " + task.Activity
			if task.Detail != "" {
				detail += ": " + truncateStr(task.Detail, width-10)
			}
			sb.WriteString(mutedStyle(theme).Render(detail) + "\n")
		}
	}

	sb.WriteString(mutedStyle(theme).Render(sep) + "\n")
	sb.WriteString(mutedStyle(theme).Render("↑↓ navigate · Enter view · x dismiss · Esc back"))

	return sb.String()
}

func agentStatusIcon(s AgentStatus) string {
	switch s {
	case AgentRunning:
		return "●"
	case AgentPaused:
		return "⏸"
	case AgentCompleted:
		return "✓"
	case AgentFailed:
		return "✗"
	default:
		return "○"
	}
}

// agentTypeBadge returns a short badge for the agent type.
func agentTypeBadge(t string) string {
	switch t {
	case "explore":
		return "[E]"
	case "plan":
		return "[P]"
	case "verify":
		return "[V]"
	case "guide":
		return "[G]"
	case "worker":
		return "[W]"
	default:
		if t != "" {
			// Custom agent type — show first letter uppercase.
			r := []rune(t)
			if len(r) > 0 {
				return "[" + string(r[0]) + "]"
			}
		}
		return "[W]"
	}
}

func formatTokenCount(n int) string {
	if n >= 1000 {
		return itoa(n/1000) + "." + itoa((n%1000)/100) + "k tok"
	}
	return itoa(n) + " tok"
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// lipglossWidth wraps lipgloss.Width to avoid an import cycle in this file.
func lipglossWidth(s string) int {
	// Count visible characters (naïve — ANSI escape stripped).
	// For accurate measurement we rely on lipgloss in styles.go; use it here.
	return len([]rune(stripANSI(s)))
}

// truncateStr truncates s to maxLen runes, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

// stripANSI strips ANSI escape sequences from s.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
