package tools

import (
	"fmt"
	"sync"
)

// Registry holds all registered tools and provides thread-safe access.
// Tools are stored by their canonical name plus any aliases.
//
// Thread-safety:
//   - Register / Deregister / Replace acquire the write lock.
//   - Get / All / Filter acquire the read lock.
//   - Concurrent reads are safe and do not block each other.
//
// NOTE: DefaultRegistry is a package-level convenience for CLI entry points
// and tests. Application code should always inject *Registry explicitly via
// dependency injection; never rely on DefaultRegistry in library code.
type Registry struct {
	mu      sync.RWMutex
	tools   map[string]Tool // canonical name → Tool
	aliases map[string]Tool // alias → Tool (may overlap with tools)
	order   []string        // insertion order of canonical names
}

// NewRegistry returns an empty, ready-to-use Registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:   make(map[string]Tool),
		aliases: make(map[string]Tool),
	}
}

// DefaultRegistry is the global default registry.
// It is provided for CLI entry-point convenience only.
// Library code must use an explicitly injected *Registry.
var DefaultRegistry = NewRegistry()

// Register adds t to the registry under t.Name() and t.Aliases().
// Panics if the canonical name is already registered (startup-time misuse).
// For MCP reconnect-style re-registration use Replace instead.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tool.Registry: tool %q already registered; use Replace for re-registration", name))
	}
	r.tools[name] = t
	r.aliases[name] = t
	r.order = append(r.order, name)

	for _, alias := range t.Aliases() {
		r.aliases[alias] = t
	}
}

// Deregister removes the tool with the given canonical name from the registry.
// It also removes all of the tool's aliases.
// Returns an error if no tool with that name is registered.
//
// Thread-safety: callers must ensure that no in-flight Call for this tool is
// still executing when Deregister is called (e.g. drain active requests first).
func (r *Registry) Deregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, exists := r.tools[name]
	if !exists {
		return fmt.Errorf("tool.Registry: tool %q is not registered", name)
	}

	delete(r.tools, name)
	delete(r.aliases, name)

	for _, alias := range t.Aliases() {
		delete(r.aliases, alias)
	}

	// Remove from ordered slice.
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return nil
}

// Replace atomically replaces an existing tool registration with a new tool
// that has the same Name(). If no tool with that name is registered the new
// tool is simply registered (equivalent to Register but without the panic).
//
// Replace is the correct path for MCP tool re-registration after reconnect.
func (r *Registry) Replace(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()

	// Clean up old aliases if a previous registration exists.
	if old, exists := r.tools[name]; exists {
		for _, alias := range old.Aliases() {
			delete(r.aliases, alias)
		}
		// Keep order; just overwrite.
	} else {
		r.order = append(r.order, name)
	}

	r.tools[name] = t
	r.aliases[name] = t
	for _, alias := range t.Aliases() {
		r.aliases[alias] = t
	}
}

// Get looks up a tool by canonical name or alias.
// Returns (nil, false) if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.aliases[name]
	return t, ok
}

// All returns all enabled tools in registration order.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		if t.IsEnabled() {
			result = append(result, t)
		}
	}
	return result
}

// Filter returns the enabled subset of allowedNames, preserving registration
// order. If allowedNames is empty, All() is returned.
func (r *Registry) Filter(allowedNames []string) []Tool {
	if len(allowedNames) == 0 {
		return r.All()
	}

	allowed := make(map[string]bool, len(allowedNames))
	for _, n := range allowedNames {
		allowed[n] = true
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Tool
	for _, name := range r.order {
		if allowed[name] {
			if t := r.tools[name]; t.IsEnabled() {
				result = append(result, t)
			}
		}
	}
	return result
}

// Names returns the canonical names of all registered tools (including
// disabled ones), in registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}

// Len returns the total number of registered tools (enabled or not).
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}
