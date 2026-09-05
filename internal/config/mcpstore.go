package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MCP configuration scopes, matching the oracle TS CLI's storage layout.
const (
	MCPScopeLocal   = "local"   // ~/.claude.json → projects.<cwd>.mcpServers
	MCPScopeUser    = "user"    // ~/.claude.json → top-level mcpServers
	MCPScopeProject = "project" // <cwd>/.mcp.json → mcpServers
)

// ClaudeJsonPath returns the path of the ~/.claude.json state file that holds
// user-scope and local-scope MCP server configuration.
func ClaudeJsonPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude.json")
}

// McpJsonPath returns the path of the project-scope .mcp.json file.
func McpJsonPath(projectDir string) string {
	return filepath.Join(projectDir, ".mcp.json")
}

// writableMCPScope reports whether scope is one of the three user-writable
// scopes.  The oracle accepts more scope words on the command line (dynamic,
// enterprise, claudeai, managed, agent) but rejects them at write time with
// "Cannot add/remove MCP server ... scope: X".
var writableMCPScope = map[string]bool{
	MCPScopeLocal:   true,
	MCPScopeUser:    true,
	MCPScopeProject: true,
}

// ValidateMCPScope validates a --scope flag value against the oracle's full
// scope word list.  The returned error text matches the oracle verbatim.
// Writability is a separate check (AddMCPServer/RemoveMCPServer).
func ValidateMCPScope(scope string) error {
	switch scope {
	case "local", "user", "project", "dynamic", "enterprise", "claudeai", "managed", "agent":
		return nil
	}
	return fmt.Errorf("Invalid scope: %s. Must be one of: local, user, project, dynamic, enterprise, claudeai, managed, agent", scope)
}

// EnvPair is one KEY=VALUE entry of a stdio server's env map.
type EnvPair struct {
	Key   string
	Value string
}

// MCPServerEntry is one server definition as stored in configuration.  The
// raw ordered map is kept so entries round-trip verbatim — including entries
// that omit "type" (the legacy stdio format) and unknown transport types,
// which the CLI must still be able to name and diagnose.
type MCPServerEntry struct {
	Name string
	Raw  *OrderedMap
}

// Type returns the stored transport type ("stdio", "sse", "http") or "" when
// the entry is a legacy definition without a "type" field.
func (e MCPServerEntry) Type() string {
	return e.Raw.GetString("type")
}

// Command returns the stdio command, or "" for http/sse entries.
func (e MCPServerEntry) Command() string {
	return e.Raw.GetString("command")
}

// Args returns the stdio args slice (never nil for stdio entries).
func (e MCPServerEntry) Args() []string {
	rawArgs, _ := e.Raw.Get("args")
	arr, _ := rawArgs.([]any)
	args := make([]string, 0, len(arr))
	for _, a := range arr {
		if s, ok := a.(string); ok {
			args = append(args, s)
		}
	}
	return args
}

// EnvPairs returns the stdio environment entries in storage insertion order.
func (e MCPServerEntry) EnvPairs() []EnvPair {
	var pairs []EnvPair
	rawEnv, ok := e.Raw.GetMap("env")
	if !ok {
		return pairs
	}
	for _, k := range rawEnv.Keys() {
		if v, ok := rawEnv.Get(k); ok {
			if s, ok := v.(string); ok {
				pairs = append(pairs, EnvPair{Key: k, Value: s})
			}
		}
	}
	return pairs
}

// Headers returns the http/sse header entries in storage insertion order.
func (e MCPServerEntry) Headers() []EnvPair {
	var pairs []EnvPair
	rawHeaders, ok := e.Raw.GetMap("headers")
	if !ok {
		return pairs
	}
	for _, k := range rawHeaders.Keys() {
		if v, ok := rawHeaders.Get(k); ok {
			if s, ok := v.(string); ok {
				pairs = append(pairs, EnvPair{Key: k, Value: s})
			}
		}
	}
	return pairs
}

// URL returns the http/sse endpoint URL, or "" for stdio entries.
func (e MCPServerEntry) URL() string {
	return e.Raw.GetString("url")
}

// KnownType reports whether the entry's transport type is one the CLI
// recognizes.  Legacy typeless entries count as known (the oracle treats them
// as stdio for connection purposes); "ws" counts as known too — `mcp get`
// finds and health-checks ws entries (verified by capture) — while non-empty
// unknown types are "Skipped" in list output and flagged in diagnostics.
func (e MCPServerEntry) KnownType() bool {
	switch e.Type() {
	case "", "stdio", "sse", "http", "ws":
		return true
	}
	return false
}

