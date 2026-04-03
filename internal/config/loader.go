// Package config handles loading and merging layered settings files.
// Priority (lowest to highest): User → Project → Local → Policy (unconditional).
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/anthropics/claude-code-go/pkg/types"
)

// PermissionsConfig corresponds to the permissions field in settings.json.
type PermissionsConfig struct {
	Allow          []string             `json:"allow,omitempty"`
	Deny           []string             `json:"deny,omitempty"`
	Ask            []string             `json:"ask,omitempty"`
	DefaultMode    types.PermissionMode `json:"defaultMode,omitempty"`
	DisableBypass  string               `json:"disableBypassPermissionsMode,omitempty"`
	AdditionalDirs []string             `json:"additionalDirectories,omitempty"`
}

// WorktreeConfig corresponds to the worktree field in settings.json.
type WorktreeConfig struct {
	SymlinkDirectories []string `json:"symlinkDirectories,omitempty"`
	SparsePaths        []string `json:"sparsePaths,omitempty"`
}

// AttributionConfig corresponds to the attribution field in settings.json.
type AttributionConfig struct {
	Commit string `json:"commit,omitempty"`
	PR     string `json:"pr,omitempty"`
}

// SettingsJson corresponds to the complete settings.json file structure.
// All fields use omitempty to ensure unset fields are not written back (backwards-compatible).
type SettingsJson struct {
	Schema              string                           `json:"$schema,omitempty"`
	APIKey              string                           `json:"apiKey,omitempty"`
	APIKeyHelper        string                           `json:"apiKeyHelper,omitempty"`
	BaseURL             string                           `json:"baseUrl,omitempty"`
	AWSCredentialExport string                           `json:"awsCredentialExport,omitempty"`
	AWSAuthRefresh      string                           `json:"awsAuthRefresh,omitempty"`
	GCPAuthRefresh      string                           `json:"gcpAuthRefresh,omitempty"`
	RespectGitignore    *bool                            `json:"respectGitignore,omitempty"`
	CleanupPeriodDays   *int                             `json:"cleanupPeriodDays,omitempty"`
	Env                 map[string]string                `json:"env,omitempty"`
	Attribution         *AttributionConfig               `json:"attribution,omitempty"`
	Permissions         *PermissionsConfig               `json:"permissions,omitempty"`
	Model               string                           `json:"model,omitempty"`
	AvailableModels     []string                         `json:"availableModels,omitempty"`
	ModelOverrides      map[string]string                `json:"modelOverrides,omitempty"`
	EnableAllProjectMCP *bool                            `json:"enableAllProjectMcpServers,omitempty"`
	EnabledMCPServers   []string                         `json:"enabledMcpjsonServers,omitempty"`
	DisabledMCPServers  []string                         `json:"disabledMcpjsonServers,omitempty"`
	Hooks               map[types.HookType][]types.HookDefinition `json:"hooks,omitempty"`
	Worktree            *WorktreeConfig                  `json:"worktree,omitempty"`
	DisableAllHooks     *bool                            `json:"disableAllHooks,omitempty"`
	DefaultShell        string                           `json:"defaultShell,omitempty"`
	AllowManagedHooksOnly *bool                          `json:"allowManagedHooksOnly,omitempty"`
}

// SettingSource identifies the configuration layer.
type SettingSource string

const (
	SourceUser    SettingSource = "userSettings"
	SourceProject SettingSource = "projectSettings"
	SourceLocal   SettingSource = "localSettings"
	SourcePolicy  SettingSource = "policySettings"
)

// Config directory and file name constants.
const (
	ClaudeDir           = ".claude"
	ClaudeLocalDir      = ".claude.local"
	SettingsFile        = "settings.json"
	ManagedSettingsFile = "managed-settings.json"

	// Subdirectories under the global ~/.claude/ directory.
	SessionsDir = "projects" // JSONL session storage
	TodosDir    = "todos"
	StatsFile   = "statsig.json"
)

// LayeredSettings holds the raw settings per tier and the merged effective settings.
// Merge order (ascending priority): User → Project → Local
// Policy is applied last as an unconditional override (highest priority).
type LayeredSettings struct {
	User    *SettingsJson
	Project *SettingsJson
	Local   *SettingsJson
	// Policy is the enterprise managed-settings.json; its locked fields cannot
	// be overridden by any other layer.
	Policy *SettingsJson
	// Merged is the effective configuration after all layers are applied:
	//   base = merge(User, Project, Local)  [ascending priority]
	//   Merged = applyPolicyOverrides(base, Policy)
	Merged *SettingsJson
}

// ConfigLoader is the interface for loading layered settings.
// Define the interface in the consumer package (internal/bootstrap) following Go convention;
// the concrete *Loader implementation is used directly by callers that can import this package.
type ConfigLoader interface {
	Load() (*LayeredSettings, error)
}

// Loader loads and merges layered configuration from the filesystem.
type Loader struct {
	homeDir    string
	projectDir string
}

// NewLoader creates a Loader for the given home and project directories.
func NewLoader(homeDir, projectDir string) *Loader {
	return &Loader{homeDir: homeDir, projectDir: projectDir}
}

