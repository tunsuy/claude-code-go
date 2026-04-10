package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/pkg/types"
	utilfs "github.com/anthropics/claude-code-go/pkg/utils/fs"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// writeSessionJSONL creates a valid JSONL session file under
// <projectDir>/.claude/projects/<hash>/<sessionID>.jsonl and
// returns the session ID string (stem of the file).
func writeSessionJSONL(t *testing.T, projectDir, sessionID string, messages []types.Message) {
	t.Helper()

	hash, err := utilfs.ProjectHash(projectDir)
	if err != nil {
		t.Fatalf("writeSessionJSONL: project hash: %v", err)
	}
	dir := filepath.Join(projectDir, ".claude", "projects", hash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("writeSessionJSONL: mkdir: %v", err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("writeSessionJSONL: create: %v", err)
	}
	defer f.Close()

	for _, msg := range messages {
		entryType := types.EntryTypeUser
		if msg.Role == types.RoleAssistant {
			entryType = types.EntryTypeAssistant
		}
		sm := types.SerializedMessage{
			Message:   msg,
			CWD:       projectDir,
			SessionId: types.AsSessionId(sessionID),
			Timestamp: time.Now(),
		}
		// We need to encode the envelope with the message fields merged.
		// The JSONL line must have "type" at top level AND the SerializedMessage fields.
		// Build a merged map.
		smBytes, err := json.Marshal(sm)
		if err != nil {
			t.Fatalf("writeSessionJSONL: marshal sm: %v", err)
		}
		var merged map[string]any
		if err := json.Unmarshal(smBytes, &merged); err != nil {
			t.Fatalf("writeSessionJSONL: unmarshal sm: %v", err)
		}
		merged["type"] = string(entryType)

		line, err := json.Marshal(merged)
		if err != nil {
			t.Fatalf("writeSessionJSONL: marshal merged: %v", err)
		}
		f.Write(append(line, '\n')) //nolint:errcheck
	}
}

// sampleMessages returns a simple two-turn conversation for test fixtures.
func sampleMessages() []types.Message {
	return []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hello")},
			},
		},
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hi there")},
			},
		},
	}
}

// ── loadSessionMessages ───────────────────────────────────────────────────────

// TestLoadSessionMessages_NeitherFlag verifies that when neither --resume
// nor --continue is set, loadSessionMessages returns nil without error.
func TestLoadSessionMessages_NeitherFlag(t *testing.T) {
	cwd := t.TempDir()
	f := &rootFlags{}

	msgs, err := loadSessionMessages(cwd, f)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if msgs != nil {
		t.Fatalf("expected nil messages, got %v", msgs)
	}
}

// TestLoadSessionMessages_Resume_NotFound verifies that --resume with an
// unknown session ID returns a non-nil error with a friendly message.
func TestLoadSessionMessages_Resume_NotFound(t *testing.T) {
	cwd := t.TempDir()
	f := &rootFlags{resume: "nonexistent-session-id"}

	_, err := loadSessionMessages(cwd, f)
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}

// TestLoadSessionMessages_Resume_Happy verifies that --resume with a valid
// session ID loads and returns the correct messages.
func TestLoadSessionMessages_Resume_Happy(t *testing.T) {
	cwd := t.TempDir()
	sessionID := "test-session-abc123"
	want := sampleMessages()
	writeSessionJSONL(t, cwd, sessionID, want)

	f := &rootFlags{resume: sessionID}
	got, err := loadSessionMessages(cwd, f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d messages, got %d", len(want), len(got))
	}
	for i, m := range got {
		if m.Role != want[i].Role {
			t.Errorf("msg[%d] role: got %q, want %q", i, m.Role, want[i].Role)
		}
	}
}

// TestLoadSessionMessages_Continue_NoSessions verifies that --continue
// returns a friendly error when there are no sessions.
func TestLoadSessionMessages_Continue_NoSessions(t *testing.T) {
	cwd := t.TempDir()
	f := &rootFlags{continueSession: true}

	_, err := loadSessionMessages(cwd, f)
	if err == nil {
		t.Fatal("expected error when no sessions exist, got nil")
	}
}

// TestLoadSessionMessages_Continue_Happy verifies that --continue loads the
// most-recently-modified session.
func TestLoadSessionMessages_Continue_Happy(t *testing.T) {
	cwd := t.TempDir()
	olderID := "older-session-001"
	newerID := "newer-session-002"

	olderMsgs := []types.Message{{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("old")}},
	}}
	newerMsgs := []types.Message{{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("new")}},
	}}

	writeSessionJSONL(t, cwd, olderID, olderMsgs)

	// Touch the newer file after the older one to ensure its mtime is later.
	writeSessionJSONL(t, cwd, newerID, newerMsgs)

	// Explicitly set different mtimes to avoid sub-millisecond races.
	hash, _ := utilfs.ProjectHash(cwd)
	sessDir := filepath.Join(cwd, ".claude", "projects", hash)
	olderPath := filepath.Join(sessDir, olderID+".jsonl")
	newerPath := filepath.Join(sessDir, newerID+".jsonl")

	now := time.Now()
	os.Chtimes(olderPath, now.Add(-10*time.Minute), now.Add(-10*time.Minute)) //nolint:errcheck
	os.Chtimes(newerPath, now, now)                                            //nolint:errcheck

	f := &rootFlags{continueSession: true}
	got, err := loadSessionMessages(cwd, f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one message")
	}
	// The first message should be from the newer session ("new").
	text := ""
	if got[0].Content != nil && len(got[0].Content) > 0 && got[0].Content[0].Text != nil {
		text = *got[0].Content[0].Text
	}
	if text != "new" {
		t.Errorf("expected message from newer session (\"new\"), got %q", text)
	}
}

// TestLoadSessionMessages_Continue_EmptyDir verifies that --continue on a
// project dir whose .claude/projects/<hash> directory has no .jsonl files
// returns a friendly error.
func TestLoadSessionMessages_Continue_EmptyDir(t *testing.T) {
	cwd := t.TempDir()

	// Create the sessions dir but leave it empty.
	hash, _ := utilfs.ProjectHash(cwd)
	sessDir := filepath.Join(cwd, ".claude", "projects", hash)
	os.MkdirAll(sessDir, 0o755) //nolint:errcheck

	f := &rootFlags{continueSession: true}
	_, err := loadSessionMessages(cwd, f)
	if err == nil {
		t.Fatal("expected error for empty session directory, got nil")
	}
}
