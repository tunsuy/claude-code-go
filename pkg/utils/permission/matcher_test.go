package permission_test

import (
	"testing"

	"github.com/tunsuy/claude-code-go/pkg/utils/permission"
)

func TestMatchToolRule(t *testing.T) {
	tests := []struct {
		toolName string
		pattern  string
		want     bool
	}{
		{"Bash", "", true},
		{"Bash", "*", true},
		{"Bash", "Bash", true},
		{"Bash", "bash", false}, // case-sensitive
		{"Bash", "Bash*", true},
		{"BashExec", "Bash*", true},
		{"Read", "Bash*", false},
		{"Read", "Read(*)", false}, // parens in pattern — no glob match
		{"Read(*)", "Read(*)", true},
	}
	for _, tt := range tests {
		got := permission.MatchToolRule(tt.toolName, tt.pattern)
		if got != tt.want {
			t.Errorf("MatchToolRule(%q, %q) = %v, want %v", tt.toolName, tt.pattern, got, tt.want)
		}
	}
}

func TestMatchPathRule(t *testing.T) {
	tests := []struct {
		path    string
		rule    string
		want    bool
	}{
		{"/tmp/foo", "", true},
		{"/tmp/foo", "*", true},
		{"/tmp/foo", "**", true},
		{"/tmp/foo/bar", "/tmp/**", true},
		{"/tmp", "/tmp/**", true},
		{"/other/foo", "/tmp/**", false},
		{"/tmp/file.go", "/tmp/file.go", true},
		{"/tmp/other.go", "/tmp/file.go", false},
		{"/tmp/file.go", "/tmp/*.go", true},
		{"/tmp/sub/file.go", "/tmp/*.go", false}, // glob does not cross dirs
	}
	for _, tt := range tests {
		got := permission.MatchPathRule(tt.path, tt.rule)
		if got != tt.want {
			t.Errorf("MatchPathRule(%q, %q) = %v, want %v", tt.path, tt.rule, got, tt.want)
		}
	}
}
