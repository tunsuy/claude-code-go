package coordinator

import (
	"context"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// Compile-time assertion: coordinatorAdapter implements tools.AgentCoordinator.
var _ tools.AgentCoordinator = (*coordinatorAdapter)(nil)

// coordinatorAdapter wraps a coordinatorImpl to satisfy tools.AgentCoordinator.
// This adapter bridges the coordinator's typed AgentID to the string-based
// interface expected by tools, avoiding a dependency cycle.
type coordinatorAdapter struct {
	impl *coordinatorImpl
}

// NewAgentCoordinator returns a tools.AgentCoordinator backed by the given Coordinator.
// Returns nil if c is nil or not a *coordinatorImpl.
func NewAgentCoordinator(c Coordinator) tools.AgentCoordinator {
	impl, ok := c.(*coordinatorImpl)
	if !ok || impl == nil {
		return nil
	}
	return &coordinatorAdapter{impl: impl}
}

// SpawnAgent launches a new sub-agent and returns its ID as a string.
func (a *coordinatorAdapter) SpawnAgent(ctx context.Context, req tools.AgentSpawnRequest) (string, error) {
	agentID, err := a.impl.SpawnAgent(ctx, SpawnRequest{
		Description:  req.Description,
		Prompt:       req.Prompt,
		AllowedTools: req.AllowedTools,
		MaxTurns:     req.MaxTurns,
		SubagentType: "worker",
	})
	if err != nil {
		return "", err
	}
	return string(agentID), nil
}

// SendMessage delivers a follow-up message to a running sub-agent.
func (a *coordinatorAdapter) SendMessage(ctx context.Context, agentID string, message string) error {
	return a.impl.SendMessage(ctx, AgentID(agentID), message)
}

// StopAgent stops a running sub-agent.
func (a *coordinatorAdapter) StopAgent(ctx context.Context, agentID string) error {
	return a.impl.StopAgent(ctx, AgentID(agentID))
}

// GetAgentStatus returns the current lifecycle state of a sub-agent as a string.
func (a *coordinatorAdapter) GetAgentStatus(ctx context.Context, agentID string) (string, error) {
	status, err := a.impl.GetAgentStatus(ctx, AgentID(agentID))
	if err != nil {
		return "", err
	}
	return string(status), nil
}

// GetAgentResult returns the final result and status of a sub-agent.
func (a *coordinatorAdapter) GetAgentResult(_ context.Context, agentID string) (string, string, error) {
	a.impl.mu.RLock()
	entry, ok := a.impl.agents[AgentID(agentID)]
	a.impl.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("coordinator: unknown agent %s", agentID)
	}

	entry.mu.Lock()
	status := entry.status
	result := entry.result
	entry.mu.Unlock()

	return result, string(status), nil
}

// ListAgents returns all agent IDs with their statuses as strings.
func (a *coordinatorAdapter) ListAgents() map[string]string {
	typed := a.impl.ListAgents()
	out := make(map[string]string, len(typed))
	for id, status := range typed {
		out[string(id)] = string(status)
	}
	return out
}

// WaitForAgent blocks until the specified agent finishes (or ctx is cancelled).
// Returns the agent's final result text and any error.
func (a *coordinatorAdapter) WaitForAgent(ctx context.Context, agentID string) (string, error) {
	// Subscribe to completion notification.
	ch, err := a.impl.Subscribe(AgentID(agentID))
	if err != nil {
		return "", err
	}

	select {
	case notif, ok := <-ch:
		if !ok {
			// Channel closed without notification — agent already done, fetch result.
			a.impl.mu.RLock()
			entry, exists := a.impl.agents[AgentID(agentID)]
			a.impl.mu.RUnlock()
			if !exists {
				return "", fmt.Errorf("coordinator: unknown agent %s", agentID)
			}
			entry.mu.Lock()
			result := entry.result
			agentErr := entry.err
			entry.mu.Unlock()
			if agentErr != nil {
				return "", agentErr
			}
			return result, nil
		}
		if notif.Status == AgentStatusFailed {
			return "", fmt.Errorf("agent %s failed: %s", agentID, notif.Summary)
		}
		return notif.Result, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
