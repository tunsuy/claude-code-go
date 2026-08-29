package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readAllowRules loads the permissions list from a written settings.json.
func readPermissionList(t *testing.T, dir, list string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ClaudeDir, SettingsFile))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var doc struct {
		Permissions map[string][]string `json:"permissions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return doc.Permissions[list]
}

func TestAddPermissionRuleCreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := AddPermissionRule(dir, PermissionListAllow, "Bash(go test:*)"); err != nil {
		t.Fatalf("AddPermissionRule: %v", err)
	}
	got := readPermissionList(t, dir, PermissionListAllow)
	if len(got) != 1 || got[0] != "Bash(go test:*)" {
		t.Errorf("allow = %v, want [Bash(go test:*)]", got)
	}
}

func TestAddPermissionRuleIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := AddPermissionRule(dir, PermissionListAllow, "Read"); err != nil {
			t.Fatalf("AddPermissionRule #%d: %v", i, err)
		}
	}
	got := readPermissionList(t, dir, PermissionListAllow)
	if len(got) != 1 || got[0] != "Read" {
		t.Errorf("allow = %v, want single [Read] entry", got)
	}
}

func TestAddPermissionRulePreservesOtherFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	settingsDir := filepath.Join(dir, ClaudeDir)
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{
  "model": "claude-sonnet",
  "permissions": {"allow": ["Read"], "deny": ["Bash(rm -rf *)"]},
  "unknownFutureField": {"nested": true}
}`
	if err := os.WriteFile(filepath.Join(settingsDir, SettingsFile), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AddPermissionRule(dir, PermissionListAllow, "Write"); err != nil {
		t.Fatalf("AddPermissionRule: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(settingsDir, SettingsFile))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc["model"] != "claude-sonnet" {
		t.Errorf("model = %v, want preserved", doc["model"])
	}
	if _, ok := doc["unknownFutureField"]; !ok {
		t.Error("unknownFutureField should be preserved")
	}
	perms := doc["permissions"].(map[string]any)
	if deny, ok := perms["deny"].([]any); !ok || len(deny) != 1 {
		t.Errorf("deny = %v, want preserved single entry", perms["deny"])
	}
	allow := readPermissionList(t, dir, PermissionListAllow)
	if len(allow) != 2 {
		t.Errorf("allow = %v, want [Read Write]", allow)
	}
}

func TestAddPermissionRuleSeparateLists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := AddPermissionRule(dir, PermissionListAllow, "Read"); err != nil {
		t.Fatal(err)
	}
	if err := AddPermissionRule(dir, PermissionListDeny, "Bash(sudo *)"); err != nil {
		t.Fatal(err)
	}
	if got := readPermissionList(t, dir, PermissionListDeny); len(got) != 1 || got[0] != "Bash(sudo *)" {
		t.Errorf("deny = %v", got)
	}
	if got := readPermissionList(t, dir, PermissionListAllow); len(got) != 1 || got[0] != "Read" {
		t.Errorf("allow = %v", got)
	}
}

func TestAddPermissionRuleInvalidArgs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := AddPermissionRule(dir, "sometimes", "Read"); err == nil {
		t.Error("invalid list should error")
	}
	if err := AddPermissionRule(dir, PermissionListAllow, ""); err == nil {
		t.Error("empty rule should error")
	}
	// Nothing should have been created for the invalid calls.
	if _, err := os.Stat(filepath.Join(dir, ClaudeDir)); !os.IsNotExist(err) {
		t.Error("no settings file should be created on invalid input")
	}
}
