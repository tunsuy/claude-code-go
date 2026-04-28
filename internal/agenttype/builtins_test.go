package agenttype

import (
	"testing"
)

func TestRegisterBuiltins(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)

	// Should have exactly 5 built-in types.
	if r.Len() != 5 {
		t.Errorf("Len() = %d, want 5", r.Len())
	}

	expectedTypes := []AgentType{
		AgentTypeWorker,
		AgentTypeExplore,
		AgentTypePlan,
		AgentTypeVerify,
		AgentTypeGuide,
	}

	for _, at := range expectedTypes {
		t.Run(string(at), func(t *testing.T) {
			p, ok := r.Get(at)
			if !ok {
				t.Fatalf("built-in type %q not registered", at)
			}
			if p.DisplayName == "" {
				t.Error("DisplayName is empty")
			}
			if p.Description == "" {
				t.Error("Description is empty")
			}
			if p.SystemPrompt == "" {
				t.Error("SystemPrompt is empty")
			}
			if p.ToolFilter.Mode == "" {
				t.Error("ToolFilter.Mode is empty")
			}
			if len(p.ToolFilter.Tools) == 0 {
				t.Error("ToolFilter.Tools is empty")
			}
		})
	}
}

func TestBuiltin_WorkerProfile(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)
	p := r.MustGet(AgentTypeWorker)

	if p.ToolFilter.Mode != ToolFilterDenylist {
		t.Errorf("Worker ToolFilter.Mode = %q, want %q", p.ToolFilter.Mode, ToolFilterDenylist)
	}
	if p.CanSpawnSubAgents {
		t.Error("Worker should not be able to spawn sub-agents")
	}
}

func TestBuiltin_ExploreProfile(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)
	p := r.MustGet(AgentTypeExplore)

	if p.ToolFilter.Mode != ToolFilterAllowlist {
		t.Errorf("Explore ToolFilter.Mode = %q, want %q", p.ToolFilter.Mode, ToolFilterAllowlist)
	}

	// Explore should have Read, Glob, Grep, Bash.
	allowed := make(map[string]bool)
	for _, tool := range p.ToolFilter.Tools {
		allowed[tool] = true
	}
	for _, need := range []string{"Read", "Glob", "Grep", "Bash"} {
		if !allowed[need] {
			t.Errorf("Explore missing tool %q", need)
		}
	}
}

func TestBuiltin_NoneCanSpawnSubAgents(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)

	for _, p := range r.All() {
		if p.CanSpawnSubAgents {
			t.Errorf("built-in type %q should not be able to spawn sub-agents", p.Type)
		}
	}
}
