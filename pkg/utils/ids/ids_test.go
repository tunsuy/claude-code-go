package ids_test

import (
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/pkg/types"
	"github.com/tunsuy/claude-code-go/pkg/utils/ids"
)

// TestNewSessionId verifies that generated session IDs are unique and
// contain a timestamp-looking prefix.
func TestNewSessionId(t *testing.T) {
	a := ids.NewSessionId()
	b := ids.NewSessionId()
	if a == b {
		t.Error("NewSessionId should not return the same ID twice")
	}
	if string(a) == "" {
		t.Error("NewSessionId returned empty string")
	}
}

// TestNewAgentId_NoPrefix verifies the format ^a[0-9a-f]{16}$.
func TestNewAgentId_NoPrefix(t *testing.T) {
	id := ids.NewAgentId("")
	s := string(id)
	if !strings.HasPrefix(s, "a") {
		t.Errorf("expected prefix 'a', got %q", s)
	}
	// Validate via AsAgentId round-trip.
	if _, err := types.AsAgentId(s); err != nil {
		t.Errorf("generated AgentId %q failed AsAgentId validation: %v", s, err)
	}
}

// TestNewAgentId_WithPrefix verifies the format ^a<prefix>-[0-9a-f]{16}$.
func TestNewAgentId_WithPrefix(t *testing.T) {
	id := ids.NewAgentId("worker")
	s := string(id)
	if !strings.HasPrefix(s, "aworker-") {
		t.Errorf("expected prefix 'aworker-', got %q", s)
	}
	if _, err := types.AsAgentId(s); err != nil {
		t.Errorf("generated AgentId %q failed AsAgentId validation: %v", s, err)
	}
}

// TestNewAgentId_Unique verifies IDs are unique across calls.
func TestNewAgentId_Unique(t *testing.T) {
	seen := make(map[types.AgentId]struct{})
	for i := 0; i < 50; i++ {
		id := ids.NewAgentId("")
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate AgentId generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}
