package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// TestSessionStore_AppendAndReadAll verifies round-trip serialisation: entries
// written with AppendEntry must be returned verbatim by ReadAll.
func TestSessionStore_AppendAndReadAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	ss, err := session.OpenSessionStore(path)
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	defer ss.Close()

	type testEntry struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	e1 := testEntry{Type: "user", Data: "hello"}
	e2 := testEntry{Type: "assistant", Data: "world"}

	if err := ss.AppendEntry(e1); err != nil {
		t.Fatalf("AppendEntry(e1): %v", err)
	}
	if err := ss.AppendEntry(e2); err != nil {
		t.Fatalf("AppendEntry(e2): %v", err)
	}

	entries, err := ss.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Verify Raw bytes are a copy (RC-5 / M-1): mutating them must not affect
	// subsequent ReadAll calls.
	raw0 := entries[0].Raw
	raw0[0] = 'X' // corrupt first byte in the copy
	entries2, err := ss.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll (2nd call): %v", err)
	}
	if len(entries2) != 2 {
		t.Fatalf("expected 2 entries on 2nd read, got %d", len(entries2))
	}
	// Verify original type is still valid JSON.
	var env types.EntryEnvelope
	if err := json.Unmarshal(entries2[0].Raw, &env); err != nil {
		t.Errorf("entries2[0].Raw corrupted: %v", err)
	}
}

// TestSessionStore_ReadAll_EmptyFile verifies that reading a newly created
// (empty) session file returns no entries and no error.
func TestSessionStore_ReadAll_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")

	ss, err := session.OpenSessionStore(path)
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	defer ss.Close()

	entries, err := ss.ReadAll()
	if err != nil {
		t.Fatalf("expected nil error for empty file, got: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty file, got %d", len(entries))
	}
}

// TestSessionStore_CorruptLineSkipped verifies N-9: a corrupt JSONL line must
// not cause ReadAll to return an error; it is silently skipped.
func TestSessionStore_CorruptLineSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.jsonl")

	// Write two valid lines and one corrupt line.
	content := `{"type":"user"}` + "\n" +
		`NOT_VALID_JSON` + "\n" +
		`{"type":"assistant"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ss, err := session.OpenSessionStore(path)
	if err != nil {
		t.Fatalf("OpenSessionStore: %v", err)
	}
	defer ss.Close()

	entries, err := ss.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll must succeed even with corrupt line: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(entries))
	}
}

// TestNew_And_Resume verifies the session lifecycle: New creates a session,
// Resume reopens it and can read back entries written by the original session.
func TestNew_And_Resume(t *testing.T) {
	projectDir := t.TempDir()

	sid, mgr, err := session.New(projectDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if sid == "" {
		t.Fatal("New returned empty SessionId")
	}

	type payload struct {
		Type string `json:"type"`
		Msg  string `json:"msg"`
	}
	if err := mgr.AppendEntry(payload{Type: "user", Msg: "hi"}); err != nil {
		t.Fatalf("AppendEntry: %v", err)
	}
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Resume the session.
	sid2, mgr2, err := session.Resume(string(sid), projectDir)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	defer mgr2.Close()

	if sid2 != sid {
		t.Errorf("Resume returned different SessionId: %q vs %q", sid2, sid)
	}

	entries, err := mgr2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll after Resume: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after Resume, got %d", len(entries))
	}
}

// TestResume_MissingSession verifies that Resume returns an error for a
// nonexistent session ID.
func TestResume_MissingSession(t *testing.T) {
	projectDir := t.TempDir()
	_, _, err := session.Resume("nonexistent-session-id", projectDir)
	if err == nil {
		t.Fatal("expected error resuming nonexistent session")
	}
}
