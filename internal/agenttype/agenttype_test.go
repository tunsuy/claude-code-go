package agenttype

import (
	"testing"
)

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if r.Len() != 0 {
		t.Errorf("Len() = %d, want 0", r.Len())
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	profile := &AgentProfile{
		Type:        AgentTypeWorker,
		DisplayName: "Worker",
		Description: "test worker",
	}

	if err := r.Register(profile); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := r.Get(AgentTypeWorker)
	if !ok {
		t.Fatal("Get(worker) returned false")
	}
	if got.Description != "test worker" {
		t.Errorf("Description = %q, want %q", got.Description, "test worker")
	}
}

func TestRegistry_RegisterEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	err := r.Register(&AgentProfile{})
	if err == nil {
		t.Error("Register empty type should return error")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestRegistry_MustGetPanics(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGet should panic for unknown type")
		}
	}()
	r.MustGet("nonexistent")
}

func TestRegistry_Resolve(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_ = r.Register(&AgentProfile{
		Type:        AgentTypeExplore,
		DisplayName: "Explorer",
		Description: "explore agent",
	})

	tests := []struct {
		name     string
		input    string
		wantOK   bool
		wantType AgentType
	}{
		{"by type", "explore", true, AgentTypeExplore},
		{"by display name", "Explorer", true, AgentTypeExplore},
		{"not found", "unknown", false, ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := r.Resolve(tt.input)
			if ok != tt.wantOK {
				t.Errorf("Resolve(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && got.Type != tt.wantType {
				t.Errorf("Resolve(%q).Type = %v, want %v", tt.input, got.Type, tt.wantType)
			}
		})
	}
}

func TestRegistry_All(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(&AgentProfile{Type: AgentTypeWorker, DisplayName: "W"})
	_ = r.Register(&AgentProfile{Type: AgentTypeExplore, DisplayName: "E"})

	all := r.All()
	if len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}
}

func TestRegistry_Types(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(&AgentProfile{Type: AgentTypeWorker, DisplayName: "W"})
	_ = r.Register(&AgentProfile{Type: AgentTypePlan, DisplayName: "P"})

	types := r.Types()
	if len(types) != 2 {
		t.Errorf("Types() len = %d, want 2", len(types))
	}
}

func TestAgentType_IsBuiltin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		t    AgentType
		want bool
	}{
		{AgentTypeWorker, true},
		{AgentTypeExplore, true},
		{AgentTypePlan, true},
		{AgentTypeVerify, true},
		{AgentTypeGuide, true},
		{AgentTypeCustom, false},
		{AgentType("my-agent"), false},
	}
	for _, tt := range tests {
		if got := tt.t.IsBuiltin(); got != tt.want {
			t.Errorf("%q.IsBuiltin() = %v, want %v", tt.t, got, tt.want)
		}
	}
}

func TestAgentProfile_EffectiveModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model string
		want  string
	}{
		{"", ""},
		{"inherit", ""},
		{"haiku", "haiku"},
		{"claude-sonnet-4-20250514", "claude-sonnet-4-20250514"},
	}
	for _, tt := range tests {
		p := &AgentProfile{Model: tt.model}
		if got := p.EffectiveModel(); got != tt.want {
			t.Errorf("EffectiveModel(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}