// ListableType reports whether the entry appears as a status line in
// `mcp list`.  ws entries are deliberately absent: a captured `mcp list`
// against a store holding a ws server shows the name in neither the status
// lines nor the diagnostics section (get still finds it).
func (e MCPServerEntry) ListableType() bool {
	switch e.Type() {
	case "", "stdio", "sse", "http":
		return true
	}
	return false
}

// Endpoint renders the entry's connection endpoint the way the diagnostics
// conflict report does: joined command+args for stdio, the URL otherwise.
func (e MCPServerEntry) Endpoint() string {
	if t := e.Type(); t == "sse" || t == "http" {
		return e.URL()
	}
	return strings.Join(append([]string{e.Command()}, e.Args()...), " ")
}

// MCPStore reads and writes MCP server configuration across the three
// user-writable scopes, preserving foreign keys and key order in both storage
// files (both matter: the oracle renders servers, env vars and headers in
// storage insertion order, and rewrites must not reshuffle them).
type MCPStore struct {
	homeDir    string
	projectDir string
}

// NewMCPStore creates a store bound to the given home and project directories.
func NewMCPStore(homeDir, projectDir string) *MCPStore {
	return &MCPStore{homeDir: homeDir, projectDir: projectDir}
}

// HomeDir returns the home directory the store is bound to (verbatim — the
// oracle displays $HOME as set, without symlink resolution).
func (s *MCPStore) HomeDir() string { return s.homeDir }

// ProjectKey is the resolved project path the oracle uses as the
// projects.<key> entry for local-scope servers.  On macOS the oracle stores
// the symlink-resolved path (/private/tmp vs /tmp).
func (s *MCPStore) ProjectKey() string {
	resolved, err := filepath.EvalSymlinks(s.projectDir)
	if err != nil {
		return s.projectDir
	}
	return resolved
}

// ScopePath returns the file backing a scope: ~/.claude.json for local and
// user, <project>/.mcp.json for project.
func (s *MCPStore) ScopePath(scope string) string {
	if scope == MCPScopeProject {
		return McpJsonPath(s.projectDir)
	}
	return ClaudeJsonPath(s.homeDir)
}

// ScopeLabel returns the human-readable scope description used by `get`.
func ScopeLabel(scope string) string {
	switch scope {
	case MCPScopeLocal:
		return "Local config (private to you in this project)"
	case MCPScopeUser:
		return "User config (available in all your projects)"
	case MCPScopeProject:
		return "Project config (shared via .mcp.json)"
	}
	return scope
}

// shortScopeLabel is the scope name used inside error messages: "local
// config", "user config", or ".mcp.json" for project scope.
func shortScopeLabel(scope string) string {
	switch scope {
	case MCPScopeLocal:
		return "local config"
	case MCPScopeUser:
		return "user config"
	case MCPScopeProject:
		return ".mcp.json"
	}
	return scope
}

// claudeJsonDoc loads ~/.claude.json (empty when missing).
func (s *MCPStore) claudeJsonDoc() (*OrderedMap, error) {
	return LoadOrderedJSON(ClaudeJsonPath(s.homeDir))
}

// mcpJsonDoc loads the project's .mcp.json (empty when missing).
func (s *MCPStore) mcpJsonDoc() (*OrderedMap, error) {
	return LoadOrderedJSON(McpJsonPath(s.projectDir))
}

// docForScope loads the document backing a scope.
func (s *MCPStore) docForScope(scope string) (*OrderedMap, error) {
	if scope == MCPScopeProject {
		return s.mcpJsonDoc()
	}
	return s.claudeJsonDoc()
}

