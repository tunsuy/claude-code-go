// Package oauth provides OAuth2 PKCE authentication for Claude Code.
package oauth

// OAuthTokens stores a complete OAuth token set.
type OAuthTokens struct {
	AccessToken      string        `json:"access_token"`
	RefreshToken     string        `json:"refresh_token"`
	ExpiresAt        int64         `json:"expires_at"` // Unix ms
	Scopes           []string      `json:"scopes"`
	SubscriptionType string        `json:"subscription_type,omitempty"` // "max"|"pro"|"enterprise"|"team"
	RateLimitTier    string        `json:"rate_limit_tier,omitempty"`
	TokenAccount     *TokenAccount `json:"token_account,omitempty"`
}

// TokenAccount corresponds to the account field in the token exchange response.
type TokenAccount struct {
	UUID             string `json:"uuid"`
	EmailAddress     string `json:"email_address"`
	OrganizationUUID string `json:"organization_uuid,omitempty"`
}

// OAuthTokenExchangeResponse is the /oauth/token endpoint response body.
type OAuthTokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	Account      *struct {
		UUID         string `json:"uuid"`
		EmailAddress string `json:"email_address"`
	} `json:"account,omitempty"`
	Organization *struct {
		UUID string `json:"uuid"`
	} `json:"organization,omitempty"`
}

// ProfileInfo corresponds to the fetchProfileInfo() result.
type ProfileInfo struct {
	SubscriptionType string
	RateLimitTier    string
	DisplayName      string
	HasExtraUsage    bool
	BillingType      string
	AccountCreatedAt string
	OrgCreatedAt     string
}
