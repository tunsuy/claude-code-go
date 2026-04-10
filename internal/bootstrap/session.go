package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/pkg/types"
	utilfs "github.com/anthropics/claude-code-go/pkg/utils/fs"
)

// loadSessionMessages loads conversation history from a previous session
// according to the --resume / --continue flags.
// Returns (nil, nil) when neither flag is active (no-op).
// Returns a friendly error when the requested session cannot be found.
func loadSessionMessages(cwd string, f *rootFlags) ([]types.Message, error) {
	switch {
	case f.resume != "":
		return resumeSessionByID(f.resume, cwd)
	case f.continueSession:
		return continueMostRecentSession(cwd)
	}
	return nil, nil
}

// resumeSessionByID opens the session identified by sessionID inside cwd
// and returns its ordered conversation messages.
func resumeSessionByID(sessionID, cwd string) ([]types.Message, error) {
	_, mgr, err := session.Resume(sessionID, cwd)
	if err != nil {
		return nil, fmt.Errorf("session %q not found (cwd: %s).\n"+
			"Tip: use --continue to resume the most recent session", sessionID, cwd)
	}
	defer mgr.Close() //nolint:errcheck

	msgs, err := extractMessages(mgr)
	if err != nil {
		return nil, fmt.Errorf("resume session %q: %w", sessionID, err)
	}
	return msgs, nil
}

// continueMostRecentSession discovers the most-recently-modified JSONL session
// file for the current project directory and loads its messages.
func continueMostRecentSession(cwd string) ([]types.Message, error) {
	hash, err := utilfs.ProjectHash(cwd)
	if err != nil {
		return nil, fmt.Errorf("compute project hash: %w", err)
	}

	sessDir := filepath.Join(cwd, ".claude", "projects", hash)
	dirEntries, err := os.ReadDir(sessDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no previous sessions found in %s.\n"+
				"Start a new session to create one", cwd)
		}
		return nil, fmt.Errorf("read session directory: %w", err)
	}

	type candidate struct {
		sessionID string
		modTime   int64
	}
	var candidates []candidate

	for _, e := range dirEntries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		stem := e.Name()[:len(e.Name())-len(".jsonl")]
		candidates = append(candidates, candidate{sessionID: stem, modTime: info.ModTime().UnixNano()})
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no previous sessions found in %s.\n"+
			"Start a new session to create one", cwd)
	}

	// Sort descending by modification time — most recent first.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})

	return resumeSessionByID(candidates[0].sessionID, cwd)
}

// extractMessages reads all JSONL entries from mgr and returns the
// conversation messages (user + assistant roles) in order.
// Corrupt / unknown entry types are silently skipped.
func extractMessages(mgr *session.SessionManager) ([]types.Message, error) {
	entries, err := mgr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read session entries: %w", err)
	}

	var msgs []types.Message
	for _, env := range entries {
		if env.Type != types.EntryTypeUser && env.Type != types.EntryTypeAssistant {
			continue
		}
		var sm types.SerializedMessage
		if err := json.Unmarshal(env.Raw, &sm); err != nil {
			// Skip corrupt entries (already logged at WARN level by SessionStore).
			continue
		}
		msgs = append(msgs, sm.Message)
	}
	return msgs, nil
}
