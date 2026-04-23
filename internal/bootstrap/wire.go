package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/config"
	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/internal/hooks"
	"github.com/tunsuy/claude-code-go/internal/mcp"
	"github.com/tunsuy/claude-code-go/internal/oauth"
	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// P1-F: Compile-time interface assertions for AppContainer field types.
//
// engine.engineImpl is unexported; its assertion lives in internal/engine/engine.go:
//
//	var _ engine.QueryEngine = (*engineImpl)(nil)
//
// api.directClient is unexported; its assertion lives in internal/api/client.go:
//
//	var _ api.Client = (*directClient)(nil)
//
// The assertion below verifies that the concrete return type of engine.New
// satisfies engine.QueryEngine at this package boundary.  It is checked by
// the compiler whenever this file is compiled — no runtime overhead.
var _ engine.QueryEngine = engine.New(engine.Config{})

// ContainerOptions is the minimal set of startup parameters fed into BuildContainer.
// It is populated by runInteractiveOrHeadless before dispatching.
type ContainerOptions struct {
	HomeDir       string
	WorkingDir    string
	ModelOverride string
	Verbose       bool
	Debug         bool
	DebugFile     string // File path for debug log output (empty = stderr when Debug is true)
}

// AppContainer holds all wired application dependencies.
// It is the single object threaded through the run functions.
type AppContainer struct {
	// QueryEngine drives all LLM interactions.
	QueryEngine engine.QueryEngine
	// AppStateStore holds the global reactive application state.
	AppStateStore *state.AppStateStore
	// ToolRegistry holds all registered tools.
	ToolRegistry *tools.Registry
	// MCPPool manages MCP server connections.
	MCPPool *mcp.Pool
	// Settings is the merged layered config.
	Settings *config.LayeredSettings
	// PermAskCh receives permission requests from the engine (for TUI to consume).
	PermAskCh <-chan permissions.AskRequest
	// PermRespCh is used by TUI to send permission responses back to the engine.
	PermRespCh chan<- permissions.AskResponse
}

// defaultModel is used when no model override is provided.
const defaultModel = "claude-opus-4-5"

// BuildContainer wires up the full application container used in interactive mode.
//
// Initialization order:
//  1. Config loading (layered settings merge)
//  2. OAuth token pre-warm
//  3. API client construction
//  4. Tool registry setup
//  5. Permission checker setup (HIL support)
//  6. Engine construction
//  7. App state store construction
func BuildContainer(opts ContainerOptions) (*AppContainer, error) {
	// ── Phase 1: Config ─────────────────────────────────────────────────────
	settings, err := loadSettings(opts.HomeDir, opts.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("wire: load settings: %w", err)
	}

	// ── Phase 2: OAuth pre-warm ──────────────────────────────────────────────
	apiKey, err := resolveAPIKey(settings)
	if err != nil {
		return nil, fmt.Errorf("wire: resolve API key: %w", err)
	}

	// ── Phase 3: API client ──────────────────────────────────────────────────
	apiClient, err := buildAPIClient(settings, apiKey, opts)
	if err != nil {
		return nil, fmt.Errorf("wire: build API client: %w", err)
	}

	// ── Phase 4: Tool registry ───────────────────────────────────────────────
	reg := tools.NewRegistry()
	RegisterBuiltinTools(reg)

	// ── Phase 5: MCP pool (deferred connections happen on first use) ─────────
	pool := mcp.NewPool()

	// ── Phase 6: App state store (needed for permission context) ─────────────
	model := resolveModel(settings, opts.ModelOverride)
	appState := buildAppState(settings, opts, model)
	store := state.NewAppStateStore(appState)

	// ── Phase 7: Permission checker (HIL support) ────────────────────────────
	// Create permission channels for TUI ↔ Engine communication.
	askCh := make(chan permissions.AskRequest, 1)
	respCh := make(chan permissions.AskResponse, 1)

	// Build the permission checker with the tool registry and channels.
	permChecker := permissions.NewChecker(permissions.CheckerConfig{
		PermCtx:    appState.ToolPermissionContext,
		Dispatcher: hooks.NewDispatcher(nil, false), // Empty dispatcher (no hooks configured yet)
		Registry:   reg,
		AskCh:      askCh,
		RespCh:     respCh,
	})

	// ── Phase 8: Engine construction ─────────────────────────────────────────
	eng := engine.New(engine.Config{
		Client:            apiClient,
		Registry:          reg,
		Model:             model,
		PermissionChecker: permChecker,
	})

	return &AppContainer{
		QueryEngine:   eng,
		AppStateStore: store,
		ToolRegistry:  reg,
		MCPPool:       pool,
		Settings:      settings,
		PermAskCh:     askCh,
		PermRespCh:    respCh,
	}, nil
}

