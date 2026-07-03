package memdir

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

const (
	autoDreamQuerySource   = "auto_dream"
	consolidateLockName    = ".consolidate-lock"
	minHoursBetweenDream   = 24
	autoDreamMaxTurns      = 8
)

// AutoDreamConfig configures background memory consolidation.
type AutoDreamConfig struct {
	Store   *MemoryStore
	Enabled bool
}

// DefaultAutoDreamConfig returns the default auto-dream configuration.
func DefaultAutoDreamConfig() AutoDreamConfig {
	return AutoDreamConfig{Enabled: true}
}

// ExecuteAutoDream runs memory consolidation as a StopHook when gates pass.
func ExecuteAutoDream(ctx context.Context, hookCtx *engine.StopHookContext, cfg AutoDreamConfig) {
	if !cfg.Enabled || cfg.Store == nil || hookCtx == nil {
		return
	}
	if hookCtx.IsBareMode || hookCtx.QuerySource != foregroundQuerySource {
		return
	}
	if hookCtx.Engine == nil || hookCtx.CacheParams == nil {
		return
	}
	if !shouldRunAutoDream(cfg.Store.Dir()) {
		return
	}

	lockPath := filepath.Join(cfg.Store.Dir(), consolidateLockName)
	if err := touchConsolidateLock(lockPath); err != nil {
		log.Printf("memdir: auto dream lock: %v", err)
		return
	}

	prompt := buildDreamPrompt(cfg.Store)
	forkedCfg := engine.ForkedAgentConfig{
		PromptMessages: []types.Message{{
			Role: types.RoleUser,
			Content: []types.ContentBlock{{
				Type: types.ContentTypeText,
				Text: strPtr(prompt),
			}},
		}},
		CacheSafeParams: hookCtx.CacheParams,
		QuerySource:     autoDreamQuerySource,
		MaxTurns:        autoDreamMaxTurns,
		AllowedTools:    []string{"MemoryRead", "MemoryWrite", "MemoryDelete"},
	}

	if _, err := engine.RunForkedAgent(ctx, hookCtx.Engine, forkedCfg); err != nil {
		log.Printf("memdir: auto dream: %v", err)
	}
}

// RunManualDream triggers consolidation immediately (used by /dream).
func RunManualDream(ctx context.Context, eng engine.QueryEngine, cache *engine.CacheSafeParams, store *MemoryStore) error {
	if eng == nil || cache == nil || store == nil {
		return fmt.Errorf("memdir: manual dream: engine, cache params, and store are required")
	}
	lockPath := filepath.Join(store.Dir(), consolidateLockName)
	if err := touchConsolidateLock(lockPath); err != nil {
		return err
	}

	prompt := buildDreamPrompt(store)
	_, err := engine.RunForkedAgent(ctx, eng, engine.ForkedAgentConfig{
		PromptMessages: []types.Message{{
			Role: types.RoleUser,
			Content: []types.ContentBlock{{
				Type: types.ContentTypeText,
				Text: strPtr(prompt),
			}},
		}},
		CacheSafeParams: cache,
		QuerySource:     autoDreamQuerySource,
		MaxTurns:        autoDreamMaxTurns,
		AllowedTools:    []string{"MemoryRead", "MemoryWrite", "MemoryDelete"},
	})
	return err
}

func shouldRunAutoDream(memoryDir string) bool {
	lockPath := filepath.Join(memoryDir, consolidateLockName)
	info, err := os.Stat(lockPath)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) >= minHoursBetweenDream*time.Hour
}

func touchConsolidateLock(lockPath string) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("ensure memory dir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create consolidate lock: %w", err)
	}
	return f.Close()
}

func buildDreamPrompt(store *MemoryStore) string {
	idx, _ := store.LoadMemoryIndex()
	if strings.TrimSpace(idx) == "" {
		idx = "(empty)"
	}
	return fmt.Sprintf(`You are a memory consolidation assistant. Review the project's auto-memory index and memory files.

Goals:
1. Merge duplicate or overlapping memories
2. Remove outdated or contradictory entries
3. Keep MEMORY.md index under %d lines

Current MEMORY.md index:
%s

Use MemoryRead to inspect files, MemoryWrite/MemoryDelete to update. Be conservative — only change when clearly beneficial.`, MaxMemoryIndexLines, idx)
}