// Load reads all configuration tiers, merges them, and returns a LayeredSettings.
// Missing files are silently skipped.  I/O errors other than "not exist" are returned.
func (l *Loader) Load() (*LayeredSettings, error) {
	ls := &LayeredSettings{}

	type pathTarget struct {
		src SettingSource
		ptr **SettingsJson
	}
	targets := []pathTarget{
		{SourceUser, &ls.User},
		{SourceProject, &ls.Project},
		{SourceLocal, &ls.Local},
		{SourcePolicy, &ls.Policy},
	}
	paths := map[SettingSource]string{
		SourceUser:    filepath.Join(l.homeDir, ClaudeDir, SettingsFile),
		SourceProject: filepath.Join(l.projectDir, ClaudeDir, SettingsFile),
		SourceLocal:   filepath.Join(l.projectDir, ClaudeLocalDir, SettingsFile),
		SourcePolicy:  filepath.Join(l.homeDir, ClaudeDir, ManagedSettingsFile),
	}

	for _, t := range targets {
		data, err := os.ReadFile(paths[t.src])
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // missing layer is OK
			}
			return nil, err
		}
		s := &SettingsJson{}
		if err := json.Unmarshal(data, s); err != nil {
			return nil, err
		}
		*t.ptr = s
	}

	// Step 1: merge user-controllable layers in ascending priority.
	base := mergeSettings(ls.User, ls.Project, ls.Local)
	// Step 2: apply environment-variable overrides (higher than any file).
	applyEnvOverrides(base)
	// Step 3: Policy locked fields unconditionally override everything.
	ls.Merged = applyPolicyOverrides(base, ls.Policy)
	return ls, nil
}

// mergeSettings merges layers left-to-right (later layers override earlier ones).
// nil layers are skipped.
func mergeSettings(layers ...*SettingsJson) *SettingsJson {
	merged := &SettingsJson{}
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		applyLayer(merged, layer)
	}
	return merged
}

// applyLayer copies all non-zero fields from src into dst.
// Array fields (permission allow/deny/ask, available models, etc.) are merged with
// deduplication; scalar fields are overwritten.
func applyLayer(dst, src *SettingsJson) {
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
	}
	if src.APIKeyHelper != "" {
		dst.APIKeyHelper = src.APIKeyHelper
	}
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.AWSCredentialExport != "" {
		dst.AWSCredentialExport = src.AWSCredentialExport
	}
	if src.AWSAuthRefresh != "" {
		dst.AWSAuthRefresh = src.AWSAuthRefresh
	}
	if src.GCPAuthRefresh != "" {
		dst.GCPAuthRefresh = src.GCPAuthRefresh
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.DefaultShell != "" {
		dst.DefaultShell = src.DefaultShell
	}
	if src.Schema != "" {
		dst.Schema = src.Schema
	}
	if src.RespectGitignore != nil {
		dst.RespectGitignore = src.RespectGitignore
	}
	if src.CleanupPeriodDays != nil {
		dst.CleanupPeriodDays = src.CleanupPeriodDays
	}
	if src.EnableAllProjectMCP != nil {
		dst.EnableAllProjectMCP = src.EnableAllProjectMCP
	}
	if src.DisableAllHooks != nil {
		dst.DisableAllHooks = src.DisableAllHooks
	}
	if src.AllowManagedHooksOnly != nil {
		dst.AllowManagedHooksOnly = src.AllowManagedHooksOnly
	}
	if src.Attribution != nil {
		if dst.Attribution == nil {
			dst.Attribution = &AttributionConfig{}
		}
		if src.Attribution.Commit != "" {
			dst.Attribution.Commit = src.Attribution.Commit
		}
		if src.Attribution.PR != "" {
			dst.Attribution.PR = src.Attribution.PR
		}
	}
	if src.Worktree != nil {
		if dst.Worktree == nil {
			dst.Worktree = &WorktreeConfig{}
		}
		dst.Worktree.SymlinkDirectories = uniqueAppend(dst.Worktree.SymlinkDirectories, src.Worktree.SymlinkDirectories...)
		dst.Worktree.SparsePaths = uniqueAppend(dst.Worktree.SparsePaths, src.Worktree.SparsePaths...)
	}
	// env: src keys override dst keys with same name.
	if len(src.Env) > 0 {
		if dst.Env == nil {
			dst.Env = make(map[string]string, len(src.Env))
		}
		for k, v := range src.Env {
			dst.Env[k] = v
		}
	}
	// modelOverrides: src keys override dst keys with same name.
	if len(src.ModelOverrides) > 0 {
		if dst.ModelOverrides == nil {
			dst.ModelOverrides = make(map[string]string, len(src.ModelOverrides))
		}
		for k, v := range src.ModelOverrides {
			dst.ModelOverrides[k] = v
		}
	}
	// array fields: unique-append.
	dst.AvailableModels = uniqueAppend(dst.AvailableModels, src.AvailableModels...)
	dst.EnabledMCPServers = uniqueAppend(dst.EnabledMCPServers, src.EnabledMCPServers...)
	dst.DisabledMCPServers = uniqueAppend(dst.DisabledMCPServers, src.DisabledMCPServers...)
	// permissions: merge sub-fields.
	if src.Permissions != nil {
		if dst.Permissions == nil {
			dst.Permissions = &PermissionsConfig{}
		}
		mergePermissions(dst.Permissions, src.Permissions)
	}
	// hooks: merge per HookType.
	if len(src.Hooks) > 0 {
		if dst.Hooks == nil {
			dst.Hooks = make(map[types.HookType][]types.HookDefinition, len(src.Hooks))
		}
		for ht, defs := range src.Hooks {
			dst.Hooks[ht] = append(dst.Hooks[ht], defs...)
		}
	}
}

