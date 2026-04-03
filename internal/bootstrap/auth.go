package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/anthropics/claude-code-go/internal/oauth"
)

// newAuthCmd creates the `claude auth` subcommand with login/logout/status children.
func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication credentials",
		Long: longDesc(`
Manage your Anthropic API authentication.

Use 'claude auth login' to authenticate via OAuth or API key,
'claude auth logout' to remove stored credentials, and
'claude auth status' to check the current authentication state.
`),
		SilenceUsage: true,
	}

	cmd.AddCommand(
		newAuthLoginCmd(),
		newAuthLogoutCmd(),
		newAuthStatusCmd(),
	)
	return cmd
}

// newAuthLoginCmd implements `claude auth login`.
func newAuthLoginCmd() *cobra.Command {
	var (
		apiKey      string
		loginClaude bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Anthropic (OAuth PKCE or API key)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(apiKey, loginClaude)
		},
	}

	cmd.Flags().StringVar(&apiKey, "api-key", "", "Directly set an API key (skips OAuth browser flow)")
	cmd.Flags().BoolVar(&loginClaude, "claude", false, "Use claude.ai OAuth endpoint instead of console.anthropic.com")
	return cmd
}

// newAuthLogoutCmd implements `claude auth logout`.
func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored authentication credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogout()
		},
	}
}

// newAuthStatusCmd implements `claude auth status`.
func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus()
		},
	}
}

// ── Run functions ─────────────────────────────────────────────────────────────

func runAuthLogin(apiKeyOverride string, loginWithClaude bool) error {
	tokenStore := oauth.NewTokenStore()

	// Fast path: user supplied --api-key directly.
	if apiKeyOverride != "" {
		tokens := &oauth.OAuthTokens{
			AccessToken: apiKeyOverride,
		}
		if err := tokenStore.Save(tokens); err != nil {
			return fmt.Errorf("auth: save API key: %w", err)
		}
		fmt.Println("API key saved successfully.")
		return nil
	}

	// OAuth PKCE flow.
	verifier, err := oauth.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("auth: generate code verifier: %w", err)
	}
	challenge := oauth.GenerateCodeChallenge(verifier)
	state, err := oauth.GenerateState()
	if err != nil {
		return fmt.Errorf("auth: generate state: %w", err)
	}

	oauthCfg := oauth.DefaultOAuthConfig()
	oauthClient := oauth.NewClient(oauthCfg, tokenStore, nil)

	authURL := oauthClient.BuildAuthURL(oauth.AuthURLParams{
		CodeChallenge:   challenge,
		State:           state,
		IsManual:        true,
		LoginWithClaude: loginWithClaude,
	})

	fmt.Printf("Opening browser for authentication...\n%s\n\n", authURL)
	openBrowser(authURL) //nolint:errcheck

	fmt.Print("Paste the authorization code: ")
	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return fmt.Errorf("auth: read code: %w", err)
	}

	tokens, err := oauthClient.ExchangeCodeForTokens(context.Background(), code, state, verifier, 0)
	if err != nil {
		return fmt.Errorf("auth: exchange code: %w", err)
	}

	if err := tokenStore.Save(tokens); err != nil {
		return fmt.Errorf("auth: save tokens: %w", err)
	}

	fmt.Println("Logged in successfully.")
	return nil
}

func runAuthLogout() error {
	tokenStore := oauth.NewTokenStore()
	if err := tokenStore.Delete(); err != nil {
		return fmt.Errorf("auth: delete tokens: %w", err)
	}
	fmt.Println("Logged out successfully.")
	return nil
}

func runAuthStatus() error {
	tokenStore := oauth.NewTokenStore()
	oauthCfg := oauth.DefaultOAuthConfig()
	oauthClient := oauth.NewClient(oauthCfg, tokenStore, nil)
	tm := oauth.NewTokenManager(tokenStore, oauthClient)

	tokens, err := tm.CheckAndRefreshIfNeeded(context.Background())
	if err != nil || tokens == nil || tokens.AccessToken == "" {
		// Fall back to env var.
		if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			fmt.Println("Authenticated via ANTHROPIC_API_KEY environment variable.")
			return nil
		}
		fmt.Fprintln(os.Stderr, "Not authenticated. Run 'claude auth login'.")
		os.Exit(1)
	}
	fmt.Println("Authenticated.")
	return nil
}

// openBrowser attempts to open url in the default browser.
func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "cmd", []string{"/c", "start", url}
	default: // linux, etc.
		cmd, args = "xdg-open", []string{url}
	}
	return exec.Command(cmd, args...).Start()
}