// ensureScopeMap extracts (creating if needed, wired into the document) the
// mcpServers map for a scope.  Creating a fresh local-scope project entry
// also seeds the sibling keys the oracle initializes alongside mcpServers.
func (s *MCPStore) ensureScopeMap(doc *OrderedMap, scope string) (*OrderedMap, error) {
	switch scope {
	case MCPScopeUser:
		m, _ := doc.GetMap("mcpServers")
		if m == nil {
			m = NewOrderedMap()
			doc.Set("mcpServers", m)
		}
		return m, nil
	case MCPScopeProject:
		m, _ := doc.GetMap("mcpServers")
		if m == nil {
			m = NewOrderedMap()
			doc.Set("mcpServers", m)
		}
		return m, nil
	case MCPScopeLocal:
		projects, _ := doc.GetMap("projects")
		if projects == nil {
			projects = NewOrderedMap()
			doc.Set("projects", projects)
		}
		key := s.ProjectKey()
		entry, _ := projects.GetMap(key)
		if entry == nil {
			// Seeded sibling keys match oracle v2.1.261 exactly (verified by
			// capture on a fresh HOME): 8 keys, including the empty
			// enabled/disabledMcpjsonServers arrays.  (A capture that showed
			// only 6 keys ran `mcp reset-project-choices` first, which deletes
			// those two — see ResetProjectChoices.)
			entry = NewOrderedMap()
			entry.Set("allowedTools", []any{})
			entry.Set("mcpContextUris", []any{})
			entry.Set("mcpServers", NewOrderedMap())
			entry.Set("enabledMcpjsonServers", []any{})
			entry.Set("disabledMcpjsonServers", []any{})
			entry.Set("hasTrustDialogAccepted", false)
			entry.Set("hasClaudeMdExternalIncludesApproved", false)
			entry.Set("hasClaudeMdExternalIncludesWarningShown", false)
			projects.Set(key, entry)
		}
		m, _ := entry.GetMap("mcpServers")
		if m == nil {
			m = NewOrderedMap()
			entry.Set("mcpServers", m)
		}
		return m, nil
	}
	return nil, fmt.Errorf("config: unknown scope %q", scope)
}

// ScopeEntries returns one scope's server entries in storage insertion order
// without creating anything.  Values that are not JSON objects are skipped.
func (s *MCPStore) ScopeEntries(scope string) ([]MCPServerEntry, error) {
	doc, err := s.docForScope(scope)
	if err != nil {
		return nil, err
	}
	m := s.scopeMapReadOnly(doc, scope)
	if m == nil {
		return nil, nil
	}
	entries := make([]MCPServerEntry, 0, m.Len())
	for _, name := range m.Keys() {
		if raw, ok := m.GetMap(name); ok && raw != nil {
			entries = append(entries, MCPServerEntry{Name: name, Raw: raw})
		}
	}
	return entries, nil
}

// scopeMapReadOnly finds a scope's mcpServers map without creating it.
func (s *MCPStore) scopeMapReadOnly(doc *OrderedMap, scope string) *OrderedMap {
	if scope == MCPScopeUser || scope == MCPScopeProject {
		m, _ := doc.GetMap("mcpServers")
		return m
	}
	if scope == MCPScopeLocal {
		projects, _ := doc.GetMap("projects")
		if projects == nil {
			return nil
		}
		entry, _ := projects.GetMap(s.ProjectKey())
		if entry == nil {
			return nil
		}
		m, _ := entry.GetMap("mcpServers")
		return m
	}
	return nil
}

// AddMCPServer stores an entry in the given scope.  Duplicate detection is
// scope-local, matching the oracle: the same name in another scope is fine.
func (s *MCPStore) AddMCPServer(scope, name string, entry *OrderedMap) error {
	if !writableMCPScope[scope] {
		return fmt.Errorf("Cannot add MCP server to scope: %s", scope)
	}
	if name == "" {
		return errors.New("config: empty MCP server name")
	}
	doc, err := s.docForScope(scope)
	if err != nil {
		return err
	}
	m, err := s.ensureScopeMap(doc, scope)
	if err != nil {
		return err
	}
	if _, exists := m.Get(name); exists {
		return fmt.Errorf("MCP server %s already exists in %s", name, shortScopeLabel(scope))
	}
	m.Set(name, entry)
	return WriteOrderedJSONAtomic(s.ScopePath(scope), doc)
}

// RemoveMCPServer deletes an entry from one scope.  Returns false when the
// name is not present in that scope (no write happens in that case).
func (s *MCPStore) RemoveMCPServer(scope, name string) (bool, error) {
	if !writableMCPScope[scope] {
		return false, fmt.Errorf("Cannot remove MCP server from scope: %s", scope)
	}
	doc, err := s.docForScope(scope)
	if err != nil {
		return false, err
	}
	m := s.scopeMapReadOnly(doc, scope)
	if m == nil {
		return false, nil
	}
	if _, exists := m.Get(name); !exists {
		return false, nil
	}
	m.Delete(name)
	return true, WriteOrderedJSONAtomic(s.ScopePath(scope), doc)
}

// GetMCPServer looks a name up across scopes with local > user > project
// precedence and returns the winning entry plus its scope.  Entries with
// unknown transport types are skipped — the oracle's `get` treats them as
// absent (the name only resurfaces in list diagnostics).
func (s *MCPStore) GetMCPServer(name string) (MCPServerEntry, string, bool) {
	for _, scope := range []string{MCPScopeLocal, MCPScopeUser, MCPScopeProject} {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name == name && e.KnownType() {
				return e, scope, true
			}
		}
	}
	return MCPServerEntry{}, "", false
}

