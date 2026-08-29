package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Permission list names within settings.json's permissions object.
const (
	PermissionListAllow = "allow"
	PermissionListDeny  = "deny"
	PermissionListAsk   = "ask"
)

// validPermissionLists is the set of permission list names AddPermissionRule
// may write to.
var validPermissionLists = map[string]bool{
	PermissionListAllow: true,
	PermissionListDeny:  true,
	PermissionListAsk:   true,
}

// AddPermissionRule appends rule to the given permissions list
// ("allow"/"deny"/"ask") of the project's .claude/settings.json, creating the
// file if needed. The operation is idempotent — an already-present rule is a
// no-op. Unknown fields in the file are preserved. The write is atomic
// (temp file + rename) so a crash mid-write never corrupts the settings file.
func AddPermissionRule(projectDir, list, rule string) error {
	if !validPermissionLists[list] {
		return fmt.Errorf("config: invalid permission list %q (want allow, deny or ask)", list)
	}
	if rule == "" {
		return errors.New("config: empty permission rule")
	}

	settingsPath := filepath.Join(projectDir, ClaudeDir, SettingsFile)

	// Read the existing document as a generic map so unknown fields survive
	// the round-trip (SettingsJson would drop them).
	doc := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("config: parse %s: %w", settingsPath, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("config: read %s: %w", settingsPath, err)
	}

	perms, ok := doc["permissions"].(map[string]any)
	if !ok {
		perms = map[string]any{}
		doc["permissions"] = perms
	}

	existing, _ := perms[list].([]any)
	for _, v := range existing {
		if s, ok := v.(string); ok && s == rule {
			return nil // already present — idempotent
		}
	}
	perms[list] = append(existing, rule)

	return writeSettingsAtomic(settingsPath, doc)
}

// writeSettingsAtomic serialises doc as indented JSON and replaces settingsPath
// via temp-file + rename so readers never observe a partially-written file.
func writeSettingsAtomic(settingsPath string, doc map[string]any) error {
	dir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: create %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal settings: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return fmt.Errorf("config: create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("config: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, settingsPath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: rename %s → %s: %w", tmpName, settingsPath, err)
	}
	return nil
}
