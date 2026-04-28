---
package: oauth
import_path: internal/oauth
layer: services
generated_at: 2026-04-28T12:11:54Z
source_files: [client.go, crypto.go, crypto_aes.go, listener.go, refresh.go, store.go, store_darwin.go, store_other.go, types.go]
---

# internal/oauth

> Layer: **Services** · Files: 9 · Interfaces: 1 · Structs: 13 · Functions: 9

## Interfaces

### TokenStore (3 methods)
> TokenStore is the interface for persisting OAuth tokens.

```go
type TokenStore interface {
    Load() (*OAuthTokens, error)
    Save(tokens *OAuthTokens) error
    Delete() error
}
```

## Structs

- **AuthCodeListener** — 7 fields
- **AuthURLParams** — 9 fields: CodeChallenge, State, Port, IsManual, LoginWithClaude, InferenceOnly, OrgUUID, LoginHint, ...
- **Client** — 3 fields
- **FileStore** — 1 fields
- **KeychainStore** — 2 fields
- **KeychainStore** — 2 fields
- **MemoryStore** — 2 fields
- **OAuthConfig** — 10 fields: ClientID, AuthorizeURL, ClaudeAIAuthorizeURL, TokenURL, ManualRedirectURL, BaseAPIURL, ConsoleSuccessURL, ClaudeAISuccessURL, ...
- **OAuthTokenExchangeResponse** — 7 fields: AccessToken, RefreshToken, ExpiresIn, Scope, TokenType, Account, Organization
- **OAuthTokens** — 7 fields: AccessToken, RefreshToken, ExpiresAt, Scopes, SubscriptionType, RateLimitTier, TokenAccount
- **ProfileInfo** — 7 fields: SubscriptionType, RateLimitTier, DisplayName, HasExtraUsage, BillingType, AccountCreatedAt, OrgCreatedAt
- **TokenAccount** — 3 fields: UUID, EmailAddress, OrganizationUUID
- **TokenManager** — 3 fields

## Functions

- `DefaultOAuthConfig() OAuthConfig`
- `GenerateCodeChallenge(verifier string) string`
- `GenerateCodeVerifier() (string, error)`
- `GenerateState() (string, error)`
- `IsTokenExpired(expiresAt int64) bool`
- `NewAuthCodeListener(callbackPath string) *AuthCodeListener`
- `NewClient(cfg OAuthConfig, store TokenStore, httpClient *http.Client) *Client`
- `NewTokenManager(store TokenStore, client *Client) *TokenManager`
- `NewTokenStore() TokenStore`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `mockClient` in `internal/engine/engine_test.go`
- `mockClient` in `test/integration/engine_e2e_test.go`

**Exported type references (files that use types from this package):**
- `AuthURLParams` → `internal/bootstrap/auth.go`
- `OAuthTokens` → `internal/bootstrap/auth.go`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`

