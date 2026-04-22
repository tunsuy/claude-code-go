package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// VimMode represents the current Vim editing mode.
type VimMode int

const (
	VimModeInsert VimMode = iota
	VimModeNormal
	VimModeVisual
)

// InputModel is the bottom prompt input, supporting normal and Vim key modes.
type InputModel struct {
	textarea  textarea.Model
	vimMode   VimMode
	vimEnabled bool
	// For normal-mode multi-key sequences.
	pendingKey string
}

// NewInput creates a new InputModel.
func NewInput(vimEnabled bool) InputModel {
	ta := textarea.New()
	ta.Placeholder = "Message Claude…"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Focus()

	return InputModel{
		textarea:   ta,
		vimMode:    VimModeInsert,
		vimEnabled: vimEnabled,
	}
}

// Value returns the current input text.
func (m InputModel) Value() string {
	return m.textarea.Value()
}

// SetValue sets the input text.
func (m InputModel) SetValue(s string) InputModel {
	m.textarea.SetValue(s)
	return m
}

// SetWidth sets the input width.
func (m InputModel) SetWidth(w int) InputModel {
	m.textarea.SetWidth(w)
	return m
}

// Focus gives focus to the input.
func (m InputModel) Focus() (InputModel, tea.Cmd) {
	cmd := m.textarea.Focus()
	return m, cmd
}

// Blur removes focus from the input.
func (m InputModel) Blur() InputModel {
	m.textarea.Blur()
	return m
}

// Init returns the textarea init command.
func (m InputModel) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles key messages for the input model.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if m.vimEnabled {
		return m.updateVim(msg)
	}
	return m.updateNormal(msg)
}

// updateNormal handles keys in non-Vim mode.
func (m InputModel) updateNormal(msg tea.Msg) (InputModel, tea.Cmd) {
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// updateVim handles keys in Vim mode.
func (m InputModel) updateVim(msg tea.Msg) (InputModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	switch m.vimMode {
	case VimModeInsert:
		return m.vimInsertKey(keyMsg)
	case VimModeNormal:
		return m.vimNormalKey(keyMsg)
	default:
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
}

func (m InputModel) vimInsertKey(key tea.KeyMsg) (InputModel, tea.Cmd) {
	if key.Type == tea.KeyEsc {
		m.vimMode = VimModeNormal
		m.pendingKey = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(key)
	return m, cmd
}

func (m InputModel) vimNormalKey(key tea.KeyMsg) (InputModel, tea.Cmd) {
	// Handle pending sequences first.
	if m.pendingKey != "" {
		seq := m.pendingKey + key.String()
		m.pendingKey = ""
		switch seq {
		case "dd":
			// Delete line — clear the whole input.
			m.textarea.SetValue("")
			return m, nil
		case "yy":
			// Yank line — no-op in TUI context.
			return m, nil
		}
		return m, nil
	}

	switch key.String() {
	case "i":
		m.vimMode = VimModeInsert
	case "a":
		m.vimMode = VimModeInsert
		// Move cursor right.
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRight})
	case "A":
		m.vimMode = VimModeInsert
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
	case "o":
		m.vimMode = VimModeInsert
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "h":
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyLeft})
	case "l":
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRight})
	case "k":
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyUp})
	case "j":
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyDown})
	case "0":
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyHome})
	case "$":
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
	case "d", "y":
		// Start two-key sequence.
		m.pendingKey = key.String()
	case "x":
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyDelete})
	case "p":
		// Paste — no-op without clipboard integration.
	}
	return m, nil
}

// View renders the input box.
func (m InputModel) View(theme Theme) string {
	var indicator string
	if m.vimEnabled {
		switch m.vimMode {
		case VimModeNormal:
			indicator = warningStyle(theme).Render("[NORMAL] ")
		case VimModeInsert:
			indicator = successStyle(theme).Render("[INSERT] ")
		case VimModeVisual:
			indicator = accentStyle(theme).Render("[VISUAL] ")
		}
	}
	// Add > prompt prefix to match original Claude Code style
	prompt := accentStyle(theme).Bold(true).Render("> ")
	return prompt + indicator + m.textarea.View()
}

// IsSlashCommand returns true when the current value starts with '/'.
func (m InputModel) IsSlashCommand() bool {
	return strings.HasPrefix(strings.TrimSpace(m.textarea.Value()), "/")
}

// SlashPrefix returns the current /command prefix for Tab-completion.
func (m InputModel) SlashPrefix() string {
	v := strings.TrimSpace(m.textarea.Value())
	if !strings.HasPrefix(v, "/") {
		return ""
	}
	// Return everything up to the first space.
	if idx := strings.Index(v, " "); idx >= 0 {
		return v[:idx]
	}
	return v
}
