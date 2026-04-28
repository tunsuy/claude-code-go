package tui

import "sync"

// agentColorPalette is the set of colors assigned to agents round-robin.
var agentColorPalette = []string{
	"#FF6B6B", // red
	"#4ECDC4", // teal
	"#45B7D1", // blue
	"#96CEB4", // green
	"#FFEAA7", // yellow
	"#DDA0DD", // plum
	"#98D8C8", // mint
	"#F7DC6F", // gold
}

// AgentColorManager assigns stable colors to agent IDs using round-robin.
// It is safe for concurrent use.
type AgentColorManager struct {
	mu          sync.Mutex
	assignments map[string]string // agentID → color
	nextIndex   int
}

// NewAgentColorManager creates a new color manager.
func NewAgentColorManager() *AgentColorManager {
	return &AgentColorManager{
		assignments: make(map[string]string),
	}
}

// Assign returns the color for an agent, allocating one if needed.
// The same agent ID always gets the same color within a session.
func (m *AgentColorManager) Assign(agentID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if color, ok := m.assignments[agentID]; ok {
		return color
	}

	color := agentColorPalette[m.nextIndex%len(agentColorPalette)]
	m.assignments[agentID] = color
	m.nextIndex++
	return color
}

// Get returns the color for an agent, or "" if not yet assigned.
func (m *AgentColorManager) Get(agentID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.assignments[agentID]
}