// BuildHeadlessContainer wires up a minimal container for non-interactive (-p) mode.
// No MCP connections are pre-established; OAuth pre-warm is still performed.
func BuildHeadlessContainer(opts ContainerOptions) (*AppContainer, error) {
	// Headless mode uses the same wiring path; any optimisations (skipping
	// MCP init, etc.) can be added here in future iterations.
	return BuildContainer(opts)
}

// BuildContainerWithClient wires up an AppContainer using the provided api.Client.
// This bypasses OAuth and API key resolution, making it suitable for tests that
// inject a mock client.
func BuildContainerWithClient(opts ContainerOptions, client api.Client) (*AppContainer, error) {
	// ── Phase 1: Config ─────────────────────────────────────────────────────
	settings, err := loadSettings(opts.HomeDir, opts.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("wire: load settings: %w", err)
	}

	// ── Phase 4: Tool registry ───────────────────────────────────────────────
	reg := tools.NewRegistry()
	RegisterBuiltinTools(reg)

	// ── Phase 5: MCP pool (deferred connections happen on first use) ─────────
	pool := mcp.NewPool()

	// ── Phase 6: App state store (needed for permission context) ─────────────
	model := resolveModel(settings, opts.ModelOverride)
	appState := buildAppState(settings, opts, model)
	store := state.NewAppStateStore(appState)

	// ── Phase 7: Permission checker (HIL support) ────────────────────────────
	askCh := make(chan permissions.AskRequest, 1)
	respCh := make(chan permissions.AskResponse, 1)

	permChecker := permissions.NewChecker(permissions.CheckerConfig{
		PermCtx:    appState.ToolPermissionContext,
		Dispatcher: hooks.NewDispatcher(nil, false),
		Registry:   reg,
		AskCh:      askCh,
		RespCh:     respCh,
	})

	// ── Phase 8: Engine construction ─────────────────────────────────────────
	eng := engine.New(engine.Config{
		Client:            client,
		Registry:          reg,
		Model:             model,
		PermissionChecker: permChecker,
	})

	return &AppContainer{
		QueryEngine:   eng,
		AppStateStore: store,
		ToolRegistry:  reg,
		MCPPool:       pool,
		Settings:      settings,
		PermAskCh:     askCh,
		PermRespCh:    respCh,
	}, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// loadSettings calls config.NewLoader and returns merged LayeredSettings.
func loadSettings(homeDir, workingDir string) (*config.LayeredSettings, error) {
	loader := config.NewLoader(homeDir, workingDir)
	return loader.Load()
}

// resolveAPIKey returns the API key to use.
// Priority: OAuth token → settings.APIKey → ANTHROPIC_API_KEY/OPENAI_API_KEY env var.
func resolveAPIKey(settings *config.LayeredSettings) (string, error) {
	// Try OAuth token store first (Phase 3 pre-warm).
	tokenStore := oauth.NewTokenStore()
	oauthCfg := oauth.DefaultOAuthConfig()
	oauthClient := oauth.NewClient(oauthCfg, tokenStore, nil)
	tm := oauth.NewTokenManager(tokenStore, oauthClient)

	tokens, err := tm.CheckAndRefreshIfNeeded(context.Background())
	if err == nil && tokens != nil && tokens.AccessToken != "" {
		return tokens.AccessToken, nil
	}
	// Non-fatal: fall through to static key resolution.

	// Try merged settings key.
	if settings.Merged != nil && settings.Merged.APIKey != "" {
		return settings.Merged.APIKey, nil
	}

	// Environment variable - check ANTHROPIC_API_KEY first, then OPENAI_API_KEY.
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key, nil
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key, nil
	}

	// Return empty string — the API client will surface auth errors on first use.
	return "", nil
}

