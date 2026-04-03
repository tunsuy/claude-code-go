// Package tui implements the BubbleTea TUI layer for claude-code-go.
package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds semantic colour roles for the TUI.
type Theme struct {
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Muted     lipgloss.Color
	Error     lipgloss.Color
	Warning   lipgloss.Color
	Success   lipgloss.Color

	// Code block rendering
	CodeBG lipgloss.Color
	CodeFG lipgloss.Color

	// Tool call display
	ToolName  lipgloss.Color
	ToolInput lipgloss.Color
}

// DefaultDarkTheme is the built-in dark theme.
var DefaultDarkTheme = Theme{
	Primary:   lipgloss.Color("12"),  // bright blue
	Secondary: lipgloss.Color("7"),   // white
	Accent:    lipgloss.Color("205"), // pink
	Muted:     lipgloss.Color("8"),   // dark gray
	Error:     lipgloss.Color("9"),
	Warning:   lipgloss.Color("11"),
	Success:   lipgloss.Color("10"),
	CodeBG:    lipgloss.Color("236"),
	CodeFG:    lipgloss.Color("252"),
	ToolName:  lipgloss.Color("14"),
	ToolInput: lipgloss.Color("7"),
}

// DefaultLightTheme is the built-in light theme.
var DefaultLightTheme = Theme{
	Primary:   lipgloss.Color("4"),  // blue
	Secondary: lipgloss.Color("0"),  // black
	Accent:    lipgloss.Color("5"),  // magenta
	Muted:     lipgloss.Color("8"),  // gray
	Error:     lipgloss.Color("1"),
	Warning:   lipgloss.Color("3"),
	Success:   lipgloss.Color("2"),
	CodeBG:    lipgloss.Color("254"),
	CodeFG:    lipgloss.Color("238"),
	ToolName:  lipgloss.Color("6"),
	ToolInput: lipgloss.Color("0"),
}

// TokyoNightTheme is the built-in Tokyo Night theme.
var TokyoNightTheme = Theme{
	Primary:   lipgloss.Color("111"), // blue
	Secondary: lipgloss.Color("189"), // light
	Accent:    lipgloss.Color("175"), // pink
	Muted:     lipgloss.Color("61"),  // muted purple
	Error:     lipgloss.Color("203"), // red
	Warning:   lipgloss.Color("215"), // orange
	Success:   lipgloss.Color("114"), // green
	CodeBG:    lipgloss.Color("237"),
	CodeFG:    lipgloss.Color("189"),
	ToolName:  lipgloss.Color("117"),
	ToolInput: lipgloss.Color("189"),
}

// BuiltinThemes maps theme names to Theme values.
var BuiltinThemes = map[string]Theme{
	"dark":        DefaultDarkTheme,
	"light":       DefaultLightTheme,
	"tokyo-night": TokyoNightTheme,
}
