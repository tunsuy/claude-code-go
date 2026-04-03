// Package ids provides ID generation utilities for claude-code-go.
// All ID generation logic lives here; pkg/types only defines the branded types.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/anthropics/claude-code-go/pkg/types"
)

// NewSessionId generates a new SessionId using a millisecond timestamp
// concatenated with 8 random hex bytes.
func NewSessionId() types.SessionId {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand is unavailable — this is a non-recoverable condition.
		panic(fmt.Sprintf("claude-code-go: crypto/rand unavailable: %v", err))
	}
	return types.SessionId(fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b)))
}

// NewAgentId generates a new AgentId that satisfies ^a(?:.+-)?[0-9a-f]{16}$.
// prefix may be empty (produces "a<16hex>") or a non-empty label
// (produces "a<prefix>-<16hex>").
func NewAgentId(prefix string) types.AgentId {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("claude-code-go: crypto/rand unavailable: %v", err))
	}
	suffix := hex.EncodeToString(b) // 16 hex chars
	if prefix == "" {
		return types.AgentId("a" + suffix)
	}
	return types.AgentId(fmt.Sprintf("a%s-%s", prefix, suffix))
}
