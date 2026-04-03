// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
package types

import (
	"fmt"
	"regexp"
)

// SessionId is a branded string type for session identifiers.
// It prevents accidental mixing with plain strings, mirroring the TypeScript branded type.
type SessionId string

// AgentId is a branded string type for agent identifiers.
// Format: ^a(?:.+-)?[0-9a-f]{16}$
type AgentId string

var agentIdPattern = regexp.MustCompile(`^a(?:.+-)?[0-9a-f]{16}$`)

// AsSessionId converts an arbitrary string to a SessionId (no validation,
// matching the TypeScript asSessionId() semantic).
func AsSessionId(s string) SessionId { return SessionId(s) }

// AsAgentId converts a string to an AgentId, returning an error if the
// format does not match ^a(?:.+-)?[0-9a-f]{16}$.
func AsAgentId(s string) (AgentId, error) {
	if !agentIdPattern.MatchString(s) {
		return "", fmt.Errorf("invalid AgentId format: %q", s)
	}
	return AgentId(s), nil
}

// NOTE: NewAgentId() is intentionally NOT defined here.
// ID generation lives exclusively in pkg/utils/ids to keep pkg/types free of
// crypto/rand and other side-effectful dependencies.
// Use pkg/utils/ids.NewAgentId() / pkg/utils/ids.NewSessionId() instead.
