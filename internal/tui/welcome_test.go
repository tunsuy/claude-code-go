package tui

import (
	"strings"
	"testing"
)

func TestWelcomeHeader_IsShown(t *testing.T) {
	h := NewWelcomeHeader("claude-sonnet-4", "/tmp/test")
	if h.IsShown() {
		t.Error("expected new WelcomeHeader to not be shown")
	}

	h = h.MarkShown()
	if !h.IsShown() {
		t.Error("expected WelcomeHeader to be shown after MarkShown")
	}
}

func TestWelcomeHeader_View(t *testing.T) {
	h := NewWelcomeHeader("claude-sonnet-4", "/home/user/project")

	// Before shown, View should return content
	view := h.View(80, DefaultDarkTheme)
	if view == "" {
		t.Error("expected non-empty view before shown")
	}

	// Should contain version
	if !strings.Contains(view, "Claude Code") {
		t.Error("expected 'Claude Code' in view")
	}

	// After shown, View should return empty
	h = h.MarkShown()
	view = h.View(80, DefaultDarkTheme)
	if view != "" {
		t.Errorf("expected empty view after shown, got: %q", view)
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/testuser/projects/app", "~/projects/app"},
		{"/home/testuser/projects/app", "~/projects/app"},
		{"/tmp/test", "/tmp/test"},
		{"/var/log", "/var/log"},
	}

	for _, tc := range tests {
		got := shortenPath(tc.input)
		if got != tc.expected {
			t.Errorf("shortenPath(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestRenderLogo(t *testing.T) {
	logo := renderLogo(DefaultDarkTheme)
	if logo == "" {
		t.Error("expected non-empty logo")
	}
	// Logo should have multiple lines
	if !strings.Contains(logo, "\n") {
		t.Error("expected logo to have multiple lines")
	}
}

func TestInputView_HasPrompt(t *testing.T) {
	input := NewInput(false)
	view := input.View(DefaultDarkTheme)

	// View should start with > prompt
	if !strings.Contains(view, ">") {
		t.Error("expected input view to contain '>' prompt")
	}
}