// ScopesWithServer lists the scopes (in local, project, user order) in which
// name is defined.  Used by `remove` to decide between single-scope removal
// and the multi-scope disambiguation error.
func (s *MCPStore) ScopesWithServer(name string) []string {
	var scopes []string
	for _, scope := range []string{MCPScopeLocal, MCPScopeProject, MCPScopeUser} {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name == name {
				scopes = append(scopes, scope)
				break
			}
		}
	}
	return scopes
}

// ListScopeOrder is the scope iteration order of `mcp list`: user → project
// → local, deduplication by first-seen name.
var ListScopeOrder = []string{MCPScopeUser, MCPScopeProject, MCPScopeLocal}

// ListMCPServers returns the servers `mcp list` renders: all scopes in
// ListScopeOrder, storage insertion order within a scope, deduplicated by
// first-seen name.  Project-scope servers listed in the project's
// disabledMcpjsonServers are omitted; entries whose type is neither listable
// (stdio/sse/http/typeless) appear in diagnostics instead.
func (s *MCPStore) ListMCPServers() []MCPServerEntry {
	rejected := s.rejectedProjectServers()

	type listed struct {
		entry MCPServerEntry
		scope string
	}
	var out []listed
	seen := map[string]bool{}
	for _, scope := range ListScopeOrder {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if seen[e.Name] {
				continue
			}
			if scope == MCPScopeProject && rejected[e.Name] {
				continue
			}
			if !e.ListableType() {
				continue
			}
			seen[e.Name] = true
			out = append(out, listed{entry: e, scope: scope})
		}
	}

	// The rendered endpoint comes from the winning scope (local > user >
	// project), not necessarily the scope that introduced the name.  A ws
	// winner (get finds it, list does not) falls back to the introducing
	// listable entry.
	result := make([]MCPServerEntry, 0, len(out))
	for _, l := range out {
		if e, _, ok := s.GetMCPServer(l.entry.Name); ok && e.ListableType() {
			result = append(result, e)
		} else {
			result = append(result, l.entry)
		}
	}
	return result
}

// SettingsLocalPath returns the project's .claude/settings.local.json — the
// file the oracle's interactive .mcp.json approval flow writes choices to.
func SettingsLocalPath(projectDir string) string {
	return filepath.Join(projectDir, ClaudeDir, "settings.local.json")
}

// mcpApprovalSettingsPaths lists the settings files whose
// enabledMcpjsonServers/disabledMcpjsonServers keys record .mcp.json approval
// choices: project local, project shared, then user settings.  Rejection state
// is the union across all three (a name listed in any layer counts).
func (s *MCPStore) mcpApprovalSettingsPaths() []string {
	return []string{
		SettingsLocalPath(s.projectDir),
		filepath.Join(s.projectDir, ClaudeDir, SettingsFile),
		filepath.Join(s.homeDir, ClaudeDir, SettingsFile),
	}
}

// settingsApprovalNames returns the union of server names listed under key
// across the approval settings files.  Unreadable or malformed files are
// skipped — approval state degrades to "pending", never to a hard error.
func (s *MCPStore) settingsApprovalNames(key string) map[string]bool {
	names := map[string]bool{}
	for _, path := range s.mcpApprovalSettingsPaths() {
		doc, err := LoadOrderedJSON(path)
		if err != nil {
			continue
		}
		rawList, _ := doc.Get(key)
		list, _ := rawList.([]any)
		for _, v := range list {
			if name, ok := v.(string); ok {
				names[name] = true
			}
		}
	}
	return names
}

// rejectedProjectServers returns the rejected .mcp.json server names — the
// project-scope servers `mcp list` omits entirely.
func (s *MCPStore) rejectedProjectServers() map[string]bool {
	return s.settingsApprovalNames("disabledMcpjsonServers")
}

