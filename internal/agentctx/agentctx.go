// Package agentctx provides helpers for propagating AgentID through
// context.Context. This allows any code in the call chain to identify
// which agent is currently executing.
package agentctx

import "context"

type ctxKey struct{}

// WithAgentID returns a new context carrying the given agent ID.
func WithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, agentID)
}

// AgentID extracts the agent ID from ctx, or "" if none is set.
func AgentID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