// buildAPIClient constructs the api.Client from merged settings.
func buildAPIClient(settings *config.LayeredSettings, apiKey string, opts ContainerOptions) (api.Client, error) {
	cfg := api.ClientConfig{
		Provider:  api.ProviderDirect,
		APIKey:    apiKey,
		Debug:     opts.Debug,
		DebugFile: opts.DebugFile,
	}
	if settings.Merged != nil {
		if settings.Merged.BaseURL != "" {
			cfg.BaseURL = settings.Merged.BaseURL
		}
		// Set provider from settings
		if settings.Merged.Provider != "" {
			cfg.Provider = api.Provider(settings.Merged.Provider)
		}
		// OpenAI specific settings
		if settings.Merged.OpenAIOrganization != "" {
			cfg.OpenAIOrganization = settings.Merged.OpenAIOrganization
		}
		if settings.Merged.OpenAIProject != "" {
			cfg.OpenAIProject = settings.Merged.OpenAIProject
		}
		// Bedrock/Vertex detection via env is handled inside api.NewClient.
	}
	return api.NewClient(cfg, nil)
}

// resolveModel returns the active model, checking (in order):
// CLI override → merged settings → defaultModel.
func resolveModel(settings *config.LayeredSettings, override string) string {
	if override != "" {
		return override
	}
	if settings != nil && settings.Merged != nil && settings.Merged.Model != "" {
		return settings.Merged.Model
	}
	return defaultModel
}

// buildAppState constructs the initial AppState from settings and options.
func buildAppState(settings *config.LayeredSettings, opts ContainerOptions, model string) state.AppState {
	appState := state.GetDefaultAppState()
	appState.WorkingDir = opts.WorkingDir
	appState.Verbose = opts.Verbose

	// Determine provider name
	provider := "anthropic"
	if settings.Merged != nil && settings.Merged.Provider != "" {
		provider = settings.Merged.Provider
	}

	appState.MainLoopModel = state.ModelSetting{
		ModelID:  model,
		Provider: provider,
	}

	if settings.Merged != nil {
		appState.Settings = *settings.Merged
	}

	return appState
}

// applyPermissionFlags applies rootFlags permission overrides to the AppStateStore.
func applyPermissionFlags(container *AppContainer, f *rootFlags) {
	container.AppStateStore.SetState(func(prev state.AppState) state.AppState {
		ctx := prev.ToolPermissionContext

		// --dangerously-skip-permissions sets bypass mode.
		if f.dangerouslySkipPermissions {
			ctx.Mode = types.PermissionModeBypassPermissions
		}

		// --permission-mode overrides if set.
		if f.permissionMode != "" {
			ctx.Mode = types.PermissionMode(f.permissionMode)
		}

		// --add-dir adds additional allowed directories.
		for _, dir := range f.addDirs {
			if dir != "" {
				if ctx.AdditionalWorkingDirectories == nil {
					ctx.AdditionalWorkingDirectories = make(map[string]types.AdditionalWorkingDirectory)
				}
				ctx.AdditionalWorkingDirectories[dir] = types.AdditionalWorkingDirectory{
					Path:   dir,
					Source: types.RuleSourceCLI,
				}
			}
		}

		prev.ToolPermissionContext = ctx
		return prev
	})
}