// AllServerNames returns every configured server name across all scopes,
// sorted alphabetically — the oracle's "Configured servers: ..." hint list
// for `remove`, which includes pending .mcp.json servers and entries with
// unknown transport types.
func (s *MCPStore) AllServerNames() []string {
	seen := map[string]bool{}
	for _, scope := range []string{MCPScopeLocal, MCPScopeUser, MCPScopeProject} {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			seen[e.Name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// VisibleServerNames returns the server names `get` may suggest: local- and
// user-scope servers with known transport types, sorted alphabetically.
// Every project-scope server is excluded — pending, enabled and rejected
// alike (verified by capture: 24 visible = 22 local + 2 user with project
// servers present in all three states).
func (s *MCPStore) VisibleServerNames() []string {
	seen := map[string]bool{}
	for _, scope := range []string{MCPScopeLocal, MCPScopeUser} {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.KnownType() {
				continue
			}
			seen[e.Name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// HasPendingProjectServers reports whether the project scope defines at least
// one server that is neither rejected nor shadowed by the same name in local
// or user scope — the condition behind get's "(.mcp.json servers are awaiting
// approval …)" parenthetical and its dedicated miss message when nothing else
// can be listed.  Shadowed names never trigger it: the winning definition
// lives in local/user scope, so there is nothing left to approve.
func (s *MCPStore) HasPendingProjectServers() bool {
	shadowed := map[string]bool{}
	for _, scope := range []string{MCPScopeLocal, MCPScopeUser} {
		entries, err := s.ScopeEntries(scope)
		if err != nil {
			continue
		}
		for _, e := range entries {
			shadowed[e.Name] = true
		}
	}
	entries, err := s.ScopeEntries(MCPScopeProject)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if s.ProjectApprovalState(e.Name) != "rejected" && !shadowed[e.Name] {
			return true
		}
	}
	return false
}

// ProjectApprovalState reports the .mcp.json approval state of a project-scope
// server: "rejected" or "pending".  Rejection is membership in the
// disabledMcpjsonServers list of the layered settings files (union across
// project .claude/settings.local.json, project .claude/settings.json and user
// ~/.claude/settings.json — verified by capture: the oracle records choices in
// settings.local.json, not in the ~/.claude.json project entry).
// Membership in enabledMcpjsonServers does NOT count as approved — the oracle
// still renders such servers as ⏸ Pending.
func (s *MCPStore) ProjectApprovalState(name string) string {
	if s.rejectedProjectServers()[name] {
		return "rejected"
	}
	return "pending"
}

// ResetProjectChoices deletes the enabledMcpjsonServers and
// disabledMcpjsonServers keys from the project's .claude/settings.local.json
// and from the ~/.claude.json project entry (both locations verified by
// capture: `mcp add` seeds the arrays in the project entry, and
// `mcp reset-project-choices` removes them again).  Every other key is
// preserved.  Each location is a no-op (no file created, no rewrite) when its
// keys are absent.
func (s *MCPStore) ResetProjectChoices() error {
	if err := s.resetApprovalKeysIn(SettingsLocalPath(s.projectDir)); err != nil {
		return err
	}
	return s.resetApprovalKeysIn(ClaudeJsonPath(s.homeDir))
}

// resetApprovalKeysIn strips the two .mcp.json approval keys from the JSON
// file at path — either a settings file (top-level keys) or ~/.claude.json
// (keys inside projects.<key>).  The file is left untouched when missing or
// when it holds neither key.
func (s *MCPStore) resetApprovalKeysIn(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("config: stat %s: %w", path, err)
	}
	doc, err := LoadOrderedJSON(path)
	if err != nil {
		return err
	}
	holder := doc
	if path == ClaudeJsonPath(s.homeDir) {
		projects, _ := doc.GetMap("projects")
		if projects == nil {
			return nil
		}
		holder, _ = projects.GetMap(s.ProjectKey())
		if holder == nil {
			return nil
		}
	}
	_, hasEnabled := holder.Get("enabledMcpjsonServers")
	_, hasDisabled := holder.Get("disabledMcpjsonServers")
	if !hasEnabled && !hasDisabled {
		return nil
	}
	holder.Delete("enabledMcpjsonServers")
	holder.Delete("disabledMcpjsonServers")
	return WriteOrderedJSONAtomic(path, doc)
}

// McpJsonHasServers reports whether the project's .mcp.json exists and
// defines at least one server — the condition behind reset-project-choices'
// second output line.
func (s *MCPStore) McpJsonHasServers() bool {
	if _, err := os.Stat(McpJsonPath(s.projectDir)); err != nil {
		return false
	}
	entries, err := s.ScopeEntries(MCPScopeProject)
	return err == nil && len(entries) > 0
}

// DesktopConfigServers reads the Claude Desktop app's MCP server definitions
// from ~/Library/Application Support/Claude/claude_desktop_config.json.
// Returns nil when the file does not exist (the common case on machines
// without the desktop app).
func DesktopConfigServers(homeDir string) ([]MCPServerEntry, error) {
	path := filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	doc := NewOrderedMap()
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	m, _ := doc.GetMap("mcpServers")
	if m == nil {
		return nil, nil
	}
	entries := make([]MCPServerEntry, 0, m.Len())
	for _, name := range m.Keys() {
		if raw, ok := m.GetMap(name); ok && raw != nil {
			entries = append(entries, MCPServerEntry{Name: name, Raw: raw})
		}
	}
	return entries, nil
}
