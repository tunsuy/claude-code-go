package memdir_test

import (
	"testing"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

func TestIsAutoMemoryEnabled(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "")

	enabled := true
	disabled := false

	tests := []struct {
		name     string
		bare     bool
		setting  *bool
		env      string
		expected bool
	}{
		{name: "default enabled", expected: true},
		{name: "bare mode", bare: true, expected: false},
		{name: "settings disabled", setting: &disabled, expected: false},
		{name: "settings enabled", setting: &enabled, expected: true},
		{name: "env disables", env: "1", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", tc.env)
			got := memdir.IsAutoMemoryEnabled(tc.bare, tc.setting)
			if got != tc.expected {
				t.Errorf("IsAutoMemoryEnabled() = %v, want %v", got, tc.expected)
			}
		})
	}
}