func mergePermissions(dst, src *PermissionsConfig) {
	dst.Allow = uniqueAppend(dst.Allow, src.Allow...)
	dst.Deny = uniqueAppend(dst.Deny, src.Deny...)
	dst.Ask = uniqueAppend(dst.Ask, src.Ask...)
	if src.DefaultMode != "" {
		dst.DefaultMode = src.DefaultMode
	}
	if src.DisableBypass != "" {
		dst.DisableBypass = src.DisableBypass
	}
	dst.AdditionalDirs = uniqueAppend(dst.AdditionalDirs, src.AdditionalDirs...)
}

// applyPolicyOverrides unconditionally applies all non-zero fields from policy
// into base (Policy has highest priority and cannot be overridden by users).
// Returns base (modified in-place) for convenience.
func applyPolicyOverrides(base *SettingsJson, policy *SettingsJson) *SettingsJson {
	if policy == nil {
		return base
	}
	// Policy overrides: scalar fields always win.
	if policy.Model != "" {
		base.Model = policy.Model
	}
	if policy.APIKey != "" {
		base.APIKey = policy.APIKey
	}
	if policy.APIKeyHelper != "" {
		base.APIKeyHelper = policy.APIKeyHelper
	}
	if policy.BaseURL != "" {
		base.BaseURL = policy.BaseURL
	}
	if policy.DefaultShell != "" {
		base.DefaultShell = policy.DefaultShell
	}
	if policy.RespectGitignore != nil {
		base.RespectGitignore = policy.RespectGitignore
	}
	if policy.CleanupPeriodDays != nil {
		base.CleanupPeriodDays = policy.CleanupPeriodDays
	}
	if policy.DisableAllHooks != nil {
		base.DisableAllHooks = policy.DisableAllHooks
	}
	if policy.AllowManagedHooksOnly != nil {
		base.AllowManagedHooksOnly = policy.AllowManagedHooksOnly
	}
	if policy.EnableAllProjectMCP != nil {
		base.EnableAllProjectMCP = policy.EnableAllProjectMCP
	}
	if policy.AWSCredentialExport != "" {
		base.AWSCredentialExport = policy.AWSCredentialExport
	}
	if policy.AWSAuthRefresh != "" {
		base.AWSAuthRefresh = policy.AWSAuthRefresh
	}
	if policy.GCPAuthRefresh != "" {
		base.GCPAuthRefresh = policy.GCPAuthRefresh
	}
	// Policy permissions: override (not append) to enforce enterprise rules.
	if policy.Permissions != nil {
		if base.Permissions == nil {
			base.Permissions = &PermissionsConfig{}
		}
		// Policy locked fields override user choices unconditionally.
		if policy.Permissions.DefaultMode != "" {
			base.Permissions.DefaultMode = policy.Permissions.DefaultMode
		}
		if policy.Permissions.DisableBypass != "" {
			base.Permissions.DisableBypass = policy.Permissions.DisableBypass
		}
		// Deny rules from policy are prepended so they are always evaluated first.
		base.Permissions.Deny = uniqueAppend(policy.Permissions.Deny, base.Permissions.Deny...)
		base.Permissions.Allow = uniqueAppend(base.Permissions.Allow, policy.Permissions.Allow...)
	}
	// Policy env keys override base.
	if len(policy.Env) > 0 {
		if base.Env == nil {
			base.Env = make(map[string]string, len(policy.Env))
		}
		for k, v := range policy.Env {
			base.Env[k] = v
		}
	}
	// Policy hooks override (replace, not append) for managed-only enforcement.
	if len(policy.Hooks) > 0 {
		base.Hooks = policy.Hooks
	}
	return base
}

// applyEnvOverrides applies well-known environment variables as overrides
// on top of file-based settings.  Environment variables have higher priority
// than any file but lower priority than Policy.
func applyEnvOverrides(s *SettingsJson) {
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		s.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
		s.BaseURL = v
	}
	// Model: prefer ANTHROPIC_MODEL, fall back to CLAUDE_MODEL.
	if v := os.Getenv("ANTHROPIC_MODEL"); v != "" {
		s.Model = v
	} else if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		s.Model = v
	}
}

// uniqueAppend appends elements from src to dst, skipping duplicates.
func uniqueAppend(dst []string, src ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(src))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	for _, v := range src {
		if _, ok := seen[v]; !ok {
			dst = append(dst, v)
			seen[v] = struct{}{}
		}
	}
	return dst
}
