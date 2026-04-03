package bootstrap

import (
	"os"
	"testing"
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
