package agenttype

import (
	"fmt"
	"sync"
)

// Registry holds all known agent profiles (built-in + custom).
// It is safe for concurrent use.
type Registry struct {
	mu       sync.RWMutex
	profiles map[AgentType]*AgentProfile
	// nameIndex maps custom agent names to their type for reverse lookup.
	nameIndex map[string]AgentType
}

// NewRegistry creates a new empty agent profile registry.
func NewRegistry() *Registry {
	return &Registry{
		profiles:  make(map[AgentType]*AgentProfile),
		nameIndex: make(map[string]AgentType),
	}
}

// Register adds or replaces an agent profile in the registry.
// Returns an error if the profile has an empty Type.
func (r *Registry) Register(p *AgentProfile) error {
	if p.Type == "" {
		return fmt.Errorf("agenttype: profile Type must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profiles[p.Type] = p
	if p.DisplayName != "" {
		r.nameIndex[p.DisplayName] = p.Type
	}
	return nil
}

// Get returns the profile for the given type, or (nil, false) if not found.
func (r *Registry) Get(t AgentType) (*AgentProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[t]
	return p, ok
}

// MustGet returns the profile for the given type.
// Panics if the type is not registered.
func (r *Registry) MustGet(t AgentType) *AgentProfile {
	p, ok := r.Get(t)
	if !ok {
		panic(fmt.Sprintf("agenttype: unknown agent type %q", t))
	}
	return p
}

// Resolve attempts to find a profile by type or display name.
// Returns (nil, false) if neither matches.
func (r *Registry) Resolve(typeOrName string) (*AgentProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Direct type lookup.
	if p, ok := r.profiles[AgentType(typeOrName)]; ok {
		return p, true
	}
	// Name-based lookup.
	if t, ok := r.nameIndex[typeOrName]; ok {
		if p, ok := r.profiles[t]; ok {
			return p, true
		}
	}
	return nil, false
}

// All returns a snapshot of all registered profiles.
func (r *Registry) All() []*AgentProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*AgentProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		result = append(result, p)
	}
	return result
}

// Types returns all registered agent type keys.
func (r *Registry) Types() []AgentType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AgentType, 0, len(r.profiles))
	for t := range r.profiles {
		result = append(result, t)
	}
	return result
}

// Len returns the number of registered profiles.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.profiles)
}
