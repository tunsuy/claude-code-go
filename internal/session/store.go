// Package session provides JSONL-based session persistence (read/write) and
// session lifecycle management (new, resume, cleanup).
package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"github.com/tunsuy/claude-code-go/pkg/types"
	"github.com/tunsuy/claude-code-go/pkg/utils/ids"
	utilfs "github.com/tunsuy/claude-code-go/pkg/utils/fs"
)

// ---------------------------------------------------------------------------
// SessionStorer interface (defined here for use by the session package itself
// and re-used by callers that need a mockable interface for testing).
// ---------------------------------------------------------------------------

// SessionStorer is the interface for reading and writing session entries.
// Concrete types that implement it: *SessionStore.
type SessionStorer interface {
	AppendEntry(entry any) error
	ReadAll() ([]types.EntryEnvelope, error)
	Close() error
}

// ---------------------------------------------------------------------------
// SessionStore — single-session JSONL file reader/writer
// ---------------------------------------------------------------------------

// SessionStore handles JSONL file I/O for a single session.
type SessionStore struct {
	path string
	mu   sync.Mutex
	file *os.File
}

// OpenSessionStore opens (or creates) the JSONL session file at path.
func OpenSessionStore(path string) (*SessionStore, error) {
	if err := utilfs.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("session: ensure dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("session: open %s: %w", path, err)
	}
	return &SessionStore{path: path, file: f}, nil
}

// AppendEntry serialises entry as a JSON line and appends it to the JSONL file.
func (s *SessionStore) AppendEntry(entry any) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("session: marshal entry: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.file.Write(append(line, '\n'))
	return err
}

// ReadAll reads and parses all entries from the JSONL file.
// Corrupt lines are skipped with a structured WARN log (they do not cause an error).
// Each returned EntryEnvelope has its Raw field populated with a copy of the
// original line bytes (safe to retain after the call returns).
func (s *SessionStore) ReadAll() ([]types.EntryEnvelope, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("session: open %s: %w", s.path, err)
	}
	defer f.Close()

	var entries []types.EntryEnvelope
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MiB line buffer

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		// scanner.Bytes() reuses an internal buffer — copy immediately (RC-5 / M-1).
		raw := string(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var env types.EntryEnvelope
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			// Log corrupt lines as WARN; do not propagate as an error (N-9).
			// Do NOT log raw line content — it may contain PII (§5.8 of review).
			slog.Warn("session: skipping corrupt entry",
				"file", s.path,
				"line", lineNum,
				"error", err,
			)
			continue
		}
		env.Raw = json.RawMessage(raw)
		entries = append(entries, env)
	}
	return entries, scanner.Err()
}

// Close closes the underlying file descriptor.
func (s *SessionStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Close()
}

// ---------------------------------------------------------------------------
// SessionManager — session lifecycle
// ---------------------------------------------------------------------------

// SessionManager manages the lifecycle of a single chat session.
type SessionManager struct {
	SessionId  types.SessionId
	store      *SessionStore
	projectDir string
}

// New creates a new session under projectDir and returns the manager.
func New(projectDir string) (types.SessionId, *SessionManager, error) {
	sid := ids.NewSessionId()
	storePath, err := sessionPath(projectDir, sid)
	if err != nil {
		return "", nil, err
	}
	ss, err := OpenSessionStore(storePath)
	if err != nil {
		return "", nil, err
	}
	mgr := &SessionManager{SessionId: sid, store: ss, projectDir: projectDir}
	return sid, mgr, nil
}

// Resume reopens an existing session by its ID.
func Resume(sessionIDStr, projectDir string) (types.SessionId, *SessionManager, error) {
	sid := types.AsSessionId(sessionIDStr)
	storePath, err := sessionPath(projectDir, sid)
	if err != nil {
		return "", nil, err
	}
	// Verify the file exists.
	if _, err := os.Stat(storePath); err != nil {
		return "", nil, fmt.Errorf("session: resume %s: %w", sessionIDStr, err)
	}
	ss, err := OpenSessionStore(storePath)
	if err != nil {
		return "", nil, err
	}
	mgr := &SessionManager{SessionId: sid, store: ss, projectDir: projectDir}
	return sid, mgr, nil
}

// AppendEntry delegates to the underlying SessionStore.
func (m *SessionManager) AppendEntry(entry any) error {
	return m.store.AppendEntry(entry)
}

// ReadAll delegates to the underlying SessionStore.
func (m *SessionManager) ReadAll() ([]types.EntryEnvelope, error) {
	return m.store.ReadAll()
}

// Close closes the session store.
func (m *SessionManager) Close() error {
	return m.store.Close()
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// sessionPath returns the JSONL file path for the given project + session.
// Path: <homeDir?>/.claude/projects/<projectHash>/<sessionId>.jsonl
// For simplicity, the path is rooted under projectDir/.claude/projects/ here.
// The canonical global path (under ~/ ) is constructed in bootstrap.
func sessionPath(projectDir string, sid types.SessionId) (string, error) {
	hash, err := utilfs.ProjectHash(projectDir)
	if err != nil {
		return "", fmt.Errorf("session: project hash: %w", err)
	}
	dir := filepath.Join(projectDir, ".claude", "projects", hash)
	return filepath.Join(dir, string(sid)+".jsonl"), nil
}
