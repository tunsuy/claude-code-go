package oauth

import (
	"context"
	"fmt"
	"sync"
)

// singleflightCall represents an in-flight or completed singleflight call.
type singleflightCall struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// singleflightGroup deduplicates concurrent calls with the same key.
// This is a minimal stdlib-only implementation equivalent to golang.org/x/sync/singleflight.
type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]*singleflightCall
}

func (g *singleflightGroup) Do(key string, fn func() (interface{}, error)) (interface{}, error, bool) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*singleflightCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, true
	}
	c := &singleflightCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	// Wrap the call body with panic recovery (P1-4).
	// Without this, a panic in fn() would leave c.wg.Done() uncalled and
	// crash the entire process.
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.err = fmt.Errorf("singleflight: panic recovered: %v", r)
			}
		}()
		c.val, c.err = fn()
	}()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err, false
}

// TokenManager manages the lifecycle of OAuth tokens (load, check, refresh).
// Uses singleflight to ensure concurrent refresh calls only execute once.
// Corresponds to TS checkAndRefreshOAuthTokenIfNeeded().
type TokenManager struct {
	store        TokenStore
	client       *Client
	refreshGroup singleflightGroup
}

// NewTokenManager creates a TokenManager.
func NewTokenManager(store TokenStore, client *Client) *TokenManager {
	return &TokenManager{
		store:  store,
		client: client,
	}
}

// CheckAndRefreshIfNeeded loads the current token and refreshes it if expired.
// Thread-safe: concurrent callers share the same refresh operation via singleflight.
func (m *TokenManager) CheckAndRefreshIfNeeded(ctx context.Context) (*OAuthTokens, error) {
	tokens, err := m.store.Load()
	if err != nil {
		return nil, fmt.Errorf("token manager: load: %w", err)
	}
	if tokens == nil {
		return nil, nil
	}

	if !IsTokenExpired(tokens.ExpiresAt) {
		return tokens, nil
	}

	// Token is expired — refresh via singleflight to avoid concurrent duplicate refreshes.
	v, refreshErr, _ := m.refreshGroup.Do("refresh", func() (interface{}, error) {
		return m.doRefresh(ctx, tokens)
	})
	if refreshErr != nil {
		return nil, refreshErr
	}
	return v.(*OAuthTokens), nil
}

// doRefresh executes the actual token refresh and persists the result.
func (m *TokenManager) doRefresh(ctx context.Context, tokens *OAuthTokens) (*OAuthTokens, error) {
	if tokens.RefreshToken == "" {
		return nil, fmt.Errorf("token manager: no refresh token available")
	}
	refreshed, err := m.client.RefreshToken(ctx, tokens.RefreshToken, tokens.Scopes)
	if err != nil {
		return nil, fmt.Errorf("token manager: refresh: %w", err)
	}
	if err := m.store.Save(refreshed); err != nil {
		return nil, fmt.Errorf("token manager: save refreshed token: %w", err)
	}
	return refreshed, nil
}

// HandleOAuth401Error handles an API 401 error by refreshing the token.
// Only triggers a refresh when failedAccessToken matches the currently stored token
// (prevents re-refreshing a token that was already refreshed by another goroutine).
// Corresponds to TS handleOAuth401Error().
func (m *TokenManager) HandleOAuth401Error(ctx context.Context, failedAccessToken string) error {
	tokens, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("token manager: 401 handler: load: %w", err)
	}
	if tokens == nil {
		return fmt.Errorf("token manager: 401 handler: no tokens stored")
	}

	// Only refresh if the token that failed matches the currently stored token.
	// If they differ, another goroutine already refreshed it.
	if tokens.AccessToken != failedAccessToken {
		return nil
	}

	_, refreshErr, _ := m.refreshGroup.Do("refresh", func() (interface{}, error) {
		return m.doRefresh(ctx, tokens)
	})
	return refreshErr
}
