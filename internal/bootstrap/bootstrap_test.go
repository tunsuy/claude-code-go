package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/agenttype"
)

// TestMain sets up the test environment.
// We redirect HOME and XDG_CONFIG_HOME to a temp dir so no real
// keychain / token files are touched during testing.
func TestMain(m *testing.M) {
	// Use a fresh temp dir as the home directory for all tests so that
	// OAuth token stores and config files are isolated.
	tmpHome, err := os.MkdirTemp("", "bootstrap-test-home-*")
	if err != nil {
		panic("bootstrap_test: create tmp home: " + err.Error())
	}
	defer os.RemoveAll(tmpHome)

	os.Setenv("HOME", tmpHome)
	os.Setenv("XDG_CONFIG_HOME", tmpHome)
	// Ensure no real API key bleeds into tests.
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_MODEL")
	os.Unsetenv("CLAUDE_MODEL")

	os.Exit(m.Run())
}

// TestRunAuthStatus_NotAuthenticated_ReturnsError verifies that
// runAuthStatus returns a non-nil error when there is no OAuth token
// and no ANTHROPIC_API_KEY in the environment.
func TestRunAuthStatus_NotAuthenticated_ReturnsError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	err := runAuthStatus()
	if err == nil {
		t.Fatal("expected non-nil error when not authenticated, got nil")
	}
}

// TestBuildContainer_MissingConfig_Succeeds verifies that BuildContainer
// succeeds even when there are no config files or API key present.
// An empty API key is acceptable at construction time; auth errors surface
// only on the first real API call.
func TestBuildContainer_MissingConfig_Succeeds(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	opts := ContainerOptions{
		HomeDir:    t.TempDir(),
		WorkingDir: t.TempDir(),
	}

	container, err := BuildContainer(opts)
	if err != nil {
		t.Fatalf("BuildContainer returned unexpected error: %v", err)
	}
	if container == nil {
		t.Fatal("BuildContainer returned nil container, expected non-nil")
	}
}

// TestResolveModel_Override verifies that passing a non-empty override
// returns that override, ignoring any settings (including nil settings).
func TestResolveModel_Override(t *testing.T) {
	const want = "claude-3-haiku"
	got := resolveModel(nil, want)
	if got != want {
		t.Errorf("resolveModel(nil, %q) = %q, want %q", want, got, want)
	}
}

func TestLoadCustomAgentProfiles_UserAndProjectOverride(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()

	userAgents := filepath.Join(home, ".claude", "agents")
	projectAgents := filepath.Join(work, ".claude", "agents")
	if err := os.MkdirAll(userAgents, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectAgents, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(userAgents, "reviewer.json"), []byte(`{
		"name": "reviewer",
		"display_name": "Reviewer",
		"description": "user reviewer",
		"system_prompt": "user prompt"
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectAgents, "reviewer.json"), []byte(`{
		"name": "reviewer",
		"display_name": "Reviewer",
		"description": "project reviewer",
		"system_prompt": "project prompt"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	reg := agenttype.NewRegistry()
	loadCustomAgentProfiles(reg, home, work)

	profile, ok := reg.Get(agenttype.AgentType("reviewer"))
	if !ok {
		t.Fatal("expected reviewer profile")
	}
	if profile.Description != "project reviewer" {
		t.Errorf("project profile should override user profile, got %q", profile.Description)
	}
	if profile.SystemPrompt != "project prompt" {
		t.Errorf("SystemPrompt = %q", profile.SystemPrompt)
	}
}

// TestCollectHeadlessPrompt_FromArgs verifies that positional CLI arguments
// are joined with a single space and returned without error.
func TestCollectHeadlessPrompt_FromArgs(t *testing.T) {
	args := []string{"hello", "world"}
	got, err := collectHeadlessPrompt(args)
	if err != nil {
		t.Fatalf("collectHeadlessPrompt returned unexpected error: %v", err)
	}
	const want = "hello world"
	if got != want {
		t.Errorf("collectHeadlessPrompt(%v) = %q, want %q", args, got, want)
	}
}
