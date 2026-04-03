package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthConfig aggregates OAuth endpoint configuration.
// Corresponds to TS getOauthConfig().
type OAuthConfig struct {
	ClientID               string
	AuthorizeURL           string // console.anthropic.com/oauth/authorize
	ClaudeAIAuthorizeURL   string // claude.ai/oauth/authorize
	TokenURL               string
	ManualRedirectURL      string
	BaseAPIURL             string
	ConsoleSuccessURL      string
	ClaudeAISuccessURL     string
	RolesURL               string
	APIKeyURL              string
}

// DefaultOAuthConfig returns the standard Anthropic OAuth configuration.
func DefaultOAuthConfig() OAuthConfig {
	return OAuthConfig{
		ClientID:             "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		AuthorizeURL:         "https://console.anthropic.com/oauth/authorize",
		ClaudeAIAuthorizeURL: "https://claude.ai/oauth/authorize",
		TokenURL:             "https://console.anthropic.com/v1/oauth/token",
		ManualRedirectURL:    "https://console.anthropic.com/oauth/code",
		BaseAPIURL:           "https://api.anthropic.com",
		ConsoleSuccessURL:    "https://console.anthropic.com/oauth/success",
		ClaudeAISuccessURL:   "https://claude.ai/oauth/success",
		RolesURL:             "https://api.anthropic.com/api/oauth/roles",
		APIKeyURL:            "https://console.anthropic.com/settings/keys",
	}
}

// Client provides complete OAuth2 operations.
type Client struct {
	cfg        OAuthConfig
	httpClient *http.Client
	store      TokenStore
}

// NewClient creates an OAuth client.
func NewClient(cfg OAuthConfig, store TokenStore, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, httpClient: httpClient, store: store}
}

// AuthURLParams contains parameters for building the authorization URL.
type AuthURLParams struct {
	CodeChallenge   string
	State           string
	Port            int
	IsManual        bool
	LoginWithClaude bool   // true → use claude.ai OAuth endpoint
	InferenceOnly   bool   // true → request org:inference scope only (long-lived token)
	OrgUUID         string
	LoginHint       string
	LoginMethod     string // "sso" | "magic_link" | "google"
}

// BuildAuthURL constructs the PKCE authorization URL.
// Corresponds to TS buildAuthUrl().
func (c *Client) BuildAuthURL(params AuthURLParams) string {
	baseURL := c.cfg.AuthorizeURL
	if params.LoginWithClaude {
		baseURL = c.cfg.ClaudeAIAuthorizeURL
	}

	scopes := "org:inference user:profile org:billing org:teams"
	if params.InferenceOnly {
		scopes = "org:inference"
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", params.Port)
	if params.IsManual {
		redirectURI = c.cfg.ManualRedirectURL
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scopes)
	q.Set("code_challenge", params.CodeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", params.State)
	if params.OrgUUID != "" {
		q.Set("organization_uuid", params.OrgUUID)
	}
	if params.LoginHint != "" {
		q.Set("login_hint", params.LoginHint)
	}
	if params.LoginMethod != "" {
		q.Set("login_method", params.LoginMethod)
	}
	return baseURL + "?" + q.Encode()
}

// ExchangeCodeForTokens exchanges an authorization code for tokens.
// Timeout: 15s. Corresponds to TS exchangeCodeForTokens().
func (c *Client) ExchangeCodeForTokens(
	ctx context.Context,
	code, state, verifier string,
	port int,
) (*OAuthTokens, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("client_id", c.cfg.ClientID)
	body.Set("code", code)
	body.Set("redirect_uri", redirectURI)
	body.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oauth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth: token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth: token exchange failed: status %d", resp.StatusCode)
	}

	var tokenResp OAuthTokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("oauth: decode token response: %w", err)
	}

	expiresAt := time.Now().UnixMilli() + int64(tokenResp.ExpiresIn)*1000
	tokens := &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Scopes:       strings.Fields(tokenResp.Scope),
	}
	if tokenResp.Account != nil {
		tokens.TokenAccount = &TokenAccount{
			UUID:         tokenResp.Account.UUID,
			EmailAddress: tokenResp.Account.EmailAddress,
		}
		if tokenResp.Organization != nil {
			tokens.TokenAccount.OrganizationUUID = tokenResp.Organization.UUID
		}
	}
	return tokens, nil
}

// RefreshToken uses a refresh_token to obtain a new access token.
// Corresponds to TS refreshOAuthToken().
func (c *Client) RefreshToken(ctx context.Context, refreshToken string, scopes []string) (*OAuthTokens, error) {
	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("client_id", c.cfg.ClientID)
	body.Set("refresh_token", refreshToken)
	if len(scopes) > 0 {
		body.Set("scope", strings.Join(scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oauth: build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth: refresh request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth: refresh failed: status %d", resp.StatusCode)
	}

	var tokenResp OAuthTokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("oauth: decode refresh response: %w", err)
	}

	expiresAt := time.Now().UnixMilli() + int64(tokenResp.ExpiresIn)*1000
	tokens := &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Scopes:       scopes,
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken // keep old refresh token if not rotated
	}
	return tokens, nil
}

// IsTokenExpired reports whether a token is expired, with a 5-minute buffer.
// Corresponds to TS isOAuthTokenExpired().
func IsTokenExpired(expiresAt int64) bool {
	bufferMS := int64(5 * 60 * 1000)
	return time.Now().UnixMilli()+bufferMS >= expiresAt
}
