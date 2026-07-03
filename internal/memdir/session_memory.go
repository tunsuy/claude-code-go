package memdir

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

const sessionMemoryQuerySource = "session_memory"

// SessionMemoryConfig configures session-memory generation.
type SessionMemoryConfig struct {
	SessionID string
	HomeDir   string
}

// SaveSessionMemory writes a compact session summary to ~/.claude/session-memory/.
func SaveSessionMemory(messages []types.Message, summary string, cfg SessionMemoryConfig) (string, error) {
	if strings.TrimSpace(summary) == "" {
		summary = buildConversationSummary(messages, defaultMaxSummaryMessages)
	}
	if strings.TrimSpace(summary) == "" {
		return "", fmt.Errorf("memdir: session memory: empty summary")
	}

	home := cfg.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("memdir: session memory: home dir: %w", err)
		}
	}

	dir := filepath.Join(home, DefaultMemoryBase, "session-memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("memdir: session memory: mkdir: %w", err)
	}

	name := cfg.SessionID
	if name == "" {
		name = fmt.Sprintf("session_%d", time.Now().Unix())
	}
	path := filepath.Join(dir, sanitizeSessionFileName(name)+".md")

	content := fmt.Sprintf("---\ntitle: Session Memory\ntype: reference\nupdated_at: %s\n---\n\n%s\n",
		time.Now().UTC().Format(time.RFC3339), summary)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("memdir: session memory: write: %w", err)
	}
	return path, nil
}

// ExecuteSessionMemory generates session memory via a forked agent after compaction.
func ExecuteSessionMemory(ctx context.Context, hookCtx *engine.StopHookContext, cfg SessionMemoryConfig) {
	if hookCtx == nil || hookCtx.Engine == nil || hookCtx.CacheParams == nil {
		return
	}
	if hookCtx.IsBareMode {
		return
	}

	summary := buildConversationSummary(hookCtx.Messages, defaultMaxSummaryMessages)
	if summary == "" {
		return
	}

	prompt := fmt.Sprintf(`Summarize this conversation for session recovery after compaction. Keep key decisions, open tasks, and file paths.

Conversation:
%s`, summary)

	forkedCfg := engine.ForkedAgentConfig{
		PromptMessages: []types.Message{{
			Role: types.RoleUser,
			Content: []types.ContentBlock{{
				Type: types.ContentTypeText,
				Text: strPtr(prompt),
			}},
		}},
		CacheSafeParams: hookCtx.CacheParams,
		QuerySource:     sessionMemoryQuerySource,
		MaxTurns:        2,
		AllowedTools:    nil,
	}

	msgs, err := engine.RunForkedAgent(ctx, hookCtx.Engine, forkedCfg)
	if err != nil {
		return
	}

	generated := extractAssistantText(msgs)
	if generated == "" {
		generated = summary
	}
	_, _ = SaveSessionMemory(hookCtx.Messages, generated, cfg)
}

func extractAssistantText(messages []types.Message) string {
	var parts []string
	for _, msg := range messages {
		if msg.Role != types.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == types.ContentTypeText && block.Text != nil {
				parts = append(parts, strings.TrimSpace(*block.Text))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func sanitizeSessionFileName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "session"
	}
	var sb strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	out := strings.Trim(sb.String(), "_")
	if out == "" {
		return "session"
	}
	return out
}
