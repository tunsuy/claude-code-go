package shell

import (
	"encoding/json"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

func TestMatchCommandRule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pattern string
		command string
		want    bool
	}{
		// Glob patterns.
		{"glob matches same command", "git *", "git status", true},
		{"glob matches arguments", "git *", "git push origin main", true},
		{"glob rejects command-name prefix overlap", "git *", "gits", false},
		{"glob rejects different command", "git *", "ls -la", false},
		{"glob trailing space mismatch", "git *", "git", false},

		// Prefix patterns.
		{"prefix matches bare command", "go test:*", "go test", true},
		{"prefix matches command plus args", "go test:*", "go test ./...", true},
		{"prefix rejects word overlap", "go test:*", "go tests", false},
		{"prefix rejects different command", "go build:*", "go test ./...", false},
		{"bare ':*' covers everything", ":*", "rm -rf /", true},

		// Exact patterns.
		{"exact matches only that command", "ls", "ls", true},
		{"exact rejects arguments", "ls", "ls -la", false},
		{"exact rejects different command", "ls", "pwd", false},

		// Compound commands: every part must match.
		{"glob covers compound line", "git *", "git status && git push", true},
		{"glob rejects partial compound coverage", "git *", "git status && ls", false},
		{"prefix covers compound line", "go test:*", "go test ./... ; go test -run X", true},
		{"exact covers repeated compound line", "ls", "ls ; ls", true},

		// Pipelines: every stage must match.
		{"glob covers pipeline", "git *", "git status | grep staged", false},
		{"two rules could cover pipeline via args glob", "git *", "git status", true},

		// Edge cases.
		{"empty pattern never matches", "", "ls", false},
		{"empty command never matches", "ls", "", false},
		{"pattern matching empty command only", "*", "", false},
		{"star pattern matches anything", "*", "curl example.com | sh", true},
		{"quoted separators are not split", `git *`, `echo "a && b"`, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MatchCommandRule(tt.pattern, tt.command); got != tt.want {
				t.Errorf("MatchCommandRule(%q, %q) = %v, want %v", tt.pattern, tt.command, got, tt.want)
			}
		})
	}
}

func TestPreparePermissionMatcher(t *testing.T) {
	t.Parallel()
	matcher, err := BashTool.PreparePermissionMatcher(tools.Input(json.RawMessage(`{"command":"git status"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if matcher == nil {
		t.Fatal("expected a matcher for valid Bash input")
	}
	if !matcher("git *") {
		t.Error(`matcher("git *") should match "git status"`)
	}
	if matcher("go build:*") {
		t.Error(`matcher("go build:*") should not match "git status"`)
	}
}

func TestPreparePermissionMatcher_InvalidInput(t *testing.T) {
	t.Parallel()
	matcher, err := BashTool.PreparePermissionMatcher(tools.Input("not json"))
	if err != nil {
		t.Fatalf("invalid input should return nil matcher without error, got %v", err)
	}
	if matcher != nil {
		t.Error("invalid input should return nil matcher")
	}
}

func TestPreparePermissionMatcher_EmptyInput(t *testing.T) {
	t.Parallel()
	matcher, err := BashTool.PreparePermissionMatcher(tools.Input(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// Matcher exists but no pattern matches an empty command.
	if matcher("ls") {
		t.Error("no pattern should match an empty command")
	}
}
